package agents

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/PuerkitoBio/goquery"
	agent "github.com/alfredxw/denova/agent"

	"denova/config"
)

const webSearchToolDescription = "Search the public web for current or external information. Return result titles, URLs, and short summaries; cite useful URLs in the final answer."

const (
	webSearchPerEngine    = 20
	webSearchMaxTotal     = 50
	webSearchMaxHTMLBytes = 8 * 1024 * 1024
	webSearchUserAgent    = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"
)

func newWebSearchTools() ([]agent.BaseTool, error) {
	aggregator := newDefaultWebSearchAggregator()
	searchTool, err := agent.InferTool[webSearchToolInput, webSearchResponse](
		config.AgentToolWebSearch,
		webSearchToolDescription,
		func(ctx context.Context, in webSearchToolInput) (webSearchResponse, error) {
			query := strings.TrimSpace(in.Query)
			if query == "" {
				return webSearchResponse{}, fmt.Errorf("search query is required")
			}
			return aggregator.run(ctx, webSearchRequest{Query: query, TimeRange: in.TimeRange}), nil
		},
	)
	if err != nil {
		return nil, fmt.Errorf("创建网页搜索工具失败: %w", err)
	}
	definedSearchTool, err := defineTool(searchTool, boundedReadDescriptor(ToolSourceWeb, config.AgentToolWebSearch))
	if err != nil {
		return nil, err
	}
	return []agent.BaseTool{definedSearchTool}, nil
}

type webSearchToolInput struct {
	Query     string `json:"query" jsonschema:"required" jsonschema_description:"The user's search query. The query is required."`
	TimeRange string `json:"time_range,omitempty" jsonschema:"enum=d,enum=w,enum=m,enum=y" jsonschema_description:"Optional time range filter: d (past day), w (past week), m (past month), y (past year). Omit for any time."`
}

type webSearchRequest struct {
	Query     string
	TimeRange string
}

type webSearchResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Summary string `json:"summary"`
	Engine  string `json:"engine"`
}

type webSearchResponse struct {
	Message string            `json:"message"`
	Results []webSearchResult `json:"results,omitempty"`
}

type webSearchEngine interface {
	Name() string
	Search(ctx context.Context, req webSearchRequest) ([]webSearchResult, error)
}

type webSearchOutcome struct {
	name    string
	results []webSearchResult
	err     error
}

type webSearchAggregator struct {
	engines  []webSearchEngine
	maxTotal int
}

func defaultWebSearchEngines() []webSearchEngine {
	return []webSearchEngine{
		newDuckDuckGoEngine(webSearchPerEngine),
		newBingEngine(webSearchPerEngine),
		newBaiduEngine(webSearchPerEngine),
		newGoogleEngine(webSearchPerEngine),
	}
}

func newDefaultWebSearchAggregator() *webSearchAggregator {
	engines := defaultWebSearchEngines()
	return &webSearchAggregator{engines: engines, maxTotal: webSearchMaxTotal}
}

func (a *webSearchAggregator) fanOut(ctx context.Context, req webSearchRequest) []webSearchOutcome {
	outcomes := make([]webSearchOutcome, len(a.engines))
	var wg sync.WaitGroup
	for i, eng := range a.engines {
		wg.Add(1)
		go func(i int, eng webSearchEngine) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					outcomes[i] = webSearchOutcome{name: eng.Name(), err: fmt.Errorf("panic: %v", r)}
					log.Printf("[agent] web_search 引擎 %s panic: %v", eng.Name(), r)
				}
			}()
			res, err := eng.Search(ctx, req)
			outcomes[i] = webSearchOutcome{name: eng.Name(), results: res, err: err}
		}(i, eng)
	}
	wg.Wait()
	return outcomes
}

func (a *webSearchAggregator) run(ctx context.Context, req webSearchRequest) webSearchResponse {
	// The owning Agent operation controls cancellation. Installing a tool-local
	// deadline here used to terminate otherwise healthy long-running model turns
	// after a hard-coded wall-clock duration.
	outcomes := a.fanOut(ctx, req)

	var okNames, failedNames []string
	buckets := make([][]webSearchResult, 0, len(outcomes))
	for _, o := range outcomes {
		if o.err != nil {
			failedNames = append(failedNames, o.name)
			log.Printf("[agent] web_search 引擎 %s 搜索失败，结果已丢弃: %v", o.name, o.err)
			continue
		}
		if len(o.results) == 0 {
			// 引擎可访问但返回 0 条（如 Google 同意页、百度安全验证页、空结果），按“无结果即丢弃”跳过，不计入贡献也不计入失败。
			log.Printf("[agent] web_search 引擎 %s 返回 0 条结果，已跳过", o.name)
			continue
		}
		okNames = append(okNames, o.name)
		buckets = append(buckets, o.results)
	}

	merged := mergeSearchResults(buckets, a.maxTotal)
	return webSearchResponse{
		Message: buildSearchMessage(merged, okNames, failedNames),
		Results: merged,
	}
}

