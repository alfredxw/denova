package agentruntime_test

import (
	"context"
	"errors"
	"testing"

	"denova/internal/agentruntime"
)

func TestHarnessRejectsCommandWithoutIdempotencyKey(t *testing.T) {
	runtime, err := agentruntime.NewRuntime(
		agentruntime.NewScriptedEngine(),
		agentruntime.NewMemoryJournalStore(),
		agentruntime.RuntimeConfig{},
	)
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })

	harness, err := runtime.Open(context.Background(), agentruntime.WritingBinding{
		Workspace: "/book", SessionID: "missing-command-id",
	})
	if err != nil {
		t.Fatalf("open harness: %v", err)
	}
	if _, err := harness.Submit(context.Background(), agentruntime.StartTurn{
		Input: agentruntime.UserInput{Text: "write"},
	}); !errors.Is(err, agentruntime.ErrInvalidCommand) {
		t.Fatalf("submit error = %v, want ErrInvalidCommand", err)
	}
}

func TestHarnessNormalizesNilPublicContexts(t *testing.T) {
	runtime, err := agentruntime.NewRuntime(
		agentruntime.NewScriptedEngine(),
		agentruntime.NewMemoryJournalStore(),
		agentruntime.RuntimeConfig{},
	)
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })

	harness, err := runtime.Open(context.Background(), agentruntime.WritingBinding{
		Workspace: "/book", SessionID: "nil-context",
	})
	if err != nil {
		t.Fatalf("open harness: %v", err)
	}
	if _, err := harness.ObserveFromNow(nil); err != nil {
		t.Fatalf("observe with nil context: %v", err)
	}
	if _, err := harness.Submit(nil, agentruntime.StartTurn{
		ID: "nil-context-command", Input: agentruntime.UserInput{Text: "write"},
	}); err != nil {
		t.Fatalf("submit with nil context: %v", err)
	}
}
