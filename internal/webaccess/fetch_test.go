package webaccess

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
)

type browserRendererFunc func(context.Context, *url.URL) (renderedPage, error)

func (render browserRendererFunc) Render(ctx context.Context, target *url.URL) (renderedPage, error) {
	return render(ctx, target)
}

type closeTrackingBrowserRenderer struct {
	closed int
	err    error
}

func (*closeTrackingBrowserRenderer) Render(context.Context, *url.URL) (renderedPage, error) {
	return renderedPage{}, errors.New("not used")
}

func (renderer *closeTrackingBrowserRenderer) Close(context.Context) error {
	renderer.closed++
	return renderer.err
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (roundTrip roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return roundTrip(request)
}

func clientForFetchTest(t *testing.T, server *httptest.Server, config Config) *Client {
	t.Helper()
	client, err := newClient(config, dependencies{fetchHTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func TestClientCloseDelegatesToStatefulBrowserRenderer(t *testing.T) {
	wantErr := errors.New("cleanup failed")
	renderer := &closeTrackingBrowserRenderer{err: wantErr}
	client, err := newClient(testWebAccessConfig(), dependencies{browserRenderer: renderer})
	if err != nil {
		t.Fatal(err)
	}
	if closeErr := client.Close(context.Background()); !errors.Is(closeErr, wantErr) {
		t.Fatalf("Close() error = %v, want %v", closeErr, wantErr)
	}
	if renderer.closed != 1 {
		t.Fatalf("browser renderer close calls = %d, want 1", renderer.closed)
	}
}

func TestFetchReportsDirectHTTPSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "text/plain; charset=utf-8")
		writer.WriteHeader(http.StatusOK)
		_, _ = writer.Write([]byte("direct content"))
	}))
	t.Cleanup(server.Close)

	response, err := clientForFetchTest(t, server, testWebAccessConfig()).Fetch(context.Background(), FetchRequest{URL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	want := FetchResponse{
		Status:        FetchStatusSuccess,
		FetchMethod:   FetchMethodDirectHTTP,
		Attempts:      []FetchAttempt{{Method: FetchMethodDirectHTTP, Outcome: FetchAttemptSuccess, HTTPStatus: http.StatusOK}},
		RetryStrategy: FetchRetryNone,
	}
	if response.Schema != FetchResponseSchema || response.Status != want.Status || response.FetchMethod != want.FetchMethod ||
		!reflect.DeepEqual(response.Attempts, want.Attempts) || response.RetryStrategy != want.RetryStrategy {
		t.Fatalf("fetch recovery metadata = %+v, want %+v", response, want)
	}
}

func TestFetchUsesCoherentChromeNavigationHeaders(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		userAgent := request.Header.Get("User-Agent")
		if !strings.Contains(userAgent, " Chrome/") || strings.Contains(userAgent, "Denova/") {
			t.Fatalf("User-Agent = %q", userAgent)
		}
		wantHeaders := map[string]string{
			"Cache-Control":             "no-cache",
			"Sec-Fetch-Dest":            "document",
			"Sec-Fetch-Mode":            "navigate",
			"Sec-Fetch-Site":            "none",
			"Sec-Fetch-User":            "?1",
			"Upgrade-Insecure-Requests": "1",
		}
		for name, want := range wantHeaders {
			if got := request.Header.Get(name); got != want {
				t.Fatalf("%s = %q, want %q", name, got, want)
			}
		}
		writer.Header().Set("Content-Type", "text/plain")
		_, _ = writer.Write([]byte("header-compatible response"))
	}))
	t.Cleanup(target.Close)

	response, err := clientForFetchTest(t, target, testWebAccessConfig()).Fetch(context.Background(), FetchRequest{URL: target.URL})
	if err != nil {
		t.Fatal(err)
	}
	if response.FetchMethod != FetchMethodDirectHTTP || response.Content != "header-compatible response" {
		t.Fatalf("direct response = %+v", response)
	}
}

