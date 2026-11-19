package webaccess

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"denova/config"
)

type searchTestTransport func(*http.Request) (*http.Response, error)

func (transport searchTestTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	return transport(request)
}

// Preserve the production endpoint selection while sending real HTTP to a
// local server. No service credentials or external network are needed.
func searchTestHTTPClient(server *httptest.Server) *http.Client {
	target, _ := url.Parse(server.URL)
	return &http.Client{Transport: searchTestTransport(func(request *http.Request) (*http.Response, error) {
		copy := request.Clone(request.Context())
		copy.URL.Scheme, copy.URL.Host = target.Scheme, target.Host
		return http.DefaultTransport.RoundTrip(copy)
	})}
}

func searchTestConfig(provider string) Config {
	value := ConfigFromSettings(config.DefaultWebAccessConfig())
	value.SearchProvider = provider
	value.DeepSeek = &config.SearchServiceSettings{APIKey: "test-secret"}
	value.Perplexity = &config.SearchServiceSettings{APIKey: "test-secret", Mode: "fast"}
	value.Tavily = &config.SearchServiceSettings{APIKey: "test-secret", Mode: "advanced"}
	return value
}

func TestHostedSearchHTTPContracts(t *testing.T) {
	for _, provider := range []string{ProviderDeepSeek, ProviderPerplexity, ProviderTavily} {
		t.Run(provider, func(t *testing.T) {
			requests := make(chan map[string]any, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				path := "/search"
				if provider == ProviderDeepSeek {
					path = "/v1/tools/search"
				}
				host := map[string]string{ProviderDeepSeek: "api.deepseek.com", ProviderPerplexity: "api.perplexity.ai", ProviderTavily: "api.tavily.com"}[provider]
				if r.Host != host || r.Method != "POST" || r.URL.Path != path || r.Header.Get("Authorization") != "Bearer test-secret" || r.Header.Get("Content-Type") != "application/json" {
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
				}
				var payload map[string]any
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					t.Error(err)
				}
				requests <- payload
				io.WriteString(w, `{"results":[{"title":" Source ","url":"https://example.com/source","summary":"Deep summary","snippet":"Short snippet","content":"Tavily passage","date":"2026-10-18","published_date":"2026-10-17"},{"title":"Duplicate","url":"https://example.com/source#section"},{"title":"Invalid link","url":"javascript:alert(1)"},{"title":"Other","url":"https://example.com/other","snippet":"Fallback snippet","date":null}]}`)
			}))
			defer server.Close()
			client, err := newClient(searchTestConfig(provider), dependencies{searchHTTPClient: searchTestHTTPClient(server)})
			if err != nil {
				t.Fatal(err)
			}
			response, err := client.Search(context.Background(), SearchRequest{Query: "创作工具", TimeRange: "week", MaxResults: 2})
			if err != nil {
				t.Fatal(err)
			}
			payload := <-requests
			expected := map[string]any{"query": "创作工具"}
			summary, date := "Short snippet", "2026-10-18"
			switch provider {
			case ProviderDeepSeek:
				expected["query"] = "创作工具 after:" + time.Now().UTC().AddDate(0, 0, -7).Format("2006-01-02")
				expected["count"], expected["need_content"] = float64(2), false
				summary = "Deep summary"
			case ProviderPerplexity:
				expected["max_results"], expected["search_type"], expected["search_recency_filter"] = float64(2), "fast", "week"
				expected["max_tokens"], expected["max_tokens_per_page"] = float64(2048), float64(1024)
			case ProviderTavily:
				expected["max_results"], expected["search_depth"], expected["time_range"] = float64(2), "advanced", "week"
				expected["auto_parameters"], expected["include_answer"], expected["include_raw_content"], expected["include_images"], expected["include_published_date"] = false, false, false, false, true
				summary, date = "Tavily passage", "2026-10-17"
			}
			if !reflect.DeepEqual(payload, expected) {
				t.Fatalf("payload = %#v, want %#v", payload, expected)
			}
			if response.Provider != provider || response.Status != SearchStatusSuccess || len(response.Results) != 2 || response.Results[0] != (SearchResult{Title: "Source", URL: "https://example.com/source", Summary: summary, Provider: provider, PublishedAt: date}) {
				t.Fatalf("unexpected normalized response: %+v", response)
			}
			if provider == ProviderDeepSeek && response.Results[1].Summary != "Fallback snippet" {
				t.Fatal("missing snippet fallback")
			}
		})
	}
}

