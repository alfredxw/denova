package webaccess

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/url"
	"strings"
	"time"
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
	index    int
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
		outcome := runSearchProviderWithin(ctx, client.primaryProvider, providerRequest, client.config.SearchProviderTimeout)
		if outcome.err == nil {
			hadReachableProvider = true
			if len(outcome.results) > 0 {
				return successfulSearchResponse(query, outcome.provider, outcome.results, maxResults, nil), nil
			}
			log.Printf("[webaccess] search provider=%s returned no usable results; trying free fallbacks", outcome.provider)
		} else {
			failures = append(failures, outcome.err)
			log.Printf("[webaccess] search provider=%s failed; trying free fallbacks: %v", outcome.provider, outcome.err)
		}
	}

	outcome, fallbackReachable, fallbackFailures := combineSearchProviders(ctx, client.fallbackProviders, providerRequest, client.config.SearchProviderTimeout)
	hadReachableProvider = hadReachableProvider || fallbackReachable
	failures = append(failures, fallbackFailures...)
	if len(outcome.results) > 0 {
		return successfulSearchResponse(query, outcome.provider, outcome.results, maxResults, failures), nil
	}
	if err := ctx.Err(); err != nil {
		return SearchResponse{}, err
	}
	if hadReachableProvider {
		return SearchResponse{
			Query:    query,
			Message:  "No usable web search results were found.",
			Warnings: searchWarningMessages(failures),
		}, nil
	}
	if len(failures) == 0 {
		return SearchResponse{}, fmt.Errorf("web search has no configured providers")
	}
	return SearchResponse{}, fmt.Errorf("all web search providers failed: %w", errors.Join(failures...))
}

func successfulSearchResponse(query, provider string, results []SearchResult, maximum int, warnings []error) SearchResponse {
	results = sanitizeSearchResults(results, provider, maximum)
	return SearchResponse{
		Query:    query,
		Provider: provider,
		Message:  fmt.Sprintf("Found %d result(s) via %s.", len(results), provider),
		Results:  results,
		Warnings: searchWarningMessages(warnings),
	}
}

func searchWarningMessages(warnings []error) []string {
	messages := make([]string, 0, len(warnings))
	for _, warning := range warnings {
		if warning == nil {
			continue
		}
		messages = append(messages, truncateRunes(warning.Error(), 500))
	}
	return messages
}

func combineSearchProviders(ctx context.Context, providers []searchProvider, request providerSearchRequest, providerTimeout time.Duration) (searchOutcome, bool, []error) {
	if len(providers) == 0 {
		return searchOutcome{}, false, nil
	}
	searchCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	outcomes := make(chan searchOutcome, len(providers))
	for index, provider := range providers {
		index := index
		provider := provider
		go func() {
			outcome := searchOutcome{index: index, provider: "unknown"}
			defer func() {
				if recovered := recover(); recovered != nil {
					outcome.results = nil
					outcome.err = fmt.Errorf("search provider %s goroutine panicked: %v", outcome.provider, recovered)
					log.Printf("[webaccess] recovered fallback search provider=%s goroutine panic: %v", outcome.provider, recovered)
				}
				outcomes <- outcome
			}()
			outcome = runSearchProviderWithin(searchCtx, provider, request, providerTimeout)
			outcome.index = index
		}()
	}

	hadReachable := false
	failures := make([]error, 0, len(providers))
	orderedOutcomes := make([]searchOutcome, len(providers))
	for range providers {
		var outcome searchOutcome
		select {
		case outcome = <-outcomes:
		case <-ctx.Done():
			return searchOutcome{}, hadReachable, append(failures, ctx.Err())
		}
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
		log.Printf("[webaccess] fallback search provider=%s returned %d result(s)", outcome.provider, len(outcome.results))
		orderedOutcomes[outcome.index] = outcome
	}
	combined := searchOutcome{}
	var providerNames []string
	var resultSets [][]SearchResult
	for _, outcome := range orderedOutcomes {
		if len(outcome.results) == 0 {
			continue
		}
		providerNames = append(providerNames, outcome.provider)
		resultSets = append(resultSets, outcome.results)
	}
	combined.provider = strings.Join(providerNames, "+")
	combined.results = interleaveSearchResults(resultSets, request.MaxResults)
	return combined, hadReachable, failures
}

func runSearchProviderWithin(ctx context.Context, provider searchProvider, request providerSearchRequest, timeout time.Duration) searchOutcome {
	if timeout <= 0 {
		return runSearchProvider(ctx, provider, request)
	}
	providerContext, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return runSearchProvider(providerContext, provider, request)
}

func interleaveSearchResults(resultSets [][]SearchResult, maximum int) []SearchResult {
	seen := make(map[string]struct{})
	combined := make([]SearchResult, 0, maximum)
	for rank := 0; ; rank++ {
		addedAtRank := false
		for _, results := range resultSets {
			if rank >= len(results) {
				continue
			}
			addedAtRank = true
			result := results[rank]
			key := normalizedSearchURL(result.URL)
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			combined = append(combined, result)
			if maximum > 0 && len(combined) >= maximum {
				return combined
			}
		}
		if !addedAtRank {
			return combined
		}
	}
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
	if outcome.provider == ProviderBing && len(outcome.results) > 0 && !searchResultsCoverQuery(request.Query, outcome.results) {
		outcome.results = nil
		outcome.err = fmt.Errorf("%s: returned results do not sufficiently match the complete query", outcome.provider)
	}
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
		if strings.TrimSpace(result.Provider) == "" {
			result.Provider = provider
		}
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