func TestFetchUsesJinaReaderForJavaScriptPage(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = writer.Write([]byte(`<!doctype html><html><head><title>App shell</title></head><body><div id="root"></div><script src="/a.js"></script><script src="/b.js"></script><script src="/c.js"></script><script src="/d.js"></script></body></html>`))
	}))
	t.Cleanup(target.Close)

	jina := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Accept") != "application/json" || request.Header.Get("X-No-Cache") != "true" {
			t.Fatalf("unexpected Jina headers: %v", request.Header)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"code":200,"status":20000,"data":{"title":"Hosted title","description":"Hosted description","url":"` + target.URL + `","content":"# Hosted title\n\nHosted **markdown**","httpStatus":200}}`))
	}))
	t.Cleanup(jina.Close)

	client, err := newClient(testWebAccessConfig(), dependencies{
		fetchHTTPClient:   target.Client(),
		jinaHTTPClient:    jina.Client(),
		jinaReaderBaseURL: jina.URL + "/",
	})
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Fetch(context.Background(), FetchRequest{URL: target.URL})
	if err != nil {
		t.Fatal(err)
	}
	wantAttempts := []FetchAttempt{
		{Method: FetchMethodDirectHTTP, Outcome: FetchAttemptJavaScriptRequired, HTTPStatus: http.StatusOK},
		{Method: FetchMethodJinaReader, Outcome: FetchAttemptSuccess, HTTPStatus: http.StatusOK},
	}
	if response.Status != FetchStatusSuccess || response.FetchMethod != FetchMethodJinaReader ||
		response.ContentType != "text/markdown" || response.Content != "# Hosted title\n\nHosted **markdown**" ||
		!reflect.DeepEqual(response.Attempts, wantAttempts) {
		t.Fatalf("Jina fallback response = %+v, want attempts %+v", response, wantAttempts)
	}
}

func TestFetchPreservesSPAFragmentWhenCallingJinaReader(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "text/html")
		_, _ = writer.Write([]byte(`<html><body><div id="root"></div><script>1</script><script>2</script><script>3</script><script>4</script></body></html>`))
	}))
	t.Cleanup(target.Close)
	targetURL := target.URL + "/#/chapter/7"
	jina := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != "/" {
			t.Fatalf("Jina fragment request = %s %s", request.Method, request.URL.String())
		}
		var input struct {
			URL string `json:"url"`
		}
		if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
			t.Fatal(err)
		}
		if input.URL != targetURL {
			t.Fatalf("Jina target URL = %q, want %q", input.URL, targetURL)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"code":200,"status":20000,"data":{"title":"SPA","url":"` + targetURL + `","content":"# Rendered SPA","httpStatus":200}}`))
	}))
	t.Cleanup(jina.Close)
	client, err := newClient(testWebAccessConfig(), dependencies{
		fetchHTTPClient: target.Client(), jinaHTTPClient: jina.Client(), jinaReaderBaseURL: jina.URL + "/",
	})
	if err != nil {
		t.Fatal(err)
	}

	response, err := client.Fetch(context.Background(), FetchRequest{URL: targetURL})
	if err != nil {
		t.Fatal(err)
	}
	if response.FetchMethod != FetchMethodJinaReader || response.Content != "# Rendered SPA" {
		t.Fatalf("SPA response = %+v", response)
	}
}

