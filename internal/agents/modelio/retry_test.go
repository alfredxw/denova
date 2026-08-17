package modelio

import (
	"context"
	"net/http"
	"testing"

	"github.com/alfredxw/denova/agent/providers"
)

func TestTransientModelErrorStopsWhenRunContextIsCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := &providers.APIError{StatusCode: http.StatusTooManyRequests}
	if IsRetryable(ctx, err) {
		t.Fatal("cancelled run must not schedule another provider retry")
	}
}
