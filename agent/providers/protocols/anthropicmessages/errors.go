package anthropicmessages

import (
	"errors"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"

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
	var sdkError *anthropic.Error
	if !errors.As(err, &sdkError) {
		return err
	}
	return &providers.APIError{
		StatusCode: sdkError.StatusCode,
		RequestID:  sdkError.RequestID,
		Message:    strings.TrimSpace(err.Error()),
		Cause:      err,
	}
}
