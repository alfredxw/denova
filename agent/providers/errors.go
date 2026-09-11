package providers

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// APIError is the protocol-neutral HTTP error exposed by adapters. Product
// retry policy depends on StatusCode rather than a vendor SDK error type.
type APIError struct {
	StatusCode int
	Code       string
	Kind       string
	RetryAfter time.Duration
	RequestID  string
	Message    string
	Cause      error
}

// Retryable classifies structured provider failures without parsing prose.
func (err *APIError) Retryable() bool {
	if err == nil {
		return false
	}
	for _, value := range []string{err.Code, err.Kind} {
		switch strings.ToLower(value) {
		case "insufficient_quota", "billing_hard_limit_reached", "credit_balance_too_low", "authentication_error", "permission_error", "invalid_api_key", "invalid_request_error":
			return false
		}
	}
	for _, value := range []string{err.Code, err.Kind} {
		switch strings.ToLower(value) {
		case "rate_limit_exceeded", "rate_limit_error", "server_error", "overloaded_error":
			return true
		}
	}
	return err.StatusCode == http.StatusRequestTimeout || err.StatusCode == http.StatusConflict ||
		err.StatusCode == http.StatusTooManyRequests || err.StatusCode >= http.StatusInternalServerError
}

func (err *APIError) RetryDelay() time.Duration {
	if err == nil {
		return 0
	}
	return max(0, err.RetryAfter)
}

// RetryAfterDelay reads the provider's standard delay hint. Invalid and past
// values are ignored; this delay never becomes a total model execution limit.
func RetryAfterDelay(response *http.Response) time.Duration {
	if response == nil {
		return 0
	}
	if millis, err := strconv.ParseFloat(response.Header.Get("retry-after-ms"), 64); err == nil && millis > 0 && millis < float64(time.Duration(1<<63-1)/time.Millisecond) {
		return time.Duration(millis * float64(time.Millisecond))
	}
	value := response.Header.Get("Retry-After")
	if seconds, err := strconv.ParseFloat(value, 64); err == nil && seconds > 0 && seconds < float64(time.Duration(1<<63-1)/time.Second) {
		return time.Duration(seconds * float64(time.Second))
	}
	if until, err := http.ParseTime(value); err == nil {
		return max(0, time.Until(until))
	}
	return 0
}

func (err *APIError) Error() string {
	if err == nil {
		return "provider API error"
	}
	detail := err.Message
	if detail == "" && err.Cause != nil {
		detail = err.Cause.Error()
	}
	if detail == "" {
		detail = "request failed"
	}
	metadata := make([]string, 0, 2)
	if err.StatusCode != 0 {
		metadata = append(metadata, fmt.Sprintf("status %d", err.StatusCode))
	}
	if err.RequestID != "" {
		metadata = append(metadata, "request "+err.RequestID)
	}
	if len(metadata) != 0 {
		return fmt.Sprintf("provider API error (%s): %s", strings.Join(metadata, ", "), detail)
	}
	return "provider API error: " + detail
}

func (err *APIError) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.Cause
}
