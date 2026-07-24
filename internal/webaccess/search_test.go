package webaccess

import (
	"context"
	"errors"
	"fmt"
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

func (provider fakeSearchProvider) Name() string { return provider.name }

func (provider fakeSearchProvider) Search(ctx context.Context, request providerSearchRequest) ([]SearchResult, error) {
	return provider.search(ctx, request)
}

func testWebAccessConfig() Config {
	return Config{
		SearchMaxResults:      10,
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

func TestSearchRacesFallbacksAndCancelsLoser(t *testing.T) {
	slowCancelled := make(chan struct{})
	slow := fakeSearchProvider{name: ProviderDuckDuckGo, search: func(ctx context.Context, _ providerSearchRequest) ([]SearchResult, error) {
		<-ctx.Done()
		close(slowCancelled)
		return nil, ctx.Err()
	}}
	fast := fakeSearchProvider{name: ProviderBing, search: func(context.Context, providerSearchRequest) ([]SearchResult, error) {
		return []SearchResult{{Title: "Result", URL: "https://example.com/result"}}, nil
	}}
	client, err := newClient(testWebAccessConfig(), dependencies{fallbackProviders: []searchProvider{slow, fast}})
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Search(context.Background(), SearchRequest{Query: "query"})
	if err != nil {
		t.Fatal(err)
	}
	if response.Provider != ProviderBing {
		t.Fatalf("provider = %q, want %q", response.Provider, ProviderBing)
	}
	select {
	case <-slowCancelled:
	case <-time.After(250 * time.Millisecond):
		t.Fatal("winning fallback did not cancel the slower provider")
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
				return []SearchResult{{Title: "Fallback", URL: "https://example.com/fallback"}}, nil
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

func TestSearchBoundsModelVisibleProviderFields(t *testing.T) {
	provider := fakeSearchProvider{name: ProviderBing, search: func(context.Context, providerSearchRequest) ([]SearchResult, error) {
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
