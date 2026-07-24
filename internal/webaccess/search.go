package webaccess

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/url"
	"strings"
)

const (
	maxSearchQueryChars   = 4096
	maxSearchTitleChars   = 500
	maxSearchSummaryChars = 2000
	maxWebURLChars        = 8192
)

type searchProvider interface {
	Name() string
	Search(context.Context, providerSearchRequest) ([]SearchResult, error)
}

type providerSearchRequest struct {
	Query      string
	TimeRange  string
	MaxResults int
}

type searchOutcome struct {
	provider string
	results  []SearchResult
	err      error
}

func (client *Client) Search(ctx context.Context, request SearchRequest) (SearchResponse, error) {
	query := strings.TrimSpace(request.Query)
	if query == "" {
		return SearchResponse{}, fmt.Errorf("web search query is required")
	}
	if len([]rune(query)) > maxSearchQueryChars {
		return SearchResponse{}, fmt.Errorf("web search query exceeds %d-character safety limit", maxSearchQueryChars)
	}
	timeRange, err := normalizeTimeRange(request.TimeRange)
	if err != nil {
		return SearchResponse{}, err
	}
	maxResults := request.MaxResults
	if maxResults <= 0 || maxResults > client.config.SearchMaxResults {
		maxResults = client.config.SearchMaxResults
	}
	providerRequest := providerSearchRequest{Query: query, TimeRange: timeRange, MaxResults: maxResults}

	var failures []error
	hadReachableProvider := false
	if client.primaryProvider != nil {
		outcome := runSearchProvider(ctx, client.primaryProvider, providerRequest)
		if outcome.err == nil {
			hadReachableProvider = true
			if len(outcome.results) > 0 {
				return successfulSearchResponse(query, outcome.provider, outcome.results, maxResults), nil
			}
			log.Printf("[webaccess] search provider=%s returned no usable results; trying free fallbacks", outcome.provider)
		} else {
			failures = append(failures, outcome.err)
			log.Printf("[webaccess] search provider=%s failed; trying free fallbacks: %v", outcome.provider, outcome.err)
		}
	}

	outcome, fallbackReachable, fallbackFailures := firstUsefulSearch(ctx, client.fallbackProviders, providerRequest)
	hadReachableProvider = hadReachableProvider || fallbackReachable
	failures = append(failures, fallbackFailures...)
	if len(outcome.results) > 0 {
		return successfulSearchResponse(query, outcome.provider, outcome.results, maxResults), nil
	}
	if err := ctx.Err(); err != nil {
		return SearchResponse{}, err
	}
	if hadReachableProvider {
		return SearchResponse{Query: query, Message: "No usable web search results were found."}, nil
	}
	if len(failures) == 0 {
		return SearchResponse{}, fmt.Errorf("web search has no configured providers")
	}
	return SearchResponse{}, fmt.Errorf("all web search providers failed: %w", errors.Join(failures...))
}

func successfulSearchResponse(query, provider string, results []SearchResult, maximum int) SearchResponse {
	results = sanitizeSearchResults(results, provider, maximum)
	return SearchResponse{
		Query:    query,
		Provider: provider,
		Message:  fmt.Sprintf("Found %d result(s) via %s.", len(results), provider),
		Results:  results,
	}
}

func firstUsefulSearch(ctx context.Context, providers []searchProvider, request providerSearchRequest) (searchOutcome, bool, []error) {
	if len(providers) == 0 {
		return searchOutcome{}, false, nil
	}
	searchCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	outcomes := make(chan searchOutcome, len(providers))
	for _, provider := range providers {
		provider := provider
		go func() {
			outcome := searchOutcome{provider: "unknown"}
			defer func() {
				if recovered := recover(); recovered != nil {
					outcome.results = nil
					outcome.err = fmt.Errorf("search provider %s goroutine panicked: %v", outcome.provider, recovered)
					log.Printf("[webaccess] recovered fallback search provider=%s goroutine panic: %v", outcome.provider, recovered)
				}
				outcomes <- outcome
			}()
			outcome = runSearchProvider(searchCtx, provider, request)
		}()
	}

	hadReachable := false
	failures := make([]error, 0, len(providers))
	for range providers {
		outcome := <-outcomes
		if outcome.err != nil {
			failures = append(failures, outcome.err)
			log.Printf("[webaccess] fallback search provider=%s failed: %v", outcome.provider, outcome.err)
			continue
		}
		hadReachable = true
		outcome.results = sanitizeSearchResults(outcome.results, outcome.provider, request.MaxResults)
		if len(outcome.results) == 0 {
			log.Printf("[webaccess] fallback search provider=%s returned no usable results", outcome.provider)
			continue
		}
		cancel()
		log.Printf("[webaccess] fallback search provider=%s returned %d result(s)", outcome.provider, len(outcome.results))
		return outcome, hadReachable, failures
	}
	return searchOutcome{}, hadReachable, failures
}

func runSearchProvider(ctx context.Context, provider searchProvider, request providerSearchRequest) (outcome searchOutcome) {
	outcome.provider = "unknown"
	defer func() {
		if recovered := recover(); recovered != nil {
			outcome.results = nil
			outcome.err = fmt.Errorf("search provider %s panicked: %v", outcome.provider, recovered)
			log.Printf("[webaccess] recovered search provider=%s panic: %v", outcome.provider, recovered)
		}
	}()
	if provider == nil {
		return searchOutcome{provider: "unknown", err: fmt.Errorf("search provider is nil")}
	}
	outcome.provider = provider.Name()
	results, err := provider.Search(ctx, request)
	if err != nil {
		outcome.err = fmt.Errorf("%s: %w", outcome.provider, err)
		return outcome
	}
	outcome.results = sanitizeSearchResults(results, outcome.provider, request.MaxResults)
	return outcome
}

func sanitizeSearchResults(results []SearchResult, provider string, maximum int) []SearchResult {
	seen := make(map[string]struct{}, len(results))
	sanitized := make([]SearchResult, 0, len(results))
	for _, result := range results {
		result.Title = truncateRunes(strings.TrimSpace(result.Title), maxSearchTitleChars)
		result.URL = strings.TrimSpace(result.URL)
		result.Summary = truncateRunes(strings.TrimSpace(result.Summary), maxSearchSummaryChars)
		result.PublishedAt = truncateRunes(strings.TrimSpace(result.PublishedAt), 100)
		if result.Title == "" || len([]rune(result.URL)) > maxWebURLChars || !isHTTPURL(result.URL) {
			continue
		}
		key := normalizedSearchURL(result.URL)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result.Provider = provider
		sanitized = append(sanitized, result)
		if maximum > 0 && len(sanitized) >= maximum {
			break
		}
	}
	return sanitized
}

func truncateRunes(value string, maximum int) string {
	if maximum <= 0 {
		return ""
	}
	characters := []rune(value)
	if len(characters) <= maximum {
		return value
	}
	return string(characters[:maximum])
}

func isHTTPURL(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	return err == nil && parsed.Host != "" && (parsed.Scheme == "http" || parsed.Scheme == "https")
}

func normalizedSearchURL(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	parsed.Fragment = ""
	parsed.Host = strings.ToLower(parsed.Host)
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	return parsed.String()
}

func normalizeTimeRange(value string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "":
		return "", nil
	case "d", "day":
		return "day", nil
	case "w", "week":
		return "week", nil
	case "m", "month":
		return "month", nil
	case "y", "year":
		return "year", nil
	default:
		return "", fmt.Errorf("unsupported web search time range %q", value)
	}
}
