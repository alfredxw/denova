package webaccess

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// browserRenderer is the process boundary for executing an untrusted public
// page. Production implementations must isolate page state and enforce the
// same public-network-only policy as direct HTTP fetching.
type browserRenderer interface {
	Render(context.Context, *url.URL) (renderedPage, error)
}

type browserRenderMode string

const (
	browserRenderModeStandard browserRenderMode = "standard"
	browserRenderModeStealth  browserRenderMode = "stealth"
	browserChallengeMessage                     = "The browser rendered an access-challenge page. 浏览器最终仍停留在访问验证页面。"
)

type renderedPage struct {
	FinalURL             *url.URL
	HTML                 string
	HTTPStatus           int
	RenderMode           browserRenderMode
	StealthFallbackError error
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
	accessChallengeStatus := browserStatusIndicatesAccessChallenge(status)
	if (status < http.StatusOK || status >= http.StatusMultipleChoices) && !accessChallengeStatus {
		return fetchedDocument{}, FetchAttempt{Method: FetchMethodBrowser, HTTPStatus: status}, fmt.Errorf("browser-rendered page returned HTTP %d", status)
	}
	title, byline, excerpt, content, err := extractReadableMarkdown(page.HTML, finalURL)
	challengeDocument := isLikelyBrowserChallengeDocument(page.HTML)
	if challengeDocument || (accessChallengeStatus && (err != nil || !isSubstantiveBrowserContent(title, content, page.HTML))) {
		message := browserChallengeMessage
		if page.RenderMode == browserRenderModeStealth {
			message = "The go-rod/stealth fallback still rendered an access-challenge page. go-rod/stealth 兜底后仍停留在访问验证页面。"
		} else if page.StealthFallbackError != nil {
			message = "The standard browser rendered an access challenge and the go-rod/stealth fallback failed: " + truncateRunes(page.StealthFallbackError.Error(), 300) + " 普通浏览器遇到访问验证，且 go-rod/stealth 兜底执行失败。"
		}
		return fetchedDocument{}, FetchAttempt{
			Method: FetchMethodBrowser, Outcome: FetchAttemptAccessDenied, HTTPStatus: status, Message: message,
		}, fmt.Errorf("browser-rendered page remained behind an access challenge")
	}
	if err != nil {
		return fetchedDocument{}, FetchAttempt{Method: FetchMethodBrowser, HTTPStatus: status}, fmt.Errorf("extract browser-rendered page: %w", err)
	}
	if strings.TrimSpace(content) == "" {
		return fetchedDocument{}, FetchAttempt{Method: FetchMethodBrowser, HTTPStatus: status}, fmt.Errorf("browser-rendered page contains no readable text")
	}
	attempt := FetchAttempt{Method: FetchMethodBrowser, Outcome: FetchAttemptSuccess, HTTPStatus: status}
	if page.RenderMode == browserRenderModeStealth {
		attempt.Message = "Rendered with go-rod/stealth after the standard browser attempt was challenged. 普通浏览器遇到验证后，已通过 go-rod/stealth 渲染正文。"
	}
	if accessChallengeStatus {
		attempt.Message = "The browser returned an access-challenge status but produced a substantive readable document. 浏览器导航返回访问验证状态，但最终已生成可信的可读正文。"
		if page.RenderMode == browserRenderModeStealth {
			attempt.Message = "The go-rod/stealth fallback produced a substantive readable document after an access-challenge response. go-rod/stealth 在访问验证响应后成功生成了可读正文。"
		}
	}
	return fetchedDocument{
		finalURL: finalURL, title: title, byline: byline, excerpt: excerpt,
		contentType: "text/html", content: content, responseSize: len(page.HTML),
	}, attempt, nil
}

func browserStatusIndicatesAccessChallenge(status int) bool {
	switch status {
	case http.StatusUnauthorized, http.StatusForbidden, http.StatusProxyAuthRequired,
		http.StatusTooManyRequests, http.StatusUnavailableForLegalReasons:
		return true
	default:
		return false
	}
}

func isLikelyBrowserChallengeDocument(document string) bool {
	parsed, err := goquery.NewDocumentFromReader(strings.NewReader(document))
	if err != nil {
		return false
	}
	challengeNodeFound := parsed.Find(strings.Join([]string{
		"meta#zh-zse-ck",
		"script[src*='/zse-ck/']",
		"script[src*='/challenge-platform/']",
		"[id*='captcha']",
		"[class*='g-recaptcha']",
		"[class*='h-captcha']",
		"[class*='geetest']",
	}, ", ")).Length() > 0
	title := strings.ToLower(strings.TrimSpace(parsed.Find("title").First().Text()))
	parsed.Find("script, style, noscript, svg, canvas").Remove()
	visible := strings.ToLower(strings.TrimSpace(parsed.Find("body").Text()))
	// A completed challenge can leave its bootstrap nodes in the DOM. Once the
	// page has substantial visible content, extraction quality is a better signal
	// than the stale challenge marker.
	if len([]rune(visible)) >= 500 {
		return false
	}
	if challengeNodeFound {
		return true
	}
	for _, marker := range []string{
		"access denied", "just a moment", "verify you are human", "security verification",
		"安全验证", "人机验证", "访问异常", "请输入验证码",
	} {
		if strings.Contains(title, marker) || strings.Contains(visible, marker) {
			return true
		}
	}
	return false
}

func isSubstantiveBrowserContent(title, content, document string) bool {
	if len([]rune(strings.TrimSpace(content))) < 300 {
		return false
	}
	if strings.TrimSpace(title) != "" {
		return true
	}
	parsed, err := goquery.NewDocumentFromReader(strings.NewReader(document))
	if err != nil {
		return false
	}
	return parsed.Find("article, main").Length() > 0 || parsed.Find("p").Length() >= 2
}