func TestFetchUsesBrowserWhenDirectAndJinaAreBlocked(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		writer.WriteHeader(http.StatusForbidden)
		_, _ = writer.Write([]byte(`<html><body><script src="/challenge.js"></script></body></html>`))
	}))
	t.Cleanup(target.Close)

	jina := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"code":200,"status":20000,"data":{"url":"` + target.URL + `","content":"security verification","warning":"Target URL returned error 403: Forbidden","httpStatus":403}}`))
	}))
	t.Cleanup(jina.Close)

	articleText := strings.Repeat("Rendered article content remains readable. ", 20)
	renderer := browserRendererFunc(func(_ context.Context, targetURL *url.URL) (renderedPage, error) {
		return renderedPage{
			FinalURL:   targetURL,
			HTML:       `<html><body><nav>Rendered navigation noise</nav><article><h1>Rendered title</h1><p>` + articleText + `</p><a href="/source">Source</a></article></body></html>`,
			HTTPStatus: http.StatusOK,
		}, nil
	})
	client, err := newClient(testWebAccessConfig(), dependencies{
		fetchHTTPClient:   target.Client(),
		jinaHTTPClient:    jina.Client(),
		jinaReaderBaseURL: jina.URL + "/",
		browserRenderer:   renderer,
	})
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Fetch(context.Background(), FetchRequest{URL: target.URL})
	if err != nil {
		t.Fatal(err)
	}
	wantAttempts := []FetchAttempt{
		{Method: FetchMethodDirectHTTP, Outcome: FetchAttemptAccessDenied, HTTPStatus: http.StatusForbidden},
		{Method: FetchMethodJinaReader, Outcome: FetchAttemptAccessDenied, HTTPStatus: http.StatusForbidden},
		{Method: FetchMethodBrowser, Outcome: FetchAttemptSuccess, HTTPStatus: http.StatusOK},
	}
	if response.Status != FetchStatusSuccess || response.FetchMethod != FetchMethodBrowser ||
		!strings.Contains(response.Content, "Rendered article content") || strings.Contains(response.Content, "navigation noise") ||
		!strings.Contains(response.Content, "[Source]("+target.URL+"/source)") || !reflect.DeepEqual(response.Attempts, wantAttempts) {
		t.Fatalf("browser fallback response = %+v, want attempts %+v", response, wantAttempts)
	}
}

func TestFetchAcceptsSubstantiveBrowserDocumentAfterChallengeStatus(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusForbidden)
	}))
	t.Cleanup(target.Close)
	jina := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"data":{"warning":"Target URL returned error 403: Forbidden","httpStatus":403}}`))
	}))
	t.Cleanup(jina.Close)
	articleText := strings.Repeat("The browser completed the access challenge and rendered the article. ", 12)
	client, err := newClient(testWebAccessConfig(), dependencies{
		fetchHTTPClient: target.Client(), jinaHTTPClient: jina.Client(), jinaReaderBaseURL: jina.URL + "/",
		browserRenderer: browserRendererFunc(func(_ context.Context, targetURL *url.URL) (renderedPage, error) {
			return renderedPage{
				FinalURL: targetURL, HTTPStatus: http.StatusForbidden, RenderMode: browserRenderModeStealth,
				HTML: `<html><head><title>Rendered article</title></head><body><main><h1>Rendered article</h1><p>` + articleText + `</p></main></body></html>`,
			}, nil
		}),
	})
	if err != nil {
		t.Fatal(err)
	}

	response, err := client.Fetch(context.Background(), FetchRequest{URL: target.URL})
	if err != nil {
		t.Fatal(err)
	}
	if response.Status != FetchStatusSuccess || response.FetchMethod != FetchMethodBrowser ||
		!strings.Contains(response.Content, "completed the access challenge") {
		t.Fatalf("readable challenge response = %+v", response)
	}
	browserAttempt := response.Attempts[len(response.Attempts)-1]
	if browserAttempt.Outcome != FetchAttemptSuccess || browserAttempt.HTTPStatus != http.StatusForbidden ||
		!strings.Contains(browserAttempt.Message, "go-rod/stealth") {
		t.Fatalf("browser challenge attempt = %+v", browserAttempt)
	}
}

