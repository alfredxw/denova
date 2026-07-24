package openai

import (
	"errors"
	"fmt"
	"net/http"

	sdk "github.com/openai/openai-go/v3"
)

// APIError represents an HTTP error returned by an OpenAI-compatible API.
// StatusCode is the stable adapter contract; SDK-specific error details remain
// available through the standard error chain without becoming part of this
// package's public types.
type APIError struct {
	StatusCode int
	cause      error
}

func (err *APIError) Error() string {
	if err == nil {
		return "openai API request failed"
	}
	if err.cause != nil {
		return err.cause.Error()
	}
	if status := http.StatusText(err.StatusCode); status != "" {
		return fmt.Sprintf("openai API request failed: %d %s", err.StatusCode, status)
	}
	if err.StatusCode != 0 {
		return fmt.Sprintf("openai API request failed: status %d", err.StatusCode)
	}
	return "openai API request failed"
}

func (err *APIError) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.cause
}

func adaptAPIError(err error) error {
	if err == nil {
		return nil
	}
	var adapterError *APIError
	if errors.As(err, &adapterError) {
		return err
	}
	var sdkError *sdk.Error
	if !errors.As(err, &sdkError) {
		return err
	}
	return &APIError{
		StatusCode: sdkError.StatusCode,
		cause:      err,
	}
}
