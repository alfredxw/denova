package antigravity

import (
	"strings"

	"denova/internal/agents/runtime/external"
)

// classifyAttemptError marks a failed attempt as retryable when its cause is a
// transient transport failure. Upstream policy rejections (content safety,
// eligibility, location) and protocol violations keep failing the attempt once.
func classifyAttemptError(err error) error {
	if err == nil {
		return nil
	}
	message := strings.ToLower(err.Error())
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
