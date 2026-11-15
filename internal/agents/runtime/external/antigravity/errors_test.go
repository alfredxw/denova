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

// A terminated process can leave the CLI's own conversation store incomplete.
// Resuming it must rebuild from the canonical conversation rather than fail the
// Game turn, so the lost-session wording maps to external.ErrSessionUnavailable.
func TestClassifyAttemptErrorMapsLostConversation(t *testing.T) {
	for _, message := range []string{
		"Antigravity attempt ended with status ERROR: conversation not found",
		"resume failed: session not found",
	} {
		err := classifyAttemptError(errors.New(message))
		if !errors.Is(err, external.ErrSessionUnavailable) {
			t.Fatalf("expected session unavailable: %q got %v", message, err)
		}
		if external.IsRetryable(err) {
			t.Fatalf("lost conversation must not be retryable: %q", message)
		}
	}
	if errors.Is(classifyAttemptError(errors.New("plain failure")), external.ErrSessionUnavailable) {
		t.Fatal("unrelated failure mapped to a lost conversation")
	}
}

// A content-policy refusal is user-facing: it must be recognizable so the product
// shows localized copy instead of the provider's raw policy text, and it must
// never be retried.
func TestClassifyAttemptErrorMarksContentPolicyRefusal(t *testing.T) {
	err := classifyAttemptError(errors.New("Antigravity attempt ended with status ERROR: Your previous response was blocked by content safety filters: The model output could not be generated."))
	if !external.IsContentPolicyRejected(err) {
		t.Fatalf("content policy refusal was not classified: %v", err)
	}
	if external.IsRetryable(err) {
		t.Fatalf("content policy refusal must not be retryable: %v", err)
	}
	if key := external.FailureKey(err); key != "agentRuntime.contentPolicyRejected" {
		t.Fatalf("error key = %q", key)
	}
}
