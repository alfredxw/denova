package antigravity

import (
	"errors"
	"testing"

	"denova/internal/agents/runtime/external"
)

func TestClassifyAttemptError(t *testing.T) {
	retryable := []string{
		"streamGenerateContent: EOF",
		"unexpected EOF",
		"TLS handshake timeout",
		"local error: tls: bad record MAC",
		"connection reset by peer",
		"context deadline exceeded",
	}
	for _, message := range retryable {
		if !external.IsRetryable(classifyAttemptError(errors.New(message))) {
			t.Fatalf("expected retryable: %q", message)
		}
	}
	hard := []string{
		"Antigravity attempt ended with status ERROR: Your previous response was blocked by content safety filters",
		"Antigravity attempt ended with status ERROR: FAILED_PRECONDITION (code 400): User location is not supported for the API use.",
		"Antigravity attempt ended with status ERROR: failed to construct executor: unknown component: tool \"call_mcp_tool\" not found in registry",
		"unknown Antigravity step state \"ERROR\"",
	}
	for _, message := range hard {
		if external.IsRetryable(classifyAttemptError(errors.New(message))) {
			t.Fatalf("expected non-retryable: %q", message)
		}
	}
	if classifyAttemptError(nil) != nil {
		t.Fatal("nil error must stay nil")
	}
}