type countingSearchFallback struct{ calls atomic.Int32 }

func (provider *countingSearchFallback) Name() string { return ProviderDuckDuckGo }
func (provider *countingSearchFallback) Search(context.Context, providerSearchRequest) ([]SearchResult, error) {
	provider.calls.Add(1)
	return []SearchResult{{Title: "Free result", URL: "https://example.org"}}, nil
}

func TestHostedSearchFailuresAndOptInFallback(t *testing.T) {
	for _, test := range []struct {
		name                 string
		status               int
		body, fallback, code string
		useFallback          bool
	}{
		{"empty", 200, `{"results":[]}`, "builtin", "", false},
		{"malformed", 200, `{"error":"private test-secret"}`, "builtin", "invalid_response", false},
		{"invalid", 400, "private test-secret", "builtin", "invalid_request", false},
		{"key", 401, "private test-secret", "builtin", "authentication", false},
		{"balance", 402, "private test-secret", "builtin", "quota", false},
		{"quota", 432, "private test-secret", "builtin", "quota", false},
		{"rate default", 429, "private test-secret", "", "rate_limit", false},
		{"rate opt-in", 429, "private test-secret", "builtin", "", true},
		{"outage default", 503, "private test-secret", "", "unavailable", false},
		{"outage opt-in", 503, "private test-secret", "builtin", "", true},
		{"redirect", 302, "private test-secret", "builtin", "invalid_response", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Location", "/credential-leak")
				w.WriteHeader(test.status)
				io.WriteString(w, test.body)
			}))
			defer server.Close()
			value := searchTestConfig(ProviderDeepSeek)
			value.SearchFallback = test.fallback
			fallback := &countingSearchFallback{}
			client, err := newClient(value, dependencies{searchHTTPClient: searchTestHTTPClient(server), fallbackProviders: []searchProvider{fallback}})
			if err != nil {
				t.Fatal(err)
			}
			response, err := client.Search(context.Background(), SearchRequest{Query: "query"})
			if test.code != "" {
				if err == nil || SearchErrorCode(err) != test.code || strings.Contains(err.Error(), "test-secret") {
					t.Fatalf("unexpected error: %v", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if calls.Load() != 1 {
				t.Fatal("search was retried or redirected")
			}
			if (fallback.calls.Load() > 0) != test.useFallback {
				t.Fatal("incorrect fallback policy")
			}
			if test.useFallback && (response.Provider != ProviderDuckDuckGo || len(response.Warnings) == 0) {
				t.Fatalf("fallback source/diagnostic lost: %+v", response)
			}
			if test.name == "empty" && (response.Status != SearchStatusNoResults || response.Provider != ProviderDeepSeek) {
				t.Fatalf("empty response lost: %+v", response)
			}
		})
	}
}

