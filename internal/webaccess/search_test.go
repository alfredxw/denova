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
	_, err = client.Search(context.Background(), SearchRequest{Query: "query"})
	if err == nil || !strings.Contains(err.Error(), ProviderDuckDuckGo) || !strings.Contains(err.Error(), ProviderBing) {
		t.Fatalf("unexpected all-provider error: %v", err)
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
