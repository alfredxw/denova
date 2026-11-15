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

func TestMediaUnsupportedClassification(t *testing.T) {
	if IsMediaUnsupported(nil) {
		t.Fatal("nil error must not be media unsupported")
	}
	if IsMediaUnsupported(errors.New("plain failure")) {
		t.Fatal("plain error must not be media unsupported")
	}
	marked := MarkMediaUnsupported(errors.New("argument budget exhausted"))
	if !IsMediaUnsupported(marked) {
		t.Fatalf("marked error not recognized: %v", marked)
	}
	if got := MarkMediaUnsupported(nil); got != nil {
		t.Fatalf("MarkMediaUnsupported(nil) = %v, want nil", got)
	}
	if key := FailureKey(marked); key != "agentRuntime.historyMediaUnsupported" {
		t.Fatalf("error key = %q", key)
	}
	if key := FailureKey(errors.Join(errors.New("context"), marked)); key != "agentRuntime.historyMediaUnsupported" {
		t.Fatalf("joined error key = %q", key)
	}
}
