package webaccess

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func clientForFetchTest(t *testing.T, server *httptest.Server, config Config) *Client {
	t.Helper()
	client, err := newClient(config, dependencies{fetchHTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	return client
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
	client, err := New(testWebAccessConfig())
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Fetch(context.Background(), FetchRequest{URL: server.URL})
	if err == nil || !strings.Contains(err.Error(), "blocked address") {
		t.Fatalf("private destination error = %v", err)
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