func TestFetchRejectsBrowserChallengeShellAfterStealthFallback(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusForbidden)
	}))
	t.Cleanup(target.Close)
	jina := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"data":{"warning":"Target URL returned error 403: Forbidden","httpStatus":403}}`))
	}))
	t.Cleanup(jina.Close)
	client, err := newClient(testWebAccessConfig(), dependencies{
		fetchHTTPClient: target.Client(), jinaHTTPClient: jina.Client(), jinaReaderBaseURL: jina.URL + "/",
		browserRenderer: browserRendererFunc(func(_ context.Context, targetURL *url.URL) (renderedPage, error) {
			return renderedPage{
				FinalURL: targetURL, HTTPStatus: http.StatusForbidden, RenderMode: browserRenderModeStealth,
				HTML: `<!doctype html><html><head><meta id="zh-zse-ck" content="challenge"></head><body><p>知乎，让每一次点击都充满意义。</p><script src="https://static.zhihu.com/zse-ck/v4/challenge.js"></script></body></html>`,
			}, nil
		}),
	})
	if err != nil {
		t.Fatal(err)
	}

	response, err := client.Fetch(context.Background(), FetchRequest{URL: target.URL})
	if err != nil {
		t.Fatal(err)
	}
	if response.Status != FetchStatusBlocked || response.RetryStrategy != FetchRetryUseAlternateSource {
		t.Fatalf("challenge shell response = %+v", response)
	}
	browserAttempt := response.Attempts[len(response.Attempts)-1]
	if browserAttempt.Outcome != FetchAttemptAccessDenied || !strings.Contains(browserAttempt.Message, "go-rod/stealth") {
		t.Fatalf("challenge shell attempt = %+v", browserAttempt)
	}
}

func TestFetchReturnsActionableBlockedResponseWhenEveryLayerIsDenied(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusForbidden)
	}))
	t.Cleanup(target.Close)
	jina := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"code":200,"status":20000,"data":{"url":"` + target.URL + `","warning":"Target URL returned error 403: Forbidden","httpStatus":403}}`))
	}))
	t.Cleanup(jina.Close)
	renderer := browserRendererFunc(func(_ context.Context, targetURL *url.URL) (renderedPage, error) {
		return renderedPage{FinalURL: targetURL, HTTPStatus: http.StatusForbidden}, nil
	})
	client, err := newClient(testWebAccessConfig(), dependencies{
		fetchHTTPClient: target.Client(), jinaHTTPClient: jina.Client(), jinaReaderBaseURL: jina.URL + "/", browserRenderer: renderer,
	})
	if err != nil {
		t.Fatal(err)
	}

	response, err := client.Fetch(context.Background(), FetchRequest{URL: target.URL})
	if err != nil {
		t.Fatalf("blocked page should return structured recovery metadata, got %v", err)
	}
	wantAttempts := []FetchAttempt{
		{Method: FetchMethodDirectHTTP, Outcome: FetchAttemptAccessDenied, HTTPStatus: http.StatusForbidden},
		{Method: FetchMethodJinaReader, Outcome: FetchAttemptAccessDenied, HTTPStatus: http.StatusForbidden},
		{Method: FetchMethodBrowser, Outcome: FetchAttemptAccessDenied, HTTPStatus: http.StatusForbidden, Message: browserChallengeMessage},
	}
	if response.Status != FetchStatusBlocked || response.RetryStrategy != FetchRetryUseAlternateSource ||
		response.URL != target.URL || !reflect.DeepEqual(response.Attempts, wantAttempts) || response.SuggestedAction == "" {
		t.Fatalf("blocked response = %+v, want attempts %+v", response, wantAttempts)
	}
}

func TestFetchUsesBrowserWhenJinaServiceIsUnavailable(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "text/html")
		_, _ = writer.Write([]byte(`<html><body><div id="root"></div><script>1</script><script>2</script><script>3</script><script>4</script></body></html>`))
	}))
	t.Cleanup(target.Close)
	jina := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(jina.Close)
	renderer := browserRendererFunc(func(_ context.Context, targetURL *url.URL) (renderedPage, error) {
		return renderedPage{
			FinalURL: targetURL, HTTPStatus: http.StatusOK,
			HTML: `<html><body><article><h1>Local renderer</h1><p>` + strings.Repeat("Browser fallback content. ", 20) + `</p></article></body></html>`,
		}, nil
	})
	client, err := newClient(testWebAccessConfig(), dependencies{
		fetchHTTPClient: target.Client(), jinaHTTPClient: jina.Client(), jinaReaderBaseURL: jina.URL + "/", browserRenderer: renderer,
	})
	if err != nil {
		t.Fatal(err)
	}

	response, err := client.Fetch(context.Background(), FetchRequest{URL: target.URL})
	if err != nil {
		t.Fatal(err)
	}
	wantAttempts := []FetchAttempt{
		{Method: FetchMethodDirectHTTP, Outcome: FetchAttemptJavaScriptRequired, HTTPStatus: http.StatusOK},
		{Method: FetchMethodJinaReader, Outcome: FetchAttemptProviderUnavailable, HTTPStatus: http.StatusServiceUnavailable},
		{Method: FetchMethodBrowser, Outcome: FetchAttemptSuccess, HTTPStatus: http.StatusOK},
	}
	if response.FetchMethod != FetchMethodBrowser || !strings.Contains(response.Content, "Browser fallback content") ||
		!reflect.DeepEqual(response.Attempts, wantAttempts) {
		t.Fatalf("service fallback response = %+v, want attempts %+v", response, wantAttempts)
	}
}

