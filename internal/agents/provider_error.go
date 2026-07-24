package agents

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"

	agentopenai "github.com/alfredxw/denova/agent/model/openai"
)

type ModelErrorClass string

const (
	ModelErrorUnknown        ModelErrorClass = "unknown"
	ModelErrorCancelled      ModelErrorClass = "cancelled"
	ModelErrorTimeout        ModelErrorClass = "timeout"
	ModelErrorConflict       ModelErrorClass = "conflict"
	ModelErrorRateLimited    ModelErrorClass = "rate_limited"
	ModelErrorUnavailable    ModelErrorClass = "unavailable"
	ModelErrorAuthentication ModelErrorClass = "authentication"
	ModelErrorInvalidRequest ModelErrorClass = "invalid_request"
)

// ModelErrorClassification is a transport-neutral provider failure contract.
// Retry policy consumes this value instead of parsing human-readable errors.
type ModelErrorClassification struct {
	Class      ModelErrorClass `json:"class"`
	StatusCode int             `json:"status_code,omitempty"`
	Retryable  bool            `json:"retryable"`
}

func ClassifyModelError(err error) ModelErrorClassification {
	if err == nil {
		return ModelErrorClassification{}
	}
	if errors.Is(err, context.Canceled) {
		return ModelErrorClassification{Class: ModelErrorCancelled}
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return ModelErrorClassification{Class: ModelErrorTimeout, Retryable: true}
	}

	var providerError *agentopenai.APIError
	if errors.As(err, &providerError) {
		return classifyModelHTTPStatus(providerError.StatusCode)
	}
	var networkError net.Error
	if errors.As(err, &networkError) {
		if networkError.Timeout() {
			return ModelErrorClassification{Class: ModelErrorTimeout, Retryable: true}
		}
		if networkError.Temporary() {
			return ModelErrorClassification{Class: ModelErrorUnavailable, Retryable: true}
		}
	}

	// Some OpenAI-compatible providers still erase their structured error at an
	// adapter boundary. Keep a deliberately narrow compatibility fallback; new
	// providers should expose a typed status instead of adding broad substrings.
	normalized := strings.ToLower(err.Error())
	if strings.Contains(normalized, "qpm limit") || strings.Contains(normalized, "too many requests") {
		return ModelErrorClassification{Class: ModelErrorRateLimited, StatusCode: http.StatusTooManyRequests, Retryable: true}
	}
	return ModelErrorClassification{Class: ModelErrorUnknown}
}

func classifyModelHTTPStatus(status int) ModelErrorClassification {
	classification := ModelErrorClassification{StatusCode: status}
	switch {
	case status == http.StatusRequestTimeout:
		classification.Class = ModelErrorTimeout
		classification.Retryable = true
	case status == http.StatusConflict:
		classification.Class = ModelErrorConflict
		classification.Retryable = true
	case status == http.StatusTooManyRequests:
		classification.Class = ModelErrorRateLimited
		classification.Retryable = true
	case status >= http.StatusInternalServerError:
		classification.Class = ModelErrorUnavailable
		classification.Retryable = true
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		classification.Class = ModelErrorAuthentication
	case status >= http.StatusBadRequest:
		classification.Class = ModelErrorInvalidRequest
	default:
		classification.Class = ModelErrorUnknown
	}
	return classification
}

func (c ModelErrorClassification) String() string {
	if c.StatusCode == 0 {
		return string(c.Class)
	}
	return fmt.Sprintf("%s (%d)", c.Class, c.StatusCode)
}
