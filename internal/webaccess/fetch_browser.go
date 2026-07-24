package webaccess

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// browserRenderer is the process boundary for executing an untrusted public
// page. Production implementations must isolate page state and enforce the
// same public-network-only policy as direct HTTP fetching.
type browserRenderer interface {
	Render(context.Context, *url.URL) (renderedPage, error)
}

type renderedPage struct {
	FinalURL   *url.URL
	HTML       string
	HTTPStatus int
}

func (client *Client) fetchWithBrowser(ctx context.Context, target *url.URL) (fetchedDocument, FetchAttempt, error) {
	if client.browserRenderer == nil {
		return fetchedDocument{}, FetchAttempt{
			Method: FetchMethodBrowser, Outcome: FetchAttemptProviderUnavailable,
		}, fmt.Errorf("browser renderer is unavailable")
	}
	page, err := client.browserRenderer.Render(ctx, target)
	if err != nil {
		attempt := FetchAttempt{Method: FetchMethodBrowser}
		if ctx.Err() == nil {
			attempt.Outcome = FetchAttemptProviderUnavailable
		}
		return fetchedDocument{}, attempt, err
	}
	status := page.HTTPStatus
	if status == 0 {
		status = http.StatusOK
	}
	if status < http.StatusOK || status >= http.StatusMultipleChoices {
		attempt := FetchAttempt{Method: FetchMethodBrowser, HTTPStatus: status}
		if status == http.StatusForbidden {
			attempt.Outcome = FetchAttemptAccessDenied
		}
		return fetchedDocument{}, attempt, fmt.Errorf("browser-rendered page returned HTTP %d", status)
	}
	if int64(len(page.HTML)) > client.config.FetchMaxResponseBytes {
		return fetchedDocument{}, FetchAttempt{Method: FetchMethodBrowser, HTTPStatus: status}, fmt.Errorf("browser-rendered page exceeds %d-byte safety limit", client.config.FetchMaxResponseBytes)
	}
	finalURL := page.FinalURL
	if finalURL == nil {
		finalURL = target
	}
	if len([]rune(finalURL.String())) > maxWebURLChars {
		return fetchedDocument{}, FetchAttempt{Method: FetchMethodBrowser, HTTPStatus: status}, fmt.Errorf("final browser page URL exceeds %d-character safety limit", maxWebURLChars)
	}
	title, byline, excerpt, content, err := extractReadableMarkdown(page.HTML, finalURL)
	if err != nil {
		return fetchedDocument{}, FetchAttempt{Method: FetchMethodBrowser, HTTPStatus: status}, fmt.Errorf("extract browser-rendered page: %w", err)
	}
	if strings.TrimSpace(content) == "" {
		return fetchedDocument{}, FetchAttempt{Method: FetchMethodBrowser, HTTPStatus: status}, fmt.Errorf("browser-rendered page contains no readable text")
	}
	return fetchedDocument{
		finalURL: finalURL, title: title, byline: byline, excerpt: excerpt,
		contentType: "text/html", content: content, responseSize: len(page.HTML),
	}, FetchAttempt{Method: FetchMethodBrowser, Outcome: FetchAttemptSuccess, HTTPStatus: status}, nil
}