func TestFetchUsesJinaReaderWhenDirectNetworkFails(t *testing.T) {
	targetURL := "https://example.com/network-failure"
	directClient := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("connection refused")
	})}
	jina := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"data":{"title":"Recovered","url":"` + targetURL + `","content":"Recovered through Jina","httpStatus":200}}`))
	}))
	t.Cleanup(jina.Close)
	browserCalled := false
	client, err := newClient(testWebAccessConfig(), dependencies{
		fetchHTTPClient: directClient, jinaHTTPClient: jina.Client(), jinaReaderBaseURL: jina.URL + "/",
		browserRenderer: browserRendererFunc(func(context.Context, *url.URL) (renderedPage, error) {
			browserCalled = true
			return renderedPage{}, errors.New("browser should not be called")
		}),
	})
	if err != nil {
		t.Fatal(err)
	}

	response, err := client.Fetch(context.Background(), FetchRequest{URL: targetURL})
	if err != nil {
		t.Fatal(err)
	}
	wantAttempts := []FetchAttempt{
		{Method: FetchMethodDirectHTTP, Outcome: FetchAttemptNetworkError},
		{Method: FetchMethodJinaReader, Outcome: FetchAttemptSuccess, HTTPStatus: http.StatusOK},
	}
	if response.FetchMethod != FetchMethodJinaReader || response.Content != "Recovered through Jina" ||
		!reflect.DeepEqual(response.Attempts, wantAttempts) || browserCalled {
		t.Fatalf("network fallback response = %+v, browser_called=%v", response, browserCalled)
	}
}

func TestFetchUsesBrowserWhenJinaRequestFails(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "text/html")
		_, _ = writer.Write([]byte(`<html><body><div id="root"></div><script>1</script><script>2</script><script>3</script><script>4</script></body></html>`))
	}))
	t.Cleanup(target.Close)
	jinaClient := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("hosted reader connection failed")
	})}
	client, err := newClient(testWebAccessConfig(), dependencies{
		fetchHTTPClient: target.Client(), jinaHTTPClient: jinaClient,
		browserRenderer: browserRendererFunc(func(_ context.Context, targetURL *url.URL) (renderedPage, error) {
			return renderedPage{
				FinalURL: targetURL, HTTPStatus: http.StatusOK,
				HTML: `<html><body><article><h1>Browser recovery</h1><p>` + strings.Repeat("Readable browser content. ", 20) + `</p></article></body></html>`,
			}, nil
		}),
	})
	if err != nil {
		t.Fatal(err)
	}

	response, err := client.Fetch(context.Background(), FetchRequest{URL: target.URL})
	if err != nil {
		t.Fatal(err)
	}
	wantAttempts := []FetchAttempt{
		{Method: FetchMethodDirectHTTP, Outcome: FetchAttemptJavaScriptRequired, HTTPStatus: http.StatusOK},
		{Method: FetchMethodJinaReader, Outcome: FetchAttemptProviderUnavailable},
		{Method: FetchMethodBrowser, Outcome: FetchAttemptSuccess, HTTPStatus: http.StatusOK},
	}
	if response.FetchMethod != FetchMethodBrowser || !reflect.DeepEqual(response.Attempts, wantAttempts) {
		t.Fatalf("Jina network fallback response = %+v", response)
	}
}

func TestFetchTreatsJinaAccessWarningAsBlocked(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusForbidden)
	}))
	t.Cleanup(target.Close)
	jina := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"data":{"url":"` + target.URL + `","content":"Security verification","warning":"Target returned a CAPTCHA challenge","httpStatus":200}}`))
	}))
	t.Cleanup(jina.Close)
	client, err := newClient(testWebAccessConfig(), dependencies{
		fetchHTTPClient: target.Client(), jinaHTTPClient: jina.Client(), jinaReaderBaseURL: jina.URL + "/",
		browserRenderer: browserRendererFunc(func(_ context.Context, targetURL *url.URL) (renderedPage, error) {
			return renderedPage{FinalURL: targetURL, HTTPStatus: http.StatusForbidden}, nil
		}),
	})
	if err != nil {
		t.Fatal(err)
	}

	response, err := client.Fetch(context.Background(), FetchRequest{URL: target.URL})
	if err != nil {
		t.Fatal(err)
	}
	wantAttempts := []FetchAttempt{
		{Method: FetchMethodDirectHTTP, Outcome: FetchAttemptAccessDenied, HTTPStatus: http.StatusForbidden},
		{Method: FetchMethodJinaReader, Outcome: FetchAttemptAccessDenied, HTTPStatus: http.StatusOK},
		{Method: FetchMethodBrowser, Outcome: FetchAttemptAccessDenied, HTTPStatus: http.StatusForbidden, Message: browserChallengeMessage},
	}
	if response.Status != FetchStatusBlocked || !reflect.DeepEqual(response.Attempts, wantAttempts) {
		t.Fatalf("Jina warning response = %+v, want attempts %+v", response, wantAttempts)
	}
}

