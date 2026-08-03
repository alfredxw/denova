package googlegenerativeai

import (
	"errors"
	"strings"

	"google.golang.org/genai"

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
	var sdkError genai.APIError
	if !errors.As(err, &sdkError) {
		return err
	}
	return &providers.APIError{
		StatusCode: sdkError.Code,
		Message:    strings.TrimSpace(sdkError.Message),
		Cause:      err,
	}
}
