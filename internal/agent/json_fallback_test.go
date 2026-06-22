package agent

import (
	"errors"
	"strings"
	"testing"
)

type emptyError struct{}

func (emptyError) Error() string { return "" }

func TestDescribeModelErrorPreservesNonEmptyError(t *testing.T) {
	original := errors.New("connection refused: 26.88.87.115:1234")
	got := describeModelError("test-agent", "json_mode", original)
	if !errors.Is(got, original) {
		t.Fatalf("expected original error to be preserved, got %v", got)
	}
	if !strings.Contains(got.Error(), "connection refused") {
		t.Fatalf("expected error message to contain original text, got %q", got.Error())
	}
}

func TestDescribeModelErrorReplacesEmptyError(t *testing.T) {
	got := describeModelError("test-agent", "json_mode", emptyError{})
	msg := got.Error()
	if strings.TrimSpace(msg) == "" {
		t.Fatalf("expected non-empty error message for empty error, got %q", msg)
	}
	if !strings.Contains(msg, "response_format") {
		t.Fatalf("expected error message to mention response_format, got %q", msg)
	}
}

func TestDescribeModelErrorHandlesNilStringError(t *testing.T) {
	// Simulate an error whose Error() returns only whitespace
	whitespaceErr := errors.New("   ")
	got := describeModelError("test-agent", "json_mode", whitespaceErr)
	msg := got.Error()
	if strings.TrimSpace(msg) == "" {
		t.Fatalf("expected non-empty error message for whitespace-only error, got %q", msg)
	}
}
