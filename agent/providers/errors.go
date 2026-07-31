package providers

import (
	"fmt"
	"strings"
)

// APIError is the protocol-neutral HTTP error exposed by adapters. Product
// retry policy depends on StatusCode rather than a vendor SDK error type.
type APIError struct {
	StatusCode int
	RequestID  string
	Message    string
	Cause      error
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
