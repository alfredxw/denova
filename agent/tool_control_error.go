package agent

import "errors"

// ToolControlError marks lifecycle or coordination failures that must terminate
// scheduling instead of being offered to the model as an ordinary tool error.
// The attached ToolResult, when present, is still paired into the transcript.
type ToolControlError struct{ Err error }

func (err *ToolControlError) Error() string {
	if err == nil || err.Err == nil {
		return "tool control failure"
	}
	return err.Err.Error()
}

func (err *ToolControlError) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.Err
}

// MarkToolControlError preserves an existing marker or wraps err.
func MarkToolControlError(err error) error {
	if err == nil || IsToolControlError(err) {
		return err
	}
	return &ToolControlError{Err: err}
}

// IsToolControlError reports whether scheduling must stop after this error.
func IsToolControlError(err error) bool {
	var control *ToolControlError
	return errors.As(err, &control)
}
