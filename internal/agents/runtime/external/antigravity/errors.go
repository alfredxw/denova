package antigravity

import (
	"fmt"
	"strings"

	"denova/internal/agents/runtime/external"
)

// classifyAttemptError marks a failed attempt as retryable when its cause is a
// transient transport failure. Upstream policy rejections (content safety,
// eligibility, location) and protocol violations keep failing the attempt once.
// A resumed conversation the CLI no longer holds maps to ErrSessionUnavailable so
// the runtime rebuilds the provider session from the canonical conversation
// instead of failing the turn; that path is used when a terminated process left
// the CLI's own conversation store incomplete.
func classifyAttemptError(err error) error {
	if err == nil {
		return nil
	}
	message := strings.ToLower(err.Error())
	for _, marker := range []string{"conversation not found", "session not found", "unknown conversation", "conversation does not exist"} {
		if strings.Contains(message, marker) {
			return fmt.Errorf("%w: %w", external.ErrSessionUnavailable, err)
		}
	}
	// Content-policy refusals move ahead of the transport markers: their text is
	// user-facing and must map to a localized explanation, not to "timeout".
	for _, marker := range []string{
		"content safety", "prohibited use policy", "sensitive words",
		"blocked by content", "policy violation", "content policy",
	} {
		if strings.Contains(message, marker) {
			return external.MarkContentPolicy(err)
		}
	}
	for _, marker := range []string{
		"eof", "timeout", "deadline", "connection reset", "reset by peer",
		"broken pipe", "connection refused", "tls handshake", "bad record mac",
	} {
		if strings.Contains(message, marker) {
			return external.RetryableError(err)
		}
	}
	return err
}
