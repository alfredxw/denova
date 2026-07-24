package runtime_test

import (
	"context"
	"errors"
	"testing"

	runstate "github.com/alfredxw/denova/agent/runtime"
)

func TestHarnessRejectsCommandWithoutIdempotencyKey(t *testing.T) {
	runtime, err := runstate.NewRuntime(
		runstate.NewScriptedEngine(),
		runstate.NewMemoryJournalStore(),
		runstate.RuntimeConfig{},
	)
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })

	harness, err := runtime.Open(context.Background(), testBinding("missing-command-id"))
	if err != nil {
		t.Fatalf("open harness: %v", err)
	}
	if _, err := harness.Submit(context.Background(), runstate.StartTurn{
		Input: runstate.UserInput{Text: "write"},
	}); !errors.Is(err, runstate.ErrInvalidCommand) {
		t.Fatalf("submit error = %v, want ErrInvalidCommand", err)
	}
}

func TestHarnessNormalizesNilPublicContexts(t *testing.T) {
	runtime, err := runstate.NewRuntime(
		runstate.NewScriptedEngine(),
		runstate.NewMemoryJournalStore(),
		runstate.RuntimeConfig{},
	)
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })

	harness, err := runtime.Open(context.Background(), testBinding("nil-context"))
	if err != nil {
		t.Fatalf("open harness: %v", err)
	}
	if _, err := harness.ObserveFromNow(nil); err != nil {
		t.Fatalf("observe with nil context: %v", err)
	}
	if _, err := harness.Submit(nil, runstate.StartTurn{
		ID: "nil-context-command", Input: runstate.UserInput{Text: "write"},
	}); err != nil {
		t.Fatalf("submit with nil context: %v", err)
	}
}
