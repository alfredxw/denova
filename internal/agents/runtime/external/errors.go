package external

import (
	"errors"
	"fmt"
)

// Retryable marks a failed provider attempt whose cause is transient — a
// transport error or an upstream rate limit — so the product controller may
// re-attempt it a bounded number of times instead of suspending the task.
// Policy rejections, protocol violations and unknown states stay non-retryable.
type Retryable struct{ Err error }

func (e *Retryable) Error() string {
	if e.Err == nil {
		return ""
	}
	return e.Err.Error()
}

// Unwrap keeps the wrapped cause visible to errors.Is/As and diagnostics.
func (e *Retryable) Unwrap() error { return e.Err }

// RetryableError wraps err so IsRetryable reports true. A nil err returns nil.
func RetryableError(err error) error {
	if err == nil {
		return nil
	}
	return &Retryable{Err: err}
}

// IsRetryable reports whether err is marked as a transient, retryable failure.
func IsRetryable(err error) bool {
	var retryable *Retryable
	return errors.As(err, &retryable)
}

// ErrContentPolicyRejected marks an upstream refusal for content-policy reasons.
// The product shows a localized explanation instead of the provider's raw policy
// text, and such a refusal is never retried.
var ErrContentPolicyRejected = errors.New("provider rejected the request by content policy")

// MarkContentPolicy wraps err so callers can surface a localized message. A nil
// err returns nil.
func MarkContentPolicy(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%w: %w", ErrContentPolicyRejected, err)
}

// IsContentPolicyRejected reports whether err carries a provider refusal.
func IsContentPolicyRejected(err error) bool {
	return errors.Is(err, ErrContentPolicyRejected)
}

// ErrTurnNotConverged marks a turn whose consecutive provider attempts produced
// neither a host tool call nor new narrative. The loop cannot make progress, so
// the product ends the turn instead of prompting the engine forever.
var ErrTurnNotConverged = errors.New("game turn made no progress")

// FailureKey selects the product error key for a failed external operation.
func FailureKey(err error) string {
	switch {
	case IsContentPolicyRejected(err):
		return "agentRuntime.contentPolicyRejected"
	case errors.Is(err, ErrTurnNotConverged):
		return "agentRuntime.turnIncomplete"
	}
	return "agentRuntime.operationFailed"
}
