package webaccess

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/PuerkitoBio/goquery"
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
	results, jsonErr := provider.searchJSON(ctx, request)
	if jsonErr == nil {
		return results, nil
	}
	results, htmlErr := provider.searchHTML(ctx, request)
	if htmlErr == nil {
		return results, nil
	}
	return nil, fmt.Errorf("SearXNG endpoint %s: JSON search failed (%v); HTML form fallback failed: %w", provider.endpoint.Redacted(), jsonErr, htmlErr)
}

func (provider *searXNGProvider) searchJSON(ctx context.Context, request providerSearchRequest) ([]SearchResult, error) {
	endpoint := *provider.endpoint
	query := searXNGSearchValues(request)
	query.Set("format", "json")
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
		return nil, fmt.Errorf("HTTP %d", response.StatusCode)
	}
	data, err := readSearXNGResponse(response.Body)
	if err != nil {
		return nil, err
	}
	var payload searXNGResponse
	if err := json.Unmarshal(data, &payload); err != nil {
		if results, isSearXNGPage := parseSearXNGHTMLResults(string(data)); isSearXNGPage {
			return sanitizeSearchResults(results, provider.Name(), request.MaxResults), nil
		}
		if isSearXNGAccessChallenge(string(data)) {
			return nil, fmt.Errorf("received an HTML access challenge instead of JSON; this instance is blocking automated search requests")
		}
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

func (provider *searXNGProvider) searchHTML(ctx context.Context, request providerSearchRequest) ([]SearchResult, error) {
	values := searXNGSearchValues(request)
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, provider.endpoint.String(), strings.NewReader(values.Encode()))
	if err != nil {
		return nil, fmt.Errorf("create SearXNG HTML request: %w", err)
	}
	httpRequest.Header.Set("User-Agent", webAccessUserAgent)
	httpRequest.Header.Set("Accept", "text/html,application/xhtml+xml")
	httpRequest.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	httpRequest.Header.Set("Referer", searXNGReferer(provider.endpoint))
	response, err := provider.client.Do(httpRequest)
	if err != nil {
		return nil, fmt.Errorf("send SearXNG HTML request: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", response.StatusCode)
	}
	data, err := readSearXNGResponse(response.Body)
	if err != nil {
		return nil, err
	}
	results, isSearXNGPage := parseSearXNGHTMLResults(string(data))
	if !isSearXNGPage {
		if isSearXNGAccessChallenge(string(data)) {
			return nil, fmt.Errorf("received an HTML access challenge; this instance is blocking automated search requests")
		}
		return nil, fmt.Errorf("received HTML that is not a recognizable SearXNG results page")
	}
	return sanitizeSearchResults(results, provider.Name(), request.MaxResults), nil
}

func searXNGSearchValues(request providerSearchRequest) url.Values {
	values := url.Values{
		"q":          []string{request.Query},
		"language":   []string{"auto"},
		"safesearch": []string{"0"},
	}
	switch request.TimeRange {
	case "day", "month", "year":
		values.Set("time_range", request.TimeRange)
	}
	return values
}

func searXNGReferer(endpoint *url.URL) string {
	referer := *endpoint
	referer.Path = strings.TrimSuffix(referer.Path, "/search") + "/"
	referer.RawQuery = ""
	referer.Fragment = ""
	return referer.String()
}

func readSearXNGResponse(body io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(body, maxSearchDocumentBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read SearXNG response: %w", err)
	}
	if len(data) > maxSearchDocumentBytes {
		return nil, fmt.Errorf("SearXNG response exceeds %d-byte safety limit", maxSearchDocumentBytes)
	}
	return data, nil
}

func parseSearXNGHTMLResults(htmlText string) ([]SearchResult, bool) {
	document, err := goquery.NewDocumentFromReader(strings.NewReader(htmlText))
	if err != nil {
		return nil, false
	}
	isSearXNGPage := document.Find("#urls, form#search").Length() > 0
	document.Find(`meta[name="generator"]`).EachWithBreak(func(_ int, selection *goquery.Selection) bool {
		content, _ := selection.Attr("content")
		if strings.Contains(strings.ToLower(content), "searxng") {
			isSearXNGPage = true
			return false
		}
		return true
	})
	var results []SearchResult
	document.Find("article.result").Each(func(_ int, selection *goquery.Selection) {
		link := selection.Find("h3 a[href]").First()
		href, _ := link.Attr("href")
		title := strings.TrimSpace(link.Text())
		if title == "" || !isHTTPURL(href) {
			return
		}
		publishedAt, _ := selection.Find("time.published_date").First().Attr("datetime")
		results = append(results, SearchResult{
			Title:       title,
			URL:         href,
			Summary:     strings.TrimSpace(selection.Find("p.content").First().Text()),
			PublishedAt: strings.TrimSpace(publishedAt),
		})
	})
	return results, isSearXNGPage
}

func isSearXNGAccessChallenge(htmlText string) bool {
	normalized := strings.ToLower(htmlText)
	for _, marker := range []string{
		"making sure you&#39;re not a bot",
		"making sure you're not a bot",
		"/.within.website/",
		"cf-chl-",
		"captcha",
	} {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	return false
}