func mergeSearchResults(buckets [][]webSearchResult, maxTotal int) []webSearchResult {
	seen := make(map[string]struct{})
	merged := make([]webSearchResult, 0, maxTotal)
	maxLen := 0
	for _, b := range buckets {
		if len(b) > maxLen {
			maxLen = len(b)
		}
	}
	for i := 0; i < maxLen; i++ {
		for _, b := range buckets {
			if i >= len(b) {
				continue
			}
			key := normalizeSearchURL(b[i].URL)
			if key == "" {
				continue
			}
			if _, dup := seen[key]; dup {
				continue
			}
			seen[key] = struct{}{}
			merged = append(merged, b[i])
			if maxTotal > 0 && len(merged) >= maxTotal {
				return merged
			}
		}
	}
	return merged
}

func normalizeSearchURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return ""
	}
	key := u.Scheme + "://" + strings.ToLower(u.Host) + strings.TrimRight(u.Path, "/")
	if u.RawQuery != "" {
		key += "?" + u.RawQuery
	}
	return key
}

func buildSearchMessage(merged []webSearchResult, ok, failed []string) string {
	if len(merged) == 0 {
		if len(failed) > 0 && len(ok) == 0 {
			return fmt.Sprintf("所有搜索引擎均未返回可用结果（失败: %s）。", strings.Join(failed, ", "))
		}
		return "No good results were found."
	}
	msg := fmt.Sprintf("Found %d aggregated results", len(merged))
	if len(ok) > 0 {
		msg += " from " + strings.Join(ok, ", ")
	}
	if len(failed) > 0 {
		msg += " (failed engines skipped: " + strings.Join(failed, ", ") + ")"
	}
	return msg + "."
}

