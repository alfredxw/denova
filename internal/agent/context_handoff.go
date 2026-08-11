package agent

import "fmt"

const DefaultContextHandoffLimitBytes = 256 * 1024

// ContextHandoffLimitError reports a request whose explicit model-visible
// handoff exceeds the configured byte boundary.
type ContextHandoffLimitError struct {
	Bytes int
	Limit int
}

func (e *ContextHandoffLimitError) Error() string {
	return fmt.Sprintf("context handoff exceeds byte limit: %d > %d", e.Bytes, e.Limit)
}

// ValidateContextHandoffs measures the exact encoded handoff block that would
// be appended to the model-visible user message.
func ValidateContextHandoffs(selections []TextSelectionRef, limit int) error {
	if len(selections) == 0 {
		return nil
	}
	if limit <= 0 {
		limit = DefaultContextHandoffLimitBytes
	}
	encodedBytes := len(appendSelectionContext("", selections))
	if encodedBytes > limit {
		return &ContextHandoffLimitError{Bytes: encodedBytes, Limit: limit}
	}
	return nil
}
