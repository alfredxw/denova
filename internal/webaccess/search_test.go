package webaccess

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type fakeSearchProvider struct {
	name   string
	search func(context.Context, providerSearchRequest) ([]SearchResult, error)
}

type searchRoundTripFunc func(*http.Request) (*http.Response, error)

func (roundTrip searchRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return roundTrip(request)
}

func (provider fakeSearchProvider) Name() string { return provider.name }

func (provider fakeSearchProvider) Search(ctx context.Context, request providerSearchRequest) ([]SearchResult, error) {
	return provider.search(ctx, request)
}

func testWebAccessConfig() Config {
	return Config{
		SearchMaxResults:      10,
		SearchProviderTimeout: 10 * time.Second,
		FetchMaxResponseBytes: 1024 * 1024,
		FetchMaxContentChars:  256 * 1024,
	}
}

func TestSearchUsesConfiguredPrimaryBeforeFallbacks(t *testing.T) {
	fallbackCalled := false
	client, err := newClient(testWebAccessConfig(), dependencies{
		primaryProvider: fakeSearchProvider{name: ProviderSearXNG, search: func(_ context.Context, request providerSearchRequest) ([]SearchResult, error) {
			if request.Query != "denova" || request.MaxResults != 2 {
				t.Fatalf("unexpected primary request: %+v", request)
			}
			return []SearchResult{{Title: "Denova", URL: "https://example.com/denova"}}, nil
		}},
		fallbackProviders: []searchProvider{fakeSearchProvider{name: ProviderBing, search: func(context.Context, providerSearchRequest) ([]SearchResult, error) {
			fallbackCalled = true
			return nil, nil
		}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Search(context.Background(), SearchRequest{Query: " denova ", MaxResults: 2})
	if err != nil {
		t.Fatal(err)
	}
	if fallbackCalled {
		t.Fatal("free fallback ran even though configured SearXNG returned results")
	}
	if response.Provider != ProviderSearXNG || len(response.Results) != 1 {
		t.Fatalf("unexpected response: %+v", response)
	}
}

func TestSearchReportsInvalidConfiguredSearXNGEndpoint(t *testing.T) {
	config := testWebAccessConfig()
	config.SearXNGBaseURL = "://invalid"
	client, err := newClient(config, dependencies{fallbackProviders: []searchProvider{
		fakeSearchProvider{name: ProviderBing, search: func(context.Context, providerSearchRequest) ([]SearchResult, error) {
			return []SearchResult{{Title: "Denova query result", URL: "https://example.com/denova"}}, nil
		}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Search(context.Background(), SearchRequest{Query: "denova query"})
	if err != nil {
		t.Fatal(err)
	}
	if response.Provider != ProviderBing || len(response.Warnings) != 1 || !strings.Contains(response.Warnings[0], "SearXNG configuration") {
		t.Fatalf("fallback response should expose the ignored SearXNG configuration: %+v", response)
	}
}

func TestSearchCombinesConcurrentFreeProviders(t *testing.T) {
	duck := fakeSearchProvider{name: ProviderDuckDuckGo, search: func(ctx context.Context, _ providerSearchRequest) ([]SearchResult, error) {
		select {
		case <-time.After(20 * time.Millisecond):
			return []SearchResult{{Title: "Duck result", URL: "https://duck.example/result"}}, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}}
	bing := fakeSearchProvider{name: ProviderBing, search: func(context.Context, providerSearchRequest) ([]SearchResult, error) {
		return []SearchResult{{Title: "Bing query result", URL: "https://bing.example/result"}}, nil
	}}
	client, err := newClient(testWebAccessConfig(), dependencies{fallbackProviders: []searchProvider{duck, bing}})
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Search(context.Background(), SearchRequest{Query: "query"})
	if err != nil {
		t.Fatal(err)
	}
	if response.Provider != "duckduckgo+bing" {
		t.Fatalf("provider = %q, want combined free providers", response.Provider)
	}
	if len(response.Results) != 2 || response.Results[0].Title != "Duck result" || response.Results[1].Title != "Bing query result" {
		t.Fatalf("search should preserve results from both providers: %+v", response.Results)
	}
}

func TestSearchRejectsBingResultsUnrelatedToTheWholeQuery(t *testing.T) {
	client, err := newClient(testWebAccessConfig(), dependencies{fallbackProviders: []searchProvider{
		fakeSearchProvider{name: ProviderDuckDuckGo, search: func(context.Context, providerSearchRequest) ([]SearchResult, error) {
			return []SearchResult{{
				Title:   "2025 年热门网络小说排行榜",
				URL:     "https://books.example/ranking",
				Summary: "整理本年度热门网络小说与读者榜单。",
			}}, nil
		}},
		fakeSearchProvider{name: ProviderBing, search: func(context.Context, providerSearchRequest) ([]SearchResult, error) {
			return []SearchResult{
				{Title: "How to use Google Drive", URL: "https://support.google.com/drive/answer/2424384"},
				{Title: "Free up storage space", URL: "https://support.google.com/a/users/answer/14300711"},
			}, nil
		}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Search(context.Background(), SearchRequest{Query: "当前最热小说 2025 排行榜"})
	if err != nil {
		t.Fatal(err)
	}
	if response.Provider != ProviderDuckDuckGo || len(response.Results) != 1 {
		t.Fatalf("unrelated Bing results must not enter model context: %+v", response)
	}
}

func TestSearchFiltersIndividualUnrelatedBingResults(t *testing.T) {
	client, err := newClient(testWebAccessConfig(), dependencies{fallbackProviders: []searchProvider{
		fakeSearchProvider{name: ProviderBing, search: func(context.Context, providerSearchRequest) ([]SearchResult, error) {
			return []SearchResult{
				{Title: "穿越火线官方网站", URL: "https://cf.qq.com/", Summary: "腾讯游戏射击竞技专区"},
				{Title: "穿越修仙：后宫经营小说", URL: "https://books.example/novel", Summary: "一部穿越修仙题材的后宫小说"},
			}, nil
		}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Search(context.Background(), SearchRequest{Query: "穿越 修仙 后宫 小说"})
	if err != nil {
		t.Fatal(err)
	}
	if response.Provider != ProviderBing || len(response.Results) != 1 || response.Results[0].URL != "https://books.example/novel" {
		t.Fatalf("Bing relevance filtering should keep only the matching result: %+v", response)
	}
}

func TestSearchAcceptsBingResultWithoutEveryQueryQualifier(t *testing.T) {
	client, err := newClient(testWebAccessConfig(), dependencies{fallbackProviders: []searchProvider{
		fakeSearchProvider{name: ProviderBing, search: func(context.Context, providerSearchRequest) ([]SearchResult, error) {
			return []SearchResult{{
				Title:   "读者票选网络小说排行榜",
				URL:     "https://books.example/readers-choice",
				Summary: "近期热门网络小说榜单与读者评价",
			}}, nil
		}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Search(context.Background(), SearchRequest{Query: "当前最热小说 2025 排行榜"})
	if err != nil {
		t.Fatal(err)
	}
	if response.Provider != ProviderBing || len(response.Results) != 1 {
		t.Fatalf("a matching topic should not be rejected for omitting a year qualifier: %+v", response)
	}
}

func TestSearchRejectsBingResultMatchingOnlyYearQualifier(t *testing.T) {
	client, err := newClient(testWebAccessConfig(), dependencies{fallbackProviders: []searchProvider{
		fakeSearchProvider{name: ProviderBing, search: func(context.Context, providerSearchRequest) ([]SearchResult, error) {
			return []SearchResult{{
				Title:   "Is Gout Hereditary? A 2025 Study",
				URL:     "https://health.example.com/gout-study-2025",
				Summary: "Research into genetic risk factors.",
			}}, nil
		}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Search(context.Background(), SearchRequest{Query: "当前最热小说 2025 排行榜"})
	if err != nil {
		t.Fatal(err)
	}
	if response.Status != SearchStatusNoResults || len(response.Results) != 0 {
		t.Fatalf("a year-only match must not validate an unrelated topic: %+v", response)
	}
}

func TestSearchTreatsFilteredBingResultsAsEmptyInsteadOfProviderFailure(t *testing.T) {
	client, err := newClient(testWebAccessConfig(), dependencies{fallbackProviders: []searchProvider{
		fakeSearchProvider{name: ProviderBing, search: func(context.Context, providerSearchRequest) ([]SearchResult, error) {
			return []SearchResult{{
				Title:   "穿越火线官方网站",
				URL:     "https://cf.qq.com/",
				Summary: "腾讯游戏射击竞技专区",
			}}, nil
		}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Search(context.Background(), SearchRequest{Query: "穿越 修仙 后宫 小说"})
	if err != nil {
		t.Fatalf("reachable Bing with irrelevant results should be an empty search, not a provider failure: %v", err)
	}
	if len(response.Results) != 0 || len(response.Warnings) != 1 || !strings.Contains(response.Warnings[0], "filtered 1") {
		t.Fatalf("empty response should explain the relevance filter: %+v", response)
	}
}

func TestSearchRejectsBingDomainNoiseForSiteQuery(t *testing.T) {
	client, err := newClient(testWebAccessConfig(), dependencies{fallbackProviders: []searchProvider{
		fakeSearchProvider{name: ProviderDuckDuckGo, search: func(context.Context, providerSearchRequest) ([]SearchResult, error) {
			return []SearchResult{{Title: "起点月票榜", URL: "https://www.qidian.com/rank/yuepiao/"}}, nil
		}},
		fakeSearchProvider{name: ProviderBing, search: func(context.Context, providerSearchRequest) ([]SearchResult, error) {
			return []SearchResult{{
				Title:   "Is Gout Hereditary?",
				URL:     "https://health.example.com/gout-study-2025",
				Summary: "A 2025 study about genetic risk factors.",
			}}, nil
		}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Search(context.Background(), SearchRequest{Query: "site:qidian.com 月票榜 2025"})
	if err != nil {
		t.Fatal(err)
	}
	if response.Provider != ProviderDuckDuckGo || len(response.Results) != 1 {
		t.Fatalf("generic domain and year tokens must not validate unrelated Bing results: %+v", response)
	}
}

func TestSearchDoesNotMatchShortEnglishQueryTermsInsideOtherWords(t *testing.T) {
	client, err := newClient(testWebAccessConfig(), dependencies{fallbackProviders: []searchProvider{
		fakeSearchProvider{name: ProviderDuckDuckGo, search: func(context.Context, providerSearchRequest) ([]SearchResult, error) {
			return []SearchResult{{Title: "The Go programming language", URL: "https://go.dev/"}}, nil
		}},
		fakeSearchProvider{name: ProviderBing, search: func(context.Context, providerSearchRequest) ([]SearchResult, error) {
			return []SearchResult{{
				Title:   "Google",
				URL:     "https://www.google.com/",
				Summary: "Search information in many languages.",
			}}, nil
		}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Search(context.Background(), SearchRequest{Query: "Go programming language official website"})
	if err != nil {
		t.Fatal(err)
	}
	if response.Provider != ProviderDuckDuckGo || len(response.Results) != 1 {
		t.Fatalf("the term go must not match inside google: %+v", response)
	}
}

func TestSearchReturnsPartialResultsWhenOneFreeProviderTimesOut(t *testing.T) {
	config := testWebAccessConfig()
	config.SearchProviderTimeout = 25 * time.Millisecond
	client, err := newClient(config, dependencies{fallbackProviders: []searchProvider{
		fakeSearchProvider{name: ProviderDuckDuckGo, search: func(ctx context.Context, _ providerSearchRequest) ([]SearchResult, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		}},
		fakeSearchProvider{name: ProviderBing, search: func(context.Context, providerSearchRequest) ([]SearchResult, error) {
			return []SearchResult{{Title: "Query result", URL: "https://example.com/query"}}, nil
		}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Search(context.Background(), SearchRequest{Query: "query"})
	if err != nil {
		t.Fatal(err)
	}
	if response.Provider != ProviderBing || len(response.Results) != 1 {
		t.Fatalf("expected usable Bing result after DuckDuckGo timeout: %+v", response)
	}
	if len(response.Warnings) != 1 || !strings.Contains(response.Warnings[0], ProviderDuckDuckGo) {
		t.Fatalf("provider timeout should remain visible to the model: %+v", response.Warnings)
	}
}

func TestSearchFallsBackAfterPrimaryFailureAndSkipsEmptyResult(t *testing.T) {
	client, err := newClient(testWebAccessConfig(), dependencies{
		primaryProvider: fakeSearchProvider{name: ProviderSearXNG, search: func(context.Context, providerSearchRequest) ([]SearchResult, error) {
			return nil, errors.New("instance unavailable")
		}},
		fallbackProviders: []searchProvider{
			fakeSearchProvider{name: ProviderDuckDuckGo, search: func(context.Context, providerSearchRequest) ([]SearchResult, error) { return nil, nil }},
			fakeSearchProvider{name: ProviderBing, search: func(context.Context, providerSearchRequest) ([]SearchResult, error) {
				return []SearchResult{{Title: "Query fallback", URL: "https://example.com/fallback"}}, nil
			}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Search(context.Background(), SearchRequest{Query: "query"})
	if err != nil {
		t.Fatal(err)
	}
	if response.Provider != ProviderBing || len(response.Results) != 1 {
		t.Fatalf("unexpected fallback response: %+v", response)
	}
}

func TestSearXNGProviderUsesJSONSearchEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/instance/search" {
			t.Errorf("path = %q", request.URL.Path)
		}
		if request.URL.Query().Get("q") != "release notes" || request.URL.Query().Get("format") != "json" || request.URL.Query().Get("time_range") != "month" {
			t.Errorf("unexpected query: %s", request.URL.RawQuery)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"results":[{"url":"https://example.com/release","title":"Release","content":"Summary","publishedDate":"2026-07-01"}]}`))
	}))
	t.Cleanup(server.Close)

	provider, err := newSearXNGProvider(server.URL+"/instance", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	results, err := provider.Search(context.Background(), providerSearchRequest{Query: "release notes", TimeRange: "month", MaxResults: 5})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Provider != ProviderSearXNG || results[0].PublishedAt != "2026-07-01" {
		t.Fatalf("unexpected SearXNG results: %+v", results)
	}
}

func TestSearchFallsBackToSearXNGHTMLWhenJSONIsForbidden(t *testing.T) {
	var fallbackCalled bool
	var requests []string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/search" {
			t.Errorf("path = %q", request.URL.Path)
		}
		if err := request.ParseForm(); err != nil {
			t.Errorf("parse SearXNG form: %v", err)
			writer.WriteHeader(http.StatusBadRequest)
			return
		}
		requests = append(requests, request.Method+":"+request.Form.Get("format"))
		if request.Form.Get("q") != "denova" {
			t.Errorf("query = %q", request.Form.Get("q"))
		}
		if request.Form.Get("format") == "json" {
			writer.WriteHeader(http.StatusForbidden)
			return
		}
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = writer.Write([]byte(`<div id="urls"><article class="result result-default category-general"><a class="url_header" href="https://example.com/denova">example.com</a><h3><a href="https://example.com/denova">Denova</a></h3><p class="content">Creator workspace</p></article></div>`))
	}))
	t.Cleanup(server.Close)

	config := testWebAccessConfig()
	config.SearXNGBaseURL = server.URL
	client, err := newClient(config, dependencies{
		searchHTTPClient: server.Client(),
		fallbackProviders: []searchProvider{fakeSearchProvider{name: ProviderDuckDuckGo, search: func(context.Context, providerSearchRequest) ([]SearchResult, error) {
			fallbackCalled = true
			return nil, nil
		}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Search(context.Background(), SearchRequest{Query: "denova"})
	if err != nil {
		t.Fatal(err)
	}
	if fallbackCalled {
		t.Fatal("free fallback ran even though SearXNG HTML returned a result")
	}
	if len(requests) != 2 || requests[0] != "GET:json" || requests[1] != "POST:" {
		t.Fatalf("SearXNG requests = %v, want JSON GET followed by HTML POST", requests)
	}
	if response.Provider != ProviderSearXNG || len(response.Results) != 1 || response.Results[0].Summary != "Creator workspace" {
		t.Fatalf("unexpected SearXNG HTML response: %+v", response)
	}
}

func TestSearchAcceptsSearXNGHTMLReturnedByJSONEndpoint(t *testing.T) {
	var requestCount int
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requestCount++
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = writer.Write([]byte(`<html><head><meta name="generator" content="searxng/2026.7.22"></head><body><div id="urls"><article class="result result-default"><h3><a href="https://go.dev/">The Go Programming Language</a></h3><p class="content">Build simple, secure, scalable systems with Go.</p></article></div></body></html>`))
	}))
	t.Cleanup(server.Close)

	config := testWebAccessConfig()
	config.SearXNGBaseURL = server.URL
	client, err := newClient(config, dependencies{searchHTTPClient: server.Client(), fallbackProviders: []searchProvider{}})
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Search(context.Background(), SearchRequest{Query: "Go programming language"})
	if err != nil {
		t.Fatal(err)
	}
	if requestCount != 1 {
		t.Fatalf("SearXNG HTML returned to the JSON request should be reused, requests = %d", requestCount)
	}
	if response.Provider != ProviderSearXNG || len(response.Results) != 1 || response.Results[0].URL != "https://go.dev/" {
		t.Fatalf("unexpected SearXNG HTML response: %+v", response)
	}
}

func TestSearchExplainsSearXNGHTMLAccessChallengeBeforeUsingFallback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = writer.Write([]byte(`<!doctype html><html><head><title>Making sure you're not a bot!</title></head><body><script src="/.within.website/x/cmd/anubis/static/js/main.mjs"></script></body></html>`))
	}))
	t.Cleanup(server.Close)

	config := testWebAccessConfig()
	config.SearXNGBaseURL = server.URL
	client, err := newClient(config, dependencies{
		searchHTTPClient: server.Client(),
		fallbackProviders: []searchProvider{fakeSearchProvider{name: ProviderBing, search: func(context.Context, providerSearchRequest) ([]SearchResult, error) {
			return []SearchResult{{Title: "Denova query result", URL: "https://example.com/denova"}}, nil
		}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Search(context.Background(), SearchRequest{Query: "denova query"})
	if err != nil {
		t.Fatal(err)
	}
	warnings := strings.Join(response.Warnings, "\n")
	if response.Provider != ProviderBing || !strings.Contains(warnings, ProviderSearXNG) || !strings.Contains(warnings, "access challenge") {
		t.Fatalf("fallback response should explain why configured SearXNG was unusable: %+v", response)
	}
}

func TestSearchUsesChineseBingMarketForChineseQuery(t *testing.T) {
	var market string
	var language string
	httpClient := &http.Client{Transport: searchRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		market = request.URL.Query().Get("mkt")
		language = request.URL.Query().Get("setlang")
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body: io.NopCloser(strings.NewReader(
				`<li class="b_algo"><h2><a href="https://books.example/hot">热门小说排行榜</a></h2><div class="b_caption"><p>热门小说榜单</p></div></li>`,
			)),
			Request: request,
		}, nil
	})}
	client, err := newClient(testWebAccessConfig(), dependencies{
		fallbackProviders: []searchProvider{newBingProvider(httpClient)},
	})
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Search(context.Background(), SearchRequest{Query: "热门小说排行榜"})
	if err != nil {
		t.Fatal(err)
	}
	if market != "zh-CN" || language != "zh-Hans" {
		t.Fatalf("Bing locale = %q/%q, want zh-CN/zh-Hans", market, language)
	}
	if len(response.Results) != 1 || response.Results[0].URL != "https://books.example/hot" {
		t.Fatalf("unexpected Bing response: %+v", response)
	}
}

func TestSearchUsesBingHTMLBeforeRSSFallback(t *testing.T) {
	var requestedFormats []string
	var requestedAccepts []string
	httpClient := &http.Client{Transport: searchRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		format := request.URL.Query().Get("format")
		requestedFormats = append(requestedFormats, format)
		requestedAccepts = append(requestedAccepts, request.Header.Get("Accept"))
		body := `<li class="b_algo"><h2><a href="https://go.dev/">Go programming language</a></h2><div class="b_caption"><p>The official Go programming language website.</p></div></li>`
		if format == "rss" {
			body = `<?xml version="1.0"?><rss><channel><item><title>GO Transit</title><link>https://www.gotransit.com/</link><description>Train schedules</description></item></channel></rss>`
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(body)),
			Request:    request,
		}, nil
	})}
	client, err := newClient(testWebAccessConfig(), dependencies{
		fallbackProviders: []searchProvider{newBingProvider(httpClient)},
	})
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Search(context.Background(), SearchRequest{Query: "Go programming language official website"})
	if err != nil {
		t.Fatal(err)
	}
	if len(requestedFormats) != 1 || requestedFormats[0] != "" {
		t.Fatalf("Bing should use its full HTML search before RSS fallback, formats = %v", requestedFormats)
	}
	if len(requestedAccepts) != 1 || !strings.HasPrefix(requestedAccepts[0], "text/html") {
		t.Fatalf("Bing HTML request should prefer HTML content, Accept = %v", requestedAccepts)
	}
	if len(response.Results) != 1 || response.Results[0].URL != "https://go.dev/" {
		t.Fatalf("unexpected Bing HTML result: %+v", response)
	}
}

func TestSearchHTMLParsersReturnSourceLinks(t *testing.T) {
	duckHTML := `<div class="result"><a class="result__a" href="//duckduckgo.com/l/?uddg=https%3A%2F%2Fexample.com%2Fduck">Duck title</a><div class="result__snippet">Duck summary</div></div>`
	duck := parseDuckDuckGoResults(duckHTML)
	if len(duck) != 1 || duck[0].URL != "https://example.com/duck" || duck[0].Summary != "Duck summary" {
		t.Fatalf("unexpected DuckDuckGo parse: %+v", duck)
	}

	bingRSS := `<?xml version="1.0"?><rss><channel><item><title>Bing title</title><link>https://example.com/bing</link><description>Bing summary</description></item></channel></rss>`
	bing := parseBingResults(bingRSS)
	if len(bing) != 1 || bing[0].URL != "https://example.com/bing" || bing[0].Summary != "Bing summary" {
		t.Fatalf("unexpected Bing parse: %+v", bing)
	}
}

func TestSearchReportsAllProviderFailures(t *testing.T) {
	failed := func(name string) searchProvider {
		return fakeSearchProvider{name: name, search: func(context.Context, providerSearchRequest) ([]SearchResult, error) {
			return nil, fmt.Errorf("%s unavailable", name)
		}}
	}
	client, err := newClient(testWebAccessConfig(), dependencies{fallbackProviders: []searchProvider{failed(ProviderDuckDuckGo), failed(ProviderBing)}})
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Search(context.Background(), SearchRequest{Query: "query"})
	if err != nil {
		t.Fatal(err)
	}
	warnings := strings.Join(response.Warnings, "\n")
	if response.Status != SearchStatusProvidersUnavailable || !strings.Contains(warnings, ProviderDuckDuckGo) || !strings.Contains(warnings, ProviderBing) {
		t.Fatalf("unexpected all-provider diagnostics: %+v", response)
	}
}

func TestSearchReturnsActionableResponseWhenAllProvidersFail(t *testing.T) {
	failed := func(name string) searchProvider {
		return fakeSearchProvider{name: name, search: func(context.Context, providerSearchRequest) ([]SearchResult, error) {
			return nil, fmt.Errorf("%s unavailable", name)
		}}
	}
	client, err := newClient(testWebAccessConfig(), dependencies{fallbackProviders: []searchProvider{failed(ProviderDuckDuckGo), failed(ProviderBing)}})
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Search(context.Background(), SearchRequest{Query: "query"})
	if err != nil {
		t.Fatalf("provider availability should be a structured search response: %v", err)
	}
	if response.Status != SearchStatusProvidersUnavailable || response.RetryStrategy != SearchRetryWaitOrReconfigure {
		t.Fatalf("unexpected recovery metadata: %+v", response)
	}
	if len(response.Warnings) != 2 || !strings.Contains(response.SuggestedAction, "Do not immediately repeat") || !strings.Contains(response.SuggestedAction, "不要立即重复") {
		t.Fatalf("provider failure should tell the Agent how to recover: %+v", response)
	}
}

func TestSearchKeepsProviderFailuresWhenAnotherProviderReturnsNoResults(t *testing.T) {
	client, err := newClient(testWebAccessConfig(), dependencies{fallbackProviders: []searchProvider{
		fakeSearchProvider{name: ProviderDuckDuckGo, search: func(context.Context, providerSearchRequest) ([]SearchResult, error) {
			return nil, nil
		}},
		fakeSearchProvider{name: ProviderBing, search: func(context.Context, providerSearchRequest) ([]SearchResult, error) {
			return nil, errors.New("endpoint unavailable")
		}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Search(context.Background(), SearchRequest{Query: "query"})
	if err != nil {
		t.Fatal(err)
	}
	if len(response.Results) != 0 || len(response.Warnings) != 1 || !strings.Contains(response.Warnings[0], ProviderBing) {
		t.Fatalf("empty search should preserve partial provider diagnostics: %+v", response)
	}
}

func TestSearchBoundsModelVisibleProviderFields(t *testing.T) {
	provider := fakeSearchProvider{name: ProviderDuckDuckGo, search: func(context.Context, providerSearchRequest) ([]SearchResult, error) {
		return []SearchResult{{
			Title:   strings.Repeat("T", maxSearchTitleChars+100),
			URL:     "https://example.com/result",
			Summary: strings.Repeat("S", maxSearchSummaryChars+100),
		}}, nil
	}}
	client, err := newClient(testWebAccessConfig(), dependencies{fallbackProviders: []searchProvider{provider}})
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Search(context.Background(), SearchRequest{Query: "query"})
	if err != nil {
		t.Fatal(err)
	}
	if got := len([]rune(response.Results[0].Title)); got != maxSearchTitleChars {
		t.Fatalf("bounded title chars = %d", got)
	}
	if got := len([]rune(response.Results[0].Summary)); got != maxSearchSummaryChars {
		t.Fatalf("bounded summary chars = %d", got)
	}
}