func TestFetchReturnsActionableResponseWhenFallbackProvidersAreUnavailable(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "text/html")
		_, _ = writer.Write([]byte(`<html><body><div id="root"></div><script>1</script><script>2</script><script>3</script><script>4</script></body></html>`))
	}))
	t.Cleanup(target.Close)
	jina := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(jina.Close)
	renderer := browserRendererFunc(func(context.Context, *url.URL) (renderedPage, error) {
		return renderedPage{}, errors.New("Chrome executable was not found")
	})
	client, err := newClient(testWebAccessConfig(), dependencies{
		fetchHTTPClient: target.Client(), jinaHTTPClient: jina.Client(), jinaReaderBaseURL: jina.URL + "/", browserRenderer: renderer,
	})
	if err != nil {
		t.Fatal(err)
	}

	response, err := client.Fetch(context.Background(), FetchRequest{URL: target.URL})
	if err != nil {
		t.Fatalf("unavailable fallbacks should return recovery metadata, got %v", err)
	}
	wantAttempts := []FetchAttempt{
		{Method: FetchMethodDirectHTTP, Outcome: FetchAttemptJavaScriptRequired, HTTPStatus: http.StatusOK},
		{Method: FetchMethodJinaReader, Outcome: FetchAttemptProviderUnavailable, HTTPStatus: http.StatusServiceUnavailable},
		{Method: FetchMethodBrowser, Outcome: FetchAttemptProviderUnavailable},
	}
	if response.Status != FetchStatusProvidersUnavailable || response.RetryStrategy != FetchRetryWaitOrUseAlternateSource ||
		!reflect.DeepEqual(response.Attempts, wantAttempts) || response.SuggestedAction == "" {
		t.Fatalf("unavailable response = %+v, want attempts %+v", response, wantAttempts)
	}
}

