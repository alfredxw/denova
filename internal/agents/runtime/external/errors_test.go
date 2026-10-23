package external

import (
	"errors"
	"testing"
)

func TestRetryableErrorClassification(t *testing.T) {
	if IsRetryable(nil) {
		t.Fatal("nil error must not be retryable")
	}
	if IsRetryable(errors.New("plain failure")) {
		t.Fatal("plain error must not be retryable")
	}
	if !IsRetryable(RetryableError(errors.New("transient"))) {
		t.Fatal("wrapped error must be retryable")
	}
	joined := errors.Join(errors.New("context"), RetryableError(errors.New("transient")))
	if !IsRetryable(joined) {
		t.Fatal("joined error must preserve retryability")
	}
	if got := RetryableError(nil); got != nil {
		t.Fatalf("RetryableError(nil) = %v, want nil", got)
	}
}
