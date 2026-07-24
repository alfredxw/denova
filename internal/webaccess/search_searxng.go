package webaccess

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

type searXNGProvider struct {
	endpoint *url.URL
	client   *http.Client
}

type searXNGResponse struct {
	Results []struct {
		URL           string `json:"url"`
		Title         string `json:"title"`
		Content       string `json:"content"`
		PublishedDate string `json:"publishedDate"`
	} `json:"results"`
}

func newSearXNGProvider(baseURL string, client *http.Client) (searchProvider, error) {
	endpoint, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || endpoint.Host == "" || (endpoint.Scheme != "http" && endpoint.Scheme != "https") {
		return nil, fmt.Errorf("SearXNG base URL must be an absolute HTTP(S) URL")
	}
	endpoint.RawQuery = ""
	endpoint.Fragment = ""
	path := strings.TrimRight(endpoint.Path, "/")
	if !strings.HasSuffix(path, "/search") && path != "search" {
		path += "/search"
	}
	endpoint.Path = path
	return &searXNGProvider{endpoint: endpoint, client: client}, nil
}

func (provider *searXNGProvider) Name() string { return ProviderSearXNG }

func (provider *searXNGProvider) Search(ctx context.Context, request providerSearchRequest) ([]SearchResult, error) {
	endpoint := *provider.endpoint
	query := endpoint.Query()
	query.Set("q", request.Query)
	query.Set("format", "json")
	query.Set("language", "auto")
	query.Set("safesearch", "0")
	switch request.TimeRange {
	case "day", "month", "year":
		query.Set("time_range", request.TimeRange)
	}
	endpoint.RawQuery = query.Encode()

	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create SearXNG request: %w", err)
	}
	httpRequest.Header.Set("User-Agent", webAccessUserAgent)
	httpRequest.Header.Set("Accept", "application/json")
	response, err := provider.client.Do(httpRequest)
	if err != nil {
		return nil, fmt.Errorf("send SearXNG request: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("SearXNG returned HTTP %d; ensure JSON format is enabled", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxSearchDocumentBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read SearXNG response: %w", err)
	}
	if len(data) > maxSearchDocumentBytes {
		return nil, fmt.Errorf("SearXNG response exceeds %d-byte safety limit", maxSearchDocumentBytes)
	}
	var payload searXNGResponse
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("decode SearXNG JSON response: %w", err)
	}
	results := make([]SearchResult, 0, len(payload.Results))
	for _, item := range payload.Results {
		results = append(results, SearchResult{
			Title:       item.Title,
			URL:         item.URL,
			Summary:     item.Content,
			PublishedAt: item.PublishedDate,
		})
	}
	return sanitizeSearchResults(results, provider.Name(), request.MaxResults), nil
}