func TestSearchBoundsCancellationAndIncompleteSetup(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		io.Copy(io.Discard, r.Body)
		if r.URL.Query().Get("format") != "" {
			io.WriteString(w, `{"results":[]}`)
			return
		}
		<-r.Context().Done()
	}))
	defer server.Close()
	fallback := &countingSearchFallback{}
	value := searchTestConfig(ProviderDeepSeek)
	value.SearchFallback = "builtin"
	value.SearchProviderTimeout = 20 * time.Millisecond
	client, err := newClient(value, dependencies{searchHTTPClient: searchTestHTTPClient(server), fallbackProviders: []searchProvider{fallback}})
	if err != nil {
		t.Fatal(err)
	}
	for _, request := range []SearchRequest{{Query: strings.Repeat("界", 129)}, {Query: strings.Repeat("界", 112), TimeRange: "month"}} {
		if _, err := client.Search(context.Background(), request); SearchErrorCode(err) != "query_too_long" {
			t.Fatalf("query was truncated or submitted: %v", err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := client.Search(ctx, SearchRequest{Query: "cancelled"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation lost: %v", err)
	}
	if calls.Load() != 0 || fallback.calls.Load() != 0 {
		t.Fatal("validation/cancellation triggered requests")
	}
	if response, err := client.Search(context.Background(), SearchRequest{Query: strings.Repeat("界", 128)}); err != nil || response.Provider != ProviderDuckDuckGo {
		t.Fatalf("timeout did not fall back: %+v %v", response, err)
	}
	parent, stop := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer stop()
	if _, err := client.Search(parent, SearchRequest{Query: "cancel in flight"}); !errors.Is(err, context.DeadlineExceeded) || fallback.calls.Load() != 1 {
		t.Fatalf("parent cancellation triggered fallback: %v", err)
	}

	value.DeepSeek = nil
	client, err = newClient(value, dependencies{})
	if err != nil || client.fetchHTTPClient == nil || client.browserRenderer == nil {
		t.Fatal("incomplete search setup blocked Fetch construction")
	}
	if _, err := client.Search(context.Background(), SearchRequest{Query: "query"}); SearchErrorCode(err) != "configuration" {
		t.Fatalf("missing key: %v", err)
	}

	legacy := ConfigFromSettings(config.DefaultWebAccessConfig())
	legacy.SearXNGBaseURL = server.URL
	fallback = &countingSearchFallback{}
	client, err = newClient(legacy, dependencies{fallbackProviders: []searchProvider{fallback}})
	if err != nil {
		t.Fatal(err)
	}
	if response, err := client.Search(context.Background(), SearchRequest{Query: "query"}); err != nil || response.Provider != ProviderDuckDuckGo {
		t.Fatalf("legacy empty fallback changed: %+v %v", response, err)
	}
	legacy.SearXNGBaseURL = ":invalid:"
	client, err = newClient(legacy, dependencies{fallbackProviders: []searchProvider{fallback}})
	if err != nil {
		t.Fatal(err)
	}
	if response, err := client.Search(context.Background(), SearchRequest{Query: "query"}); err != nil || response.Provider != ProviderDuckDuckGo || len(response.Warnings) == 0 {
		t.Fatalf("legacy invalid endpoint fallback changed: %+v %v", response, err)
	}
}

func TestKeenableSearchKeylessAndKeyed(t *testing.T) {
	for _, test := range []struct {
		name, key, path string
	}{
		{"keyless", "", "/v1/search/public"},
		{"keyed", " test-secret ", "/v1/search"},
	} {
		t.Run(test.name, func(t *testing.T) {
			requests := make(chan map[string]any, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				wantKey := strings.TrimSpace(test.key)
				if r.Host != "api.keenable.ai" || r.Method != "POST" || r.URL.Path != test.path || r.Header.Get("X-Keenable-Title") != "Denova" || r.Header.Get("X-API-Key") != wantKey || r.Header.Get("Authorization") != "" {
					t.Errorf("unexpected request: %s %s%s key=%q", r.Method, r.Host, r.URL.Path, r.Header.Get("X-API-Key"))
				}
				var payload map[string]any
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					t.Error(err)
				}
				requests <- payload
				io.WriteString(w, `{"results":[{"title":" Source ","url":"https://example.com/source","description":"Page description","snippet":"Matched passage","published_at":"2026-10-18T09:00:00Z","acquired_at":"2026-11-01T00:00:00Z"},{"title":"Other","url":"https://example.com/other","description":"Only a description"}]}`)
			}))
			defer server.Close()
			value := searchTestConfig(ProviderKeenable)
			if test.key != "" {
				value.Keenable = &config.SearchServiceSettings{APIKey: test.key}
			}
			client, err := newClient(value, dependencies{searchHTTPClient: searchTestHTTPClient(server)})
			if err != nil {
				t.Fatal(err)
			}
			response, err := client.Search(context.Background(), SearchRequest{Query: "creative tools", TimeRange: "week", MaxResults: 2})
			if err != nil {
				t.Fatal(err)
			}
			payload := <-requests
			if expected := map[string]any{"query": "creative tools", "max_results": float64(2), "published_after": "7d"}; !reflect.DeepEqual(payload, expected) {
				t.Fatalf("payload = %#v, want %#v", payload, expected)
			}
			if response.Provider != ProviderKeenable || len(response.Results) != 2 || response.Results[0] != (SearchResult{Title: "Source", URL: "https://example.com/source", Summary: "Matched passage", Provider: ProviderKeenable, PublishedAt: "2026-10-18T09:00:00Z"}) || response.Results[1].Summary != "Only a description" {
				t.Fatalf("unexpected normalized response: %+v", response)
			}
		})
	}
}
