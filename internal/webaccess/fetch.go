package webaccess

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"net/url"
	"strings"

	"codeberg.org/readeck/go-readability/v2"
	"github.com/JohannesKaufmann/html-to-markdown"
	"github.com/PuerkitoBio/goquery"
	"golang.org/x/net/html/charset"
)

func (client *Client) Fetch(ctx context.Context, request FetchRequest) (FetchResponse, error) {
	target, err := validateFetchURL(request.URL)
	if err != nil {
		return FetchResponse{}, err
	}
	if request.StartIndex < 0 {
		return FetchResponse{}, fmt.Errorf("web fetch start_index must not be negative")
	}
	maxChars := request.MaxChars
	if maxChars <= 0 || maxChars > client.config.FetchMaxContentChars {
		maxChars = client.config.FetchMaxContentChars
	}

	log.Printf("[webaccess] fetching public page url=%s response_limit_bytes=%d", safeURLForLog(target), client.config.FetchMaxResponseBytes)
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return FetchResponse{}, fmt.Errorf("create web fetch request: %w", err)
	}
	httpRequest.Header.Set("User-Agent", webAccessUserAgent)
	httpRequest.Header.Set("Accept", "text/html,application/xhtml+xml,text/markdown,text/plain,application/json,application/xml;q=0.9,*/*;q=0.1")
	httpRequest.Header.Set("Accept-Language", "en-US,en;q=0.8,zh-CN;q=0.7")
	response, err := client.fetchHTTPClient.Do(httpRequest)
	if err != nil {
		return FetchResponse{}, fmt.Errorf("fetch public page: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return FetchResponse{}, fmt.Errorf("web page returned HTTP %d", response.StatusCode)
	}
	raw, err := readBoundedResponse(response, client.config.FetchMaxResponseBytes)
	if err != nil {
		return FetchResponse{}, err
	}
	finalURL := response.Request.URL
	if len([]rune(finalURL.String())) > maxWebURLChars {
		return FetchResponse{}, fmt.Errorf("final web page URL exceeds %d-character safety limit", maxWebURLChars)
	}
	mediaType := responseMediaType(response.Header.Get("Content-Type"), raw)
	decoded, err := decodeResponseText(raw, response.Header.Get("Content-Type"))
	if err != nil {
		return FetchResponse{}, fmt.Errorf("decode web page text: %w", err)
	}

	var title, byline, excerpt, content string
	switch {
	case mediaType == "text/html" || mediaType == "application/xhtml+xml":
		title, byline, excerpt, content, err = extractReadableMarkdown(decoded, finalURL)
	case mediaType == "text/markdown" || strings.HasSuffix(mediaType, "/markdown"):
		content = normalizeFetchedText(decoded)
	case strings.HasPrefix(mediaType, "text/") || isStructuredTextMediaType(mediaType):
		content = normalizeStructuredText(decoded, mediaType)
	default:
		return FetchResponse{}, fmt.Errorf("unsupported web page content type %q", mediaType)
	}
	if err != nil {
		return FetchResponse{}, fmt.Errorf("extract readable web page: %w", err)
	}
	if strings.TrimSpace(content) == "" {
		return FetchResponse{}, fmt.Errorf("web page contains no readable text")
	}

	characters := []rune(content)
	if request.StartIndex > len(characters) {
		return FetchResponse{}, fmt.Errorf("web fetch start_index %d exceeds content length %d", request.StartIndex, len(characters))
	}
	endIndex := request.StartIndex + maxChars
	if endIndex > len(characters) {
		endIndex = len(characters)
	}
	fragment := string(characters[request.StartIndex:endIndex])
	truncated := endIndex < len(characters)
	var nextStartIndex *int
	if truncated {
		next := endIndex
		nextStartIndex = &next
	}

	log.Printf("[webaccess] fetched public page final_url=%s response_bytes=%d content_chars=%d returned_chars=%d", safeURLForLog(finalURL), len(raw), len(characters), len([]rune(fragment)))
	return FetchResponse{
		URL:            target.String(),
		FinalURL:       finalURL.String(),
		Title:          truncateRunes(strings.TrimSpace(title), 1000),
		Byline:         truncateRunes(strings.TrimSpace(byline), 500),
		Excerpt:        truncateRunes(strings.TrimSpace(excerpt), 4000),
		ContentType:    mediaType,
		Content:        fragment,
		StartIndex:     request.StartIndex,
		EndIndex:       endIndex,
		TotalChars:     len(characters),
		Truncated:      truncated,
		NextStartIndex: nextStartIndex,
		Warning:        untrustedContentWarning,
	}, nil
}

