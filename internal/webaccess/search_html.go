package webaccess

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"unicode"

	"github.com/PuerkitoBio/goquery"
)

const (
	maxSearchDocumentBytes = 4 * 1024 * 1024
	webAccessUserAgent     = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36 Denova/1"
)

type htmlSearchProvider struct {
	name     string
	client   *http.Client
	referer  string
	buildURL func(providerSearchRequest) string
	parse    func(string) []SearchResult
}

type bingSearchProvider struct {
	client *http.Client
}

func (provider *htmlSearchProvider) Name() string { return provider.name }

func (provider *htmlSearchProvider) Search(ctx context.Context, request providerSearchRequest) ([]SearchResult, error) {
	document, err := fetchSearchDocument(ctx, provider.client, provider.buildURL(request), provider.referer)
	if err != nil {
		return nil, err
	}
	return sanitizeSearchResults(provider.parse(document), provider.name, request.MaxResults), nil
}

func newDuckDuckGoProvider(client *http.Client) searchProvider {
	return &htmlSearchProvider{
		name:    ProviderDuckDuckGo,
		client:  client,
		referer: "https://html.duckduckgo.com/",
		buildURL: func(request providerSearchRequest) string {
			values := url.Values{"q": []string{request.Query}}
			switch request.TimeRange {
			case "day":
				values.Set("df", "d")
			case "week":
				values.Set("df", "w")
			case "month":
				values.Set("df", "m")
			case "year":
				values.Set("df", "y")
			}
			return "https://html.duckduckgo.com/html/?" + values.Encode()
		},
		parse: parseDuckDuckGoResults,
	}
}

func newBingProvider(client *http.Client) searchProvider {
	return &bingSearchProvider{client: client}
}

func (provider *bingSearchProvider) Name() string { return ProviderBing }

func (provider *bingSearchProvider) Search(ctx context.Context, request providerSearchRequest) ([]SearchResult, error) {
	htmlDocument, htmlErr := fetchSearchDocument(ctx, provider.client, buildBingSearchURL(request, false), "https://www.bing.com/")
	if htmlErr == nil {
		results := sanitizeSearchResults(parseBingHTMLResults(htmlDocument), ProviderBing, request.MaxResults)
		if len(results) > 0 {
			return results, nil
		}
	}

	rssDocument, rssErr := fetchSearchDocument(ctx, provider.client, buildBingSearchURL(request, true), "https://www.bing.com/")
	if rssErr != nil {
		if htmlErr != nil {
			return nil, fmt.Errorf("HTML search failed (%v); RSS fallback failed: %w", htmlErr, rssErr)
		}
		return nil, rssErr
	}
	return sanitizeSearchResults(parseBingResults(rssDocument), ProviderBing, request.MaxResults), nil
}

func buildBingSearchURL(request providerSearchRequest, rss bool) string {
	market, language := bingLocale(request.Query)
	values := url.Values{
		"q":       []string{request.Query},
		"mkt":     []string{market},
		"setlang": []string{language},
	}
	if rss {
		values.Set("format", "rss")
	}
	if market == "zh-CN" {
		values.Set("cc", "cn")
	}
	switch request.TimeRange {
	case "day":
		values.Set("filters", `ex1:"ez1"`)
	case "week":
		values.Set("filters", `ex1:"ez2"`)
	case "month":
		values.Set("filters", `ex1:"ez3"`)
	}
	return "https://www.bing.com/search?" + values.Encode()
}

func bingLocale(query string) (market, language string) {
	for _, character := range query {
		if unicode.Is(unicode.Han, character) {
			return "zh-CN", "zh-Hans"
		}
	}
	return "en-US", "en-US"
}

func fetchSearchDocument(ctx context.Context, client *http.Client, target, referer string) (string, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return "", fmt.Errorf("create search request: %w", err)
	}
	request.Header.Set("User-Agent", webAccessUserAgent)
	request.Header.Set("Accept", "text/html,application/xhtml+xml,application/rss+xml;q=0.9,application/xml;q=0.9")
	request.Header.Set("Accept-Language", "en-US,en;q=0.8,zh-CN;q=0.7")
	if referer != "" {
		request.Header.Set("Referer", referer)
	}
	response, err := client.Do(request)
	if err != nil {
		return "", fmt.Errorf("send search request: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("search endpoint returned HTTP %d", response.StatusCode)
	}
	content, err := io.ReadAll(io.LimitReader(response.Body, maxSearchDocumentBytes+1))
	if err != nil {
		return "", fmt.Errorf("read search response: %w", err)
	}
	if len(content) > maxSearchDocumentBytes {
		return "", fmt.Errorf("search response exceeds %d-byte safety limit", maxSearchDocumentBytes)
	}
	return string(content), nil
}

func parseDuckDuckGoResults(htmlText string) []SearchResult {
	document, err := goquery.NewDocumentFromReader(strings.NewReader(htmlText))
	if err != nil {
		return nil
	}
	var results []SearchResult
	document.Find(".result").Each(func(_ int, selection *goquery.Selection) {
		link := selection.Find("a.result__a").First()
		href, _ := link.Attr("href")
		href = unwrapDuckDuckGoURL(href)
		title := strings.TrimSpace(link.Text())
		if title == "" || !isHTTPURL(href) {
			return
		}
		results = append(results, SearchResult{
			Title:   title,
			URL:     href,
			Summary: strings.TrimSpace(selection.Find(".result__snippet").First().Text()),
		})
	})
	return results
}

func unwrapDuckDuckGoURL(href string) string {
	parsed, err := url.Parse(strings.TrimSpace(href))
	if err != nil {
		return href
	}
	if target := parsed.Query().Get("uddg"); target != "" {
		return target
	}
	return href
}

func parseBingResults(htmlText string) []SearchResult {
	type bingRSS struct {
		Channel struct {
			Items []struct {
				Title       string `xml:"title"`
				Link        string `xml:"link"`
				Description string `xml:"description"`
			} `xml:"item"`
		} `xml:"channel"`
	}
	var feed bingRSS
	if xml.Unmarshal([]byte(htmlText), &feed) == nil && len(feed.Channel.Items) > 0 {
		results := make([]SearchResult, 0, len(feed.Channel.Items))
		for _, item := range feed.Channel.Items {
			results = append(results, SearchResult{
				Title:   strings.TrimSpace(item.Title),
				URL:     strings.TrimSpace(item.Link),
				Summary: strings.TrimSpace(item.Description),
			})
		}
		return results
	}
	return parseBingHTMLResults(htmlText)
}

func parseBingHTMLResults(htmlText string) []SearchResult {
	document, err := goquery.NewDocumentFromReader(strings.NewReader(htmlText))
	if err != nil {
		return nil
	}
	var results []SearchResult
	document.Find("li.b_algo").Each(func(_ int, selection *goquery.Selection) {
		link := selection.Find("h2 a").First()
		href, _ := link.Attr("href")
		title := strings.TrimSpace(link.Text())
		if title == "" || !isHTTPURL(href) {
			return
		}
		results = append(results, SearchResult{
			Title:   title,
			URL:     href,
			Summary: strings.TrimSpace(selection.Find(".b_caption p, .b_lineclamp4, p").First().Text()),
		})
	})
	return results
}
