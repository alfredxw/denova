package external

import "errors"

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