func validateFetchURL(raw string) (*url.URL, error) {
	raw = strings.TrimSpace(raw)
	if len([]rune(raw)) > maxWebURLChars {
		return nil, fmt.Errorf("web fetch URL exceeds %d-character safety limit", maxWebURLChars)
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("parse web fetch URL: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("web fetch URL must use http or https")
	}
	if parsed.Hostname() == "" {
		return nil, fmt.Errorf("web fetch URL must include a host")
	}
	if parsed.User != nil {
		return nil, fmt.Errorf("web fetch URL must not include user credentials")
	}
	return parsed, nil
}

func readBoundedResponse(response *http.Response, maximum int64) ([]byte, error) {
	if response.ContentLength > maximum {
		return nil, fmt.Errorf("web page response exceeds %d-byte safety limit", maximum)
	}
	content, err := io.ReadAll(io.LimitReader(response.Body, maximum+1))
	if err != nil {
		return nil, fmt.Errorf("read web page response: %w", err)
	}
	if int64(len(content)) > maximum {
		return nil, fmt.Errorf("web page response exceeds %d-byte safety limit", maximum)
	}
	return content, nil
}

func responseMediaType(header string, content []byte) string {
	if mediaType, _, err := mime.ParseMediaType(header); err == nil && mediaType != "" && mediaType != "application/octet-stream" {
		return strings.ToLower(mediaType)
	}
	return strings.ToLower(strings.TrimSpace(strings.Split(http.DetectContentType(content), ";")[0]))
}

func decodeResponseText(content []byte, contentType string) (string, error) {
	reader, err := charset.NewReader(bytes.NewReader(content), contentType)
	if err != nil {
		return "", err
	}
	decoded, err := io.ReadAll(reader)
	if err != nil {
		return "", err
	}
	return string(decoded), nil
}

func extractReadableMarkdown(document string, pageURL *url.URL) (title, byline, excerpt, markdown string, err error) {
	article, readabilityErr := readability.FromReader(strings.NewReader(document), pageURL)
	if readabilityErr == nil && article.Node != nil {
		var readableHTML strings.Builder
		if renderErr := article.RenderHTML(&readableHTML); renderErr == nil {
			markdown, err = convertHTMLToMarkdown(readableHTML.String(), pageURL)
			if err == nil && strings.TrimSpace(markdown) != "" {
				return article.Title(), article.Byline(), article.Excerpt(), markdown, nil
			}
		}
	}

	title, markdown, err = fallbackHTMLToMarkdown(document, pageURL)
	if err != nil {
		if readabilityErr != nil {
			return "", "", "", "", fmt.Errorf("readability: %v; fallback: %w", readabilityErr, err)
		}
		return "", "", "", "", err
	}
	return title, "", "", markdown, nil
}

func fallbackHTMLToMarkdown(document string, pageURL *url.URL) (string, string, error) {
	parsed, err := goquery.NewDocumentFromReader(strings.NewReader(document))
	if err != nil {
		return "", "", err
	}
	title := strings.TrimSpace(parsed.Find("title").First().Text())
	selection := parsed.Find("article").First()
	if selection.Length() == 0 {
		selection = parsed.Find("main, [role='main']").First()
	}
	if selection.Length() == 0 {
		selection = parsed.Find("body").First()
	}
	if selection.Length() == 0 {
		return title, "", fmt.Errorf("HTML document has no body")
	}
	selection.Find("script, style, noscript, svg, canvas, form, button, input, textarea, select").Remove()
	htmlFragment, err := selection.Html()
	if err != nil {
		return title, "", err
	}
	markdown, err := convertHTMLToMarkdown(htmlFragment, pageURL)
	return title, markdown, err
}

func convertHTMLToMarkdown(content string, pageURL *url.URL) (string, error) {
	options := &md.Options{
		HeadingStyle:   "atx",
		CodeBlockStyle: "fenced",
		GetAbsoluteURL: func(_ *goquery.Selection, rawURL, _ string) string {
			reference, err := url.Parse(strings.TrimSpace(rawURL))
			if err != nil {
				return ""
			}
			switch strings.ToLower(reference.Scheme) {
			case "data", "javascript", "file":
				return ""
			case "", "http", "https", "mailto", "tel":
				return pageURL.ResolveReference(reference).String()
			default:
				return ""
			}
		},
	}
	converter := md.NewConverter("", true, options)
	converted, err := converter.ConvertString(content)
	if err != nil {
		return "", err
	}
	return normalizeFetchedText(converted), nil
}

func isStructuredTextMediaType(mediaType string) bool {
	return mediaType == "application/json" || mediaType == "application/xml" ||
		strings.HasSuffix(mediaType, "+json") || strings.HasSuffix(mediaType, "+xml")
}

func normalizeStructuredText(content, mediaType string) string {
	if mediaType == "application/json" || strings.HasSuffix(mediaType, "+json") {
		var compact json.RawMessage = []byte(content)
		var formatted bytes.Buffer
		if json.Valid(compact) && json.Indent(&formatted, compact, "", "  ") == nil {
			return normalizeFetchedText(formatted.String())
		}
	}
	return normalizeFetchedText(content)
}

func normalizeFetchedText(content string) string {
	content = strings.ReplaceAll(content, "\x00", "")
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.ReplaceAll(content, "\r", "\n")
	return strings.TrimSpace(content)
}

func safeURLForLog(value *url.URL) string {
	if value == nil {
		return ""
	}
	copy := *value
	copy.RawQuery = ""
	copy.Fragment = ""
	copy.User = nil
	return copy.String()
}
