package webaccess

import (
	"context"
	"fmt"
	"log/slog"
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
	warnings []error
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

	failures := append([]error(nil), client.configurationWarnings...)
	hadReachableProvider := false
	if client.primaryProvider != nil {
		outcome := runSearchProviderWithin(ctx, client.primaryProvider, providerRequest, client.config.SearchProviderTimeout)
		failures = append(failures, outcome.warnings...)
		if outcome.err == nil {
			hadReachableProvider = true
			if len(outcome.results) > 0 {
				return successfulSearchResponse(query, outcome.provider, outcome.results, maxResults, nil), nil
			}
			slog.WarnContext(ctx, fmt.Sprintf("[webaccess] search provider=%s returned no usable results; trying free fallbacks", outcome.provider))
		} else {
			failures = append(failures, outcome.err)
			slog.ErrorContext(ctx, fmt.Sprintf("[webaccess] search provider=%s failed; trying free fallbacks: %v", outcome.provider, outcome.err))
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
			Schema:          SearchResponseSchema,
			Query:           query,
			Status:          SearchStatusNoResults,
			Message:         "No usable web search results were found. 未找到可用的网页搜索结果。",
			Warnings:        searchWarningMessages(failures),
			RetryStrategy:   SearchRetryChangeQuery,
			SuggestedAction: "Do not immediately repeat the same query. Change the keywords, remove overly narrow qualifiers, or use a known URL with web_fetch. 不要立即重复相同查询；请调整关键词、移除过窄限定，或用 web_fetch 打开已知网址。",
		}, nil
	}
	if len(failures) == 0 {
		failures = append(failures, fmt.Errorf("web search has no configured providers"))
	}
	return SearchResponse{
		Schema:          SearchResponseSchema,
		Query:           query,
		Status:          SearchStatusProvidersUnavailable,
		Message:         "All web search providers are unavailable. 所有网页搜索提供方当前均不可用。",
		Warnings:        searchWarningMessages(failures),
		RetryStrategy:   SearchRetryWaitOrReconfigure,
		SuggestedAction: "Do not immediately repeat the same query. Wait before retrying, verify the configured SearXNG endpoint, or continue from a known URL with web_fetch. 不要立即重复相同查询；请稍后重试、检查 SearXNG 配置，或用 web_fetch 打开已知网址。",
	}, nil
}

func successfulSearchResponse(query, provider string, results []SearchResult, maximum int, warnings []error) SearchResponse {
	results = sanitizeSearchResults(results, provider, maximum)
	return SearchResponse{
		Schema:        SearchResponseSchema,
		Query:         query,
		Status:        SearchStatusSuccess,
		Provider:      provider,
		Message:       fmt.Sprintf("Found %d result(s) via %s. 通过 %s 找到 %d 条结果。", len(results), provider, provider, len(results)),
		Results:       results,
		Warnings:      searchWarningMessages(warnings),
		RetryStrategy: SearchRetryNone,
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
					slog.ErrorContext(ctx, fmt.Sprintf("[webaccess] recovered fallback search provider=%s goroutine panic: %v", outcome.provider, recovered))
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
		failures = append(failures, outcome.warnings...)
		if outcome.err != nil {
			failures = append(failures, outcome.err)
			slog.ErrorContext(ctx, fmt.Sprintf("[webaccess] fallback search provider=%s failed: %v", outcome.provider, outcome.err))
			continue
		}
		hadReachable = true
		outcome.results = sanitizeSearchResults(outcome.results, outcome.provider, request.MaxResults)
		if len(outcome.results) == 0 {
			slog.WarnContext(ctx, fmt.Sprintf("[webaccess] fallback search provider=%s returned no usable results", outcome.provider))
			continue
		}
		slog.WarnContext(ctx, fmt.Sprintf("[webaccess] fallback search provider=%s returned %d result(s)", outcome.provider, len(outcome.results)))
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
			slog.ErrorContext(ctx, fmt.Sprintf("[webaccess] recovered search provider=%s panic: %v", outcome.provider, recovered))
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
	if outcome.provider == ProviderBing && len(outcome.results) > 0 {
		originalCount := len(outcome.results)
		outcome.results = filterSearchResultsByQuery(request.Query, outcome.results)
		if removed := originalCount - len(outcome.results); removed > 0 {
			outcome.warnings = append(outcome.warnings, fmt.Errorf(
				"%s: filtered %d of %d result(s) that did not sufficiently match the query",
				outcome.provider,
				removed,
				originalCount,
			))
		}
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
