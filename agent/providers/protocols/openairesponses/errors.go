package openairesponses

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	sdk "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"

	"github.com/alfredxw/denova/agent/providers"
)

func adaptAPIError(err error) error {
	if err == nil {
		return nil
	}
	var providerError *providers.APIError
	if errors.As(err, &providerError) {
		return err
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
		RequestID:  requestID,
		Message:    strings.TrimSpace(err.Error()),
		Cause:      err,
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
		Message:   message,
	}
}
