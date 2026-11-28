package webaccess

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"denova/config"
)

const maxSearchAPIResponseBytes = 2 * 1024 * 1024

// Hosted APIs share transport, but keep their request and result semantics
// explicit here. Endpoints are fixed; credentials cannot follow redirects.
type apiSearchProvider struct {
	name, endpoint, apiKey, mode string
	client                       *http.Client
}

func newAPISearchProvider(name string, settings *config.SearchServiceSettings, client *http.Client) *apiSearchProvider {
	secured := *client
	secured.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	if settings == nil {
		settings = &config.SearchServiceSettings{}
	}
	provider := &apiSearchProvider{name: name, apiKey: strings.TrimSpace(settings.APIKey), mode: settings.Mode, client: &secured}
	switch name {
	case ProviderDeepSeek:
		provider.endpoint = "https://api.deepseek.com/v1/tools/search"
	case ProviderPerplexity:
		provider.endpoint = "https://api.perplexity.ai/search"
		if provider.mode == "" {
			provider.mode = "web"
		}
	case ProviderTavily:
		provider.endpoint = "https://api.tavily.com/search"
		if provider.mode == "" {
			provider.mode = "basic"
		}
	case ProviderKeenable:
		// Without a key, Keenable serves a keyless endpoint rate limited per IP
		// (1,000 requests per hour, 10 per second) that logs the query and IP.
		// See https://docs.keenable.ai/rate-limits.
		provider.endpoint = "https://api.keenable.ai/v1/search/public"
		if provider.apiKey != "" {
			provider.endpoint = "https://api.keenable.ai/v1/search"
		}
	}
	return provider
}

func (provider *apiSearchProvider) Name() string { return provider.name }

func (provider *apiSearchProvider) Search(ctx context.Context, request providerSearchRequest) ([]SearchResult, error) {
	payload := map[string]any{"query": request.Query}
	switch provider.name {
	case ProviderDeepSeek:
		query := request.Query
		if request.TimeRange != "" {
			date := time.Now().UTC()
			switch request.TimeRange {
			case "day":
				date = date.AddDate(0, 0, -1)
			case "week":
				date = date.AddDate(0, 0, -7)
			case "month":
				date = date.AddDate(0, -1, 0)
			case "year":
				date = date.AddDate(-1, 0, 0)
			}
			query += " after:" + date.Format("2006-01-02")
		}
		if utf8.RuneCountInString(query) > 128 {
			return nil, searchError("query_too_long", "DeepSeek search supports at most 128 Unicode characters, including time filters. Shorten the query; a time filter uses 17 characters.")
		}
		payload["query"], payload["count"], payload["need_content"] = query, request.MaxResults, false
	case ProviderPerplexity:
		payload["max_results"], payload["search_type"] = request.MaxResults, provider.mode
		payload["max_tokens_per_page"] = 1024
		payload["max_tokens"] = request.MaxResults * 1024
		if request.TimeRange != "" {
			payload["search_recency_filter"] = request.TimeRange
		}
	case ProviderTavily:
		payload["max_results"], payload["search_depth"] = request.MaxResults, provider.mode
		payload["auto_parameters"], payload["include_answer"], payload["include_raw_content"], payload["include_images"] = false, false, false, false
		payload["include_published_date"] = true
		if request.TimeRange != "" {
			payload["time_range"] = request.TimeRange
		}
	case ProviderKeenable:
		payload["max_results"] = request.MaxResults
		if after, ok := map[string]string{"day": "1d", "week": "7d", "month": "1mo", "year": "1y"}[request.TimeRange]; ok {
			payload["published_after"] = after
		}
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, provider.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	switch provider.name {
	case ProviderKeenable:
		req.Header.Set("X-Keenable-Title", "Denova")
		if provider.apiKey != "" {
			req.Header.Set("X-API-Key", provider.apiKey)
		}
	default:
		req.Header.Set("Authorization", "Bearer "+provider.apiKey)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	response, err := provider.client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, searchError("unavailable", "Search service connection failed. Check connectivity or retry later.")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, searchHTTPError(response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxSearchAPIResponseBytes+1))
	if err != nil {
		return nil, searchError("unavailable", "Search service response could not be read.")
	}
	if len(data) > maxSearchAPIResponseBytes {
		return nil, searchError("invalid_response", "Search service response exceeds the size limit.")
	}
	var decoded struct {
		Results *[]struct {
			Title         string `json:"title"`
			URL           string `json:"url"`
			Summary       string `json:"summary"`
			Snippet       string `json:"snippet"`
			Content       string `json:"content"`
			Date          string `json:"date"`
			PublishedDate string `json:"published_date"`
			PublishedAt   string `json:"published_at"`
			Description   string `json:"description"`
		} `json:"results"`
	}
	if err := json.Unmarshal(data, &decoded); err != nil || decoded.Results == nil {
		return nil, searchError("invalid_response", "Search service returned an invalid response.")
	}
	results := make([]SearchResult, 0, len(*decoded.Results))
	for _, item := range *decoded.Results {
		summary, date := item.Snippet, item.Date
		switch provider.name {
		case ProviderDeepSeek:
			if strings.TrimSpace(item.Summary) != "" {
				summary = item.Summary
			}
		case ProviderPerplexity:
		case ProviderTavily:
			summary, date = item.Content, item.PublishedDate
		case ProviderKeenable:
			date = item.PublishedAt
			if strings.TrimSpace(summary) == "" {
				summary = item.Description
			}
		}
		results = append(results, SearchResult{Title: item.Title, URL: item.URL, Summary: summary, PublishedAt: date, Provider: provider.name})
	}
	return results, nil
}

func searchHTTPError(status int) error {
	switch status {
	case 400, 413, 422:
		return searchError("invalid_request", "Search service rejected the query or options. Adjust the query and retry.")
	case 401, 403:
		return searchError("authentication", "Search service authentication failed. Check its API key and permissions.")
	case 402, 432, 433:
		return searchError("quota", "Search service balance or quota is exhausted. Check the service account.")
	case 429:
		return searchError("rate_limit", "Search service rate limit reached. Wait before retrying.")
	case 408, 504:
		return searchError("unavailable", "Search service request timed out.")
	default:
		if status >= 500 {
			return searchError("unavailable", "Search service is temporarily unavailable.")
		}
		return searchError("invalid_response", "Search service returned an unexpected HTTP status.")
	}
}
