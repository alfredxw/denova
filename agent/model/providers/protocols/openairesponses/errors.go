package openairesponses

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/alfredxw/denova/agent/model/providers"
	sdk "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/ssestream"
	"github.com/openai/openai-go/v3/responses"
)

func adaptAPIError(err error) error {
	if err == nil {
		return nil
	}
	var providerError *providers.APIError
	if errors.As(err, &providerError) {
		return err
	}
	// The SDK reports an in-stream error event as its own type after a 200
	// response, so the status needed for retry classification must come from
	// the event payload instead of the vendor error.
	var streamError *ssestream.StreamError
	if errors.As(err, &streamError) {
		return providers.NewStreamAPIError(streamError, streamError.Event.Data)
	}
	var sdkError *sdk.Error
	if !errors.As(err, &sdkError) {
		return err
	}
	requestID := ""
	if sdkError.Response != nil {
		requestID = responseRequestID(sdkError.Response)
	}
	return &providers.APIError{
		StatusCode: sdkError.StatusCode,
		Code:       sdkError.Code, Kind: sdkError.Type, RetryAfter: providers.RetryAfterDelay(sdkError.Response),
		RequestID: requestID,
		Message:   strings.TrimSpace(err.Error()),
		Cause:     err,
	}
}

func responseFailure(response *responses.Response, rawResponse *http.Response) error {
	if response == nil {
		return fmt.Errorf("openai responses: nil response")
	}
	if response.Status != responses.ResponseStatusFailed {
		return nil
	}
	message := strings.TrimSpace(response.Error.Message)
	if response.Error.Code != "" {
		if message != "" {
			message = string(response.Error.Code) + ": " + message
		} else {
			message = string(response.Error.Code)
		}
	}
	return &providers.APIError{
		RequestID: responseRequestID(rawResponse),
		Code:      string(response.Error.Code), RetryAfter: providers.RetryAfterDelay(rawResponse),
		Message: message,
	}
}
