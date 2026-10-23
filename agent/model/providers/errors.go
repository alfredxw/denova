package providers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	agentschema "github.com/alfredxw/denova/agent/schema"
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

// streamErrorKind is the classification used when an accepted stream fails
// without a provider-supplied code or type. A 2xx stream only aborts on a
// server-side failure, so keeping it retryable is safer than losing the run;
// explicit non-retryable provider classes still opt out through Retryable.
const streamErrorKind = "server_error"

// NewStreamAPIError promotes an error event delivered inside a successful
// response stream into the protocol-neutral APIError. The HTTP status is no
// longer available at that point, so retry classification depends on the
// payload's code and type instead. Providers only send such an event after the
// request was accepted, which is why an unclassified payload stays retryable.
func NewStreamAPIError(cause error, payload []byte) *APIError {
	apiError := &APIError{Cause: cause}
	if cause != nil {
		apiError.Message = strings.TrimSpace(cause.Error())
	}
	var envelope struct {
		Error struct {
			Code    json.RawMessage `json:"code"`
			Type    json.RawMessage `json:"type"`
			Message json.RawMessage `json:"message"`
		} `json:"error"`
	}
	if len(payload) != 0 && json.Unmarshal(payload, &envelope) == nil {
		apiError.Code = jsonScalarText(envelope.Error.Code)
		apiError.Kind = jsonScalarText(envelope.Error.Type)
		if message := jsonScalarText(envelope.Error.Message); message != "" {
			apiError.Message = message
		}
	}
	// Some relays repeat the upstream HTTP status in the code field.
	if status, err := strconv.Atoi(apiError.Code); err == nil && status >= 100 && status <= 599 {
		apiError.StatusCode = status
		apiError.Code = ""
	}
	if apiError.Code == "" && apiError.Kind == "" && apiError.StatusCode == 0 {
		apiError.Kind = streamErrorKind
	}
	return apiError
}

// jsonScalarText reads the string, number, or boolean form providers use for
// error codes. Nested payloads are ignored because they carry no classification.
func jsonScalarText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return strings.TrimSpace(text)
	}
	var scalar any
	if err := json.Unmarshal(raw, &scalar); err != nil {
		return ""
	}
	switch value := scalar.(type) {
	case float64:
		return strconv.FormatFloat(value, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(value)
	default:
		return ""
	}
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

// ModelErrorReason supplies a stable terminal reason for live and recovered
// runs. The original Error remains available to provider diagnostics.
func (err *APIError) ModelErrorReason() string {
	if err == nil {
		return ""
	}
	if err.Kind == "image_input_error" {
		return agentschema.ModelImageInputRejectedReason
	}
	if err.StatusCode == http.StatusRequestEntityTooLarge {
		return agentschema.ModelRequestTooLargeReason
	}
	return ""
}
