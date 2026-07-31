package openaichatcompletions

import (
	"errors"
	"strings"

	sdk "github.com/openai/openai-go/v3"

	"github.com/alfredxw/denova/agent/providers"
)

func adaptAPIError(err error) error {
	if err == nil {
		return nil
	}
	var adapterError *providers.APIError
	if errors.As(err, &adapterError) {
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