func TestFetchExtractsReadableMarkdownWithAbsoluteLinks(t *testing.T) {
	articleText := strings.Repeat("This is substantial article text for readable extraction. ", 20)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = writer.Write([]byte(`<!doctype html><html><head><title>Fallback title</title></head><body><nav>Navigation noise</nav><article><h1>Readable title</h1><p>` + articleText + `</p><p><a href="/source">Primary source</a><a href="javascript:alert(1)">Unsafe link</a></p></article></body></html>`))
	}))
	t.Cleanup(server.Close)

	response, err := clientForFetchTest(t, server, testWebAccessConfig()).Fetch(context.Background(), FetchRequest{URL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(response.Content, "Readable title") || !strings.Contains(response.Content, "[Primary source]("+server.URL+"/source)") {
		t.Fatalf("unexpected markdown:\n%s", response.Content)
	}
	if strings.Contains(response.Content, "Navigation noise") {
		t.Fatalf("navigation noise leaked into readable content:\n%s", response.Content)
	}
	if strings.Contains(response.Content, "javascript:") {
		t.Fatalf("unsafe link scheme leaked into markdown:\n%s", response.Content)
	}
	if response.Warning != untrustedContentWarning || response.ContentType != "text/html" {
		t.Fatalf("unexpected fetch metadata: %+v", response)
	}
}

func TestFetchPaginatesByUnicodeCharacter(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = writer.Write([]byte("甲乙丙丁戊"))
	}))
	t.Cleanup(server.Close)

	response, err := clientForFetchTest(t, server, testWebAccessConfig()).Fetch(context.Background(), FetchRequest{
		URL: server.URL, StartIndex: 1, MaxChars: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if response.Content != "乙丙" || response.TotalChars != 5 || !response.Truncated || response.NextStartIndex == nil || *response.NextStartIndex != 3 {
		t.Fatalf("unexpected page: %+v", response)
	}
}

func TestFetchRejectsOversizedAndBinaryResponses(t *testing.T) {
	t.Run("oversized", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			writer.Header().Set("Content-Type", "text/plain")
			_, _ = writer.Write([]byte(strings.Repeat("x", 33)))
		}))
		t.Cleanup(server.Close)
		config := testWebAccessConfig()
		config.FetchMaxResponseBytes = 32
		_, err := clientForFetchTest(t, server, config).Fetch(context.Background(), FetchRequest{URL: server.URL})
		if err == nil || !strings.Contains(err.Error(), "32-byte") {
			t.Fatalf("oversized response error = %v", err)
		}
	})

	t.Run("binary", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			writer.Header().Set("Content-Type", "image/png")
			_, _ = writer.Write([]byte("not really a png"))
		}))
		t.Cleanup(server.Close)
		_, err := clientForFetchTest(t, server, testWebAccessConfig()).Fetch(context.Background(), FetchRequest{URL: server.URL})
		if err == nil || !strings.Contains(err.Error(), "unsupported") {
			t.Fatalf("binary response error = %v", err)
		}
	})
}

func TestFetchRejectsPrivateNetworkDestination(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	t.Cleanup(server.Close)
	jinaCalled := false
	browserCalled := false
	client, err := newClient(testWebAccessConfig(), dependencies{
		fetchHTTPClient: newPublicHTTPClient(),
		jinaHTTPClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			jinaCalled = true
			return nil, errors.New("Jina must not receive private URLs")
		})},
		browserRenderer: browserRendererFunc(func(context.Context, *url.URL) (renderedPage, error) {
			browserCalled = true
			return renderedPage{}, errors.New("browser must not receive private URLs")
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Fetch(context.Background(), FetchRequest{URL: server.URL})
	if err == nil || !strings.Contains(err.Error(), "blocked address") {
		t.Fatalf("private destination error = %v", err)
	}
	if jinaCalled || browserCalled {
		t.Fatalf("private destination escaped direct boundary: jina=%v browser=%v", jinaCalled, browserCalled)
	}
}

func TestFetchRejectsUnsafeURLForms(t *testing.T) {
	client, err := New(testWebAccessConfig())
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{"file:///etc/passwd", "https://user:secret@example.com/"} {
		if _, err := client.Fetch(context.Background(), FetchRequest{URL: target}); err == nil {
			t.Fatalf("Fetch(%q) unexpectedly succeeded", target)
		}
	}
}

func TestNewRejectsLimitsThatCouldBypassModelContextBounds(t *testing.T) {
	config := testWebAccessConfig()
	config.FetchMaxContentChars = absoluteMaxFetchContentChars + 1
	if _, err := New(config); err == nil || !strings.Contains(err.Error(), "content limit") {
		t.Fatalf("oversized content configuration error = %v", err)
	}
}
