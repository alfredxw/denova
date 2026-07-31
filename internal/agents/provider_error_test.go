package agents

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/alfredxw/denova/agent/providers"
)

func TestClassifyModelErrorUsesStructuredProviderStatus(t *testing.T) {
	tests := []struct {
		status    int
		class     ModelErrorClass
		retryable bool
	}{
		{status: http.StatusRequestTimeout, class: ModelErrorTimeout, retryable: true},
		{status: http.StatusConflict, class: ModelErrorConflict, retryable: true},
		{status: http.StatusTooManyRequests, class: ModelErrorRateLimited, retryable: true},
		{status: http.StatusBadGateway, class: ModelErrorUnavailable, retryable: true},
		{status: http.StatusUnauthorized, class: ModelErrorAuthentication, retryable: false},
		{status: http.StatusBadRequest, class: ModelErrorInvalidRequest, retryable: false},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprint(tt.status), func(t *testing.T) {
			err := fmt.Errorf("model call: %w", providerAPIError(tt.status))
			classification := ClassifyModelError(err)
			if classification.Class != tt.class || classification.Retryable != tt.retryable || classification.StatusCode != tt.status {
				t.Fatalf("classification = %+v", classification)
			}
		})
	}
}

func TestTransientModelErrorStopsWhenRunContextIsCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := providerAPIError(http.StatusTooManyRequests)
	if isTransientModelError(ctx, err) {
		t.Fatal("cancelled run must not schedule another provider retry")
	}
}

func providerAPIError(status int) *providers.APIError {
	return &providers.APIError{StatusCode: status}
}

func TestClassifyModelErrorRecognizesWrappedContextCancellation(t *testing.T) {
	classification := ClassifyModelError(fmt.Errorf("stream: %w", context.Canceled))
	if classification.Class != ModelErrorCancelled || classification.Retryable {
		t.Fatalf("classification = %+v", classification)
	}
	classification = ClassifyModelError(errors.New("qpm limit reached"))
	if classification.Class != ModelErrorRateLimited || !classification.Retryable {
		t.Fatalf("compatibility classification = %+v", classification)
	}
}
