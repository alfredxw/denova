package adk

import (
	"encoding/json"
	"errors"
)

// InterruptError is a transport-neutral request to stop and potentially rerun
// work with host-provided resume data.
type InterruptError struct {
	Reason      string          `json:"reason"`
	ResumeToken json.RawMessage `json:"resume_token,omitempty"`
}

func (err *InterruptError) Error() string {
	if err == nil || err.Reason == "" {
		return "agent interrupted"
	}
	return "agent interrupted: " + err.Reason
}

// IsInterruptError reports whether err contains an InterruptError.
func IsInterruptError(err error) bool {
	var interrupt *InterruptError
	return errors.As(err, &interrupt)
}
