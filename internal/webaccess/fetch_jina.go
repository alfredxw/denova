package webaccess

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

const defaultJinaReaderBaseURL = "https://r.jina.ai/"

type jinaReaderResponse struct {
	Data struct {
		Title          string `json:"title"`
		Description    string `json:"description"`
		URL            string `json:"url"`
		Content        string `json:"content"`
		Warning        string `json:"warning"`
		HTTPStatus     int    `json:"httpStatus"`
		HTTPStatusText string `json:"httpStatusText"`
	} `json:"data"`
}

func (client *Client) fetchWithJinaReader(ctx context.Context, target *url.URL) (fetchedDocument, FetchAttempt, error) {
	endpoint := strings.TrimRight(client.jinaReaderBaseURL, "/") + "/" + target.String()
	method := http.MethodGet
	var body *bytes.Reader
	if target.Fragment != "" {
		payload, err := json.Marshal(struct {
			URL string `json:"url"`
		}{URL: target.String()})
		if err != nil {
			return fetchedDocument{}, FetchAttempt{Method: FetchMethodJinaReader}, fmt.Errorf("encode Jina Reader request: %w", err)
		}
		method = http.MethodPost
		endpoint = strings.TrimRight(client.jinaReaderBaseURL, "/") + "/"
		body = bytes.NewReader(payload)
	} else {
		body = bytes.NewReader(nil)
	}
	request, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return fetchedDocument{}, FetchAttempt{Method: FetchMethodJinaReader}, fmt.Errorf("create Jina Reader request: %w", err)
	}
	if method == http.MethodPost {
		request.Header.Set("Content-Type", "application/json")
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("X-No-Cache", "true")
	request.Header.Set("User-Agent", webAccessUserAgent)

	response, err := client.jinaHTTPClient.Do(request)
	if err != nil {
		attempt := FetchAttempt{Method: FetchMethodJinaReader}
		if ctx.Err() == nil {
			attempt.Outcome = FetchAttemptProviderUnavailable
		}
		return fetchedDocument{}, attempt, fmt.Errorf("request Jina Reader: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return fetchedDocument{}, FetchAttempt{
			Method: FetchMethodJinaReader, Outcome: FetchAttemptProviderUnavailable, HTTPStatus: response.StatusCode,
		}, fmt.Errorf("Jina Reader returned HTTP %d", response.StatusCode)
	}
	raw, err := readBoundedResponse(response, client.config.FetchMaxResponseBytes)
	if err != nil {
		return fetchedDocument{}, FetchAttempt{
			Method: FetchMethodJinaReader, Outcome: FetchAttemptProviderUnavailable, HTTPStatus: response.StatusCode,
		}, fmt.Errorf("read Jina Reader response: %w", err)
	}
	var decoded jinaReaderResponse
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return fetchedDocument{}, FetchAttempt{
			Method: FetchMethodJinaReader, Outcome: FetchAttemptProviderUnavailable, HTTPStatus: response.StatusCode,
		}, fmt.Errorf("decode Jina Reader response: %w", err)
	}
	targetStatus := decoded.Data.HTTPStatus
	if targetStatus == 0 {
		targetStatus = http.StatusOK
	}
	if targetStatus < http.StatusOK || targetStatus >= http.StatusMultipleChoices {
		attempt := FetchAttempt{Method: FetchMethodJinaReader, HTTPStatus: targetStatus}
		if targetStatus == http.StatusForbidden {
			attempt.Outcome = FetchAttemptAccessDenied
		}
		return fetchedDocument{}, attempt, fmt.Errorf("Jina Reader target returned HTTP %d", targetStatus)
	}
	if jinaWarningIndicatesAccessDenied(decoded.Data.Warning) {
		return fetchedDocument{}, FetchAttempt{
			Method: FetchMethodJinaReader, Outcome: FetchAttemptAccessDenied, HTTPStatus: targetStatus,
		}, fmt.Errorf("Jina Reader reported a target access challenge")
	}
	content := normalizeFetchedText(decoded.Data.Content)
	if content == "" {
		return fetchedDocument{}, FetchAttempt{
			Method: FetchMethodJinaReader, Outcome: FetchAttemptProviderUnavailable, HTTPStatus: targetStatus,
		}, fmt.Errorf("Jina Reader returned no readable content")
	}
	finalURL := target
	if candidate, parseErr := validateFetchURL(decoded.Data.URL); parseErr == nil {
		finalURL = candidate
	}

	return fetchedDocument{
		finalURL: finalURL, title: decoded.Data.Title, excerpt: decoded.Data.Description,
		contentType: "text/markdown", content: content, responseSize: len(raw),
	}, FetchAttempt{Method: FetchMethodJinaReader, Outcome: FetchAttemptSuccess, HTTPStatus: targetStatus}, nil
}

func jinaWarningIndicatesAccessDenied(warning string) bool {
	normalized := strings.ToLower(strings.TrimSpace(warning))
	if normalized == "" {
		return false
	}
	for _, marker := range []string{
		"403", "access denied", "blocked", "captcha", "challenge", "forbidden",
		"robot check", "security verification", "人机验证", "安全验证", "访问被拒绝", "验证码",
	} {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	return false
}