func fetchSearchHTML(ctx context.Context, client *http.Client, target, referer string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return "", fmt.Errorf("创建请求失败: %w", err)
	}
	req.Header.Set("User-Agent", webSearchUserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
	req.Header.Set("Upgrade-Insecure-Requests", "1")
	req.Header.Set("Sec-Fetch-Dest", "document")
	req.Header.Set("Sec-Fetch-Mode", "navigate")
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	req.Header.Set("Sec-Fetch-User", "?1")
	if referer != "" {
		req.Header.Set("Referer", referer)
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("非预期状态码 %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, webSearchMaxHTMLBytes+1))
	if err != nil {
		return "", fmt.Errorf("读取响应失败: %w", err)
	}
	if len(data) > webSearchMaxHTMLBytes {
		return "", fmt.Errorf("搜索响应超过 %d 字节上限", webSearchMaxHTMLBytes)
	}
	return string(data), nil
}

func unwrapGoogleURL(href string) string {
	if !strings.HasPrefix(href, "/url?") {
		return href
	}
	if u, err := url.Parse(href); err == nil {
		if q := u.Query().Get("q"); q != "" {
			return q
		}
	}
	return href
}

type htmlSearchEngine struct {
	name     string
	max      int
	client   *http.Client
	referer  string
	buildURL func(query, timeRange string) string
	parse    func(html string) []webSearchResult
}

func newDuckDuckGoEngine(max int) webSearchEngine {
	return &htmlSearchEngine{
		name:    "duckduckgo",
		max:     max,
		client:  &http.Client{},
		referer: "https://html.duckduckgo.com/",
		buildURL: func(q, tr string) string {
			u := "https://html.duckduckgo.com/html/?q=" + url.QueryEscape(q)
			switch tr {
			case "d", "w", "m", "y":
				u += "&df=" + tr
			}
			return u
		},
		parse: parseDuckDuckGoResults,
	}
}

func (e *htmlSearchEngine) Name() string { return e.name }

func (e *htmlSearchEngine) Search(ctx context.Context, req webSearchRequest) ([]webSearchResult, error) {
	html, err := fetchSearchHTML(ctx, e.client, e.buildURL(req.Query, req.TimeRange), e.referer)
	if err != nil {
		return nil, err
	}
	results := e.parse(html)
	for i := range results {
		results[i].Engine = e.name
	}
	if e.max > 0 && len(results) > e.max {
		results = results[:e.max]
	}
	return results, nil
}

func newBingEngine(max int) webSearchEngine {
	return &htmlSearchEngine{
		name:    "bing",
		max:     max,
		client:  &http.Client{},
		referer: "https://www.bing.com/",
		buildURL: func(q, _ string) string {
			return "https://www.bing.com/search?mkt=zh-CN&setlang=zh-CN&q=" + url.QueryEscape(q)
		},
		parse: parseBingResults,
	}
}

func parseDuckDuckGoResults(htmlText string) []webSearchResult {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(htmlText))
	if err != nil {
		return nil
	}
	var results []webSearchResult
	doc.Find(".result").Each(func(_ int, selection *goquery.Selection) {
		link := selection.Find("a.result__a").First()
		if link.Length() == 0 {
			return
		}
		href, _ := link.Attr("href")
		href = unwrapDuckDuckGoURL(href)
		title := strings.TrimSpace(link.Text())
		if normalizeSearchURL(href) == "" || title == "" {
			return
		}
		summary := strings.TrimSpace(selection.Find(".result__snippet").First().Text())
		results = append(results, webSearchResult{Title: title, URL: href, Summary: summary})
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

func newBaiduEngine(max int) webSearchEngine {
	return &htmlSearchEngine{
		name:    "baidu",
		max:     max,
		client:  &http.Client{},
		referer: "https://m.baidu.com/",
		buildURL: func(q, _ string) string {
			// 移动端比 PC 端更少返回“百度安全验证”反爬页。
			return "https://m.baidu.com/s?from=1099a&sa=tb&word=" + url.QueryEscape(q)
		},
		parse: parseBaiduResults,
	}
}

func newGoogleEngine(max int) webSearchEngine {
	return &htmlSearchEngine{
		name:    "google",
		max:     max,
		client:  &http.Client{},
		referer: "https://www.google.com/",
		buildURL: func(q, tr string) string {
			u := "https://www.google.com/search?hl=en&q=" + url.QueryEscape(q)
			switch tr {
			case "d", "w", "m", "y":
				u += "&tbs=qdr:" + tr
			}
			return u
		},
		parse: parseGoogleResults,
	}
}

func parseBingResults(htmlText string) []webSearchResult {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(htmlText))
	if err != nil {
		return nil
	}
	var results []webSearchResult
	doc.Find("li.b_algo").Each(func(_ int, s *goquery.Selection) {
		a := s.Find("h2 a").First()
		if a.Length() == 0 {
			return
		}
		href, _ := a.Attr("href")
		title := strings.TrimSpace(a.Text())
		if href == "" || title == "" {
			return
		}
		summary := strings.TrimSpace(s.Find(".b_caption p, .b_lineclamp4, p").First().Text())
		results = append(results, webSearchResult{Title: title, URL: href, Summary: summary})
	})
	return results
}

func parseBaiduResults(htmlText string) []webSearchResult {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(htmlText))
	if err != nil {
		return nil
	}
	var results []webSearchResult
	doc.Find("div.c-container, div.result, div.c-result").Each(func(_ int, s *goquery.Selection) {
		a := s.Find("h3 a").First()
		if a.Length() == 0 {
			a = s.Find("a[href]").First()
		}
		if a.Length() == 0 {
			return
		}
		href, _ := a.Attr("href")
		title := strings.TrimSpace(a.Text())
		if href == "" || title == "" {
			return
		}
		summary := strings.TrimSpace(s.Find(".c-abstract, [class*='content-right'], .c-gap-top-small, span.c-color").First().Text())
		results = append(results, webSearchResult{Title: title, URL: href, Summary: summary})
	})
	return results
}

func parseGoogleResults(htmlText string) []webSearchResult {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(htmlText))
	if err != nil {
		return nil
	}
	var results []webSearchResult
	doc.Find("div.g, div.tF2Cxc").Each(func(_ int, s *goquery.Selection) {
		a := s.Find("a").First()
		if a.Length() == 0 {
			return
		}
		href, _ := a.Attr("href")
		href = unwrapGoogleURL(href)
		title := strings.TrimSpace(s.Find("h3").First().Text())
		if !strings.HasPrefix(href, "http") || title == "" {
			return
		}
		summary := strings.TrimSpace(s.Find(".VwiC3b, span.aCOpRe, div[data-sncf], [style*='-webkit-line-clamp']").First().Text())
		results = append(results, webSearchResult{Title: title, URL: href, Summary: summary})
	})
	return results
}
