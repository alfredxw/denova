package runtime_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	runstate "denova/internal/agent/runtime"
)

func TestCommandEnvelopeIsBoundedBeforeAdmission(t *testing.T) {
	t.Parallel()

	release := make(chan struct{})
	engine := runstate.NewScriptedEngine(runstate.EngineScript{
		Continue: release,
		Result:   runstate.EngineResult{Status: runstate.EngineCompleted},
	})
	runtime, err := runstate.NewRuntime(engine, runstate.NewMemoryJournalStore(), runstate.RuntimeConfig{
		InputLimits: runstate.InputLimits{
			MaxCommandIDBytes:   8,
			MaxOperationIDBytes: 8,
			MaxAbortReasonBytes: 16,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	harness, err := runtime.Open(context.Background(), runstate.WritingBinding{
		Workspace: "/book", SessionID: "bounded-command-envelope",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := harness.Submit(context.Background(), runstate.StartTurn{
		ID: "command-id-too-long", Input: runstate.UserInput{Text: "write"},
	}); !errors.Is(err, runstate.ErrInvalidCommand) {
		t.Fatalf("oversized command id error = %v, want ErrInvalidCommand", err)
	}
	if got := len(engine.Requests()); got != 0 {
		t.Fatalf("engine calls after oversized command id = %d, want 0", got)
	}

	started, err := harness.Submit(context.Background(), runstate.StartTurn{
		ID: "start", Input: runstate.UserInput{Text: "write"},
	})
	if err != nil {
		t.Fatal(err)
	}
	for name, command := range map[string]runstate.Command{
		"steer target": runstate.Steer{
			ID: "steer", OperationID: "operation-id-too-long", Input: runstate.UserInput{Text: "redirect"},
		},
		"follow-up target": runstate.FollowUp{
			ID: "follow", OperationID: "operation-id-too-long", Input: runstate.UserInput{Text: "more"},
		},
		"next-turn target": runstate.NextTurn{
			ID: "next", AfterOperationID: "operation-id-too-long", Input: runstate.UserInput{Text: "next"},
		},
		"abort target": runstate.Abort{
			ID: "abort", OperationID: "operation-id-too-long", Reason: "stop",
		},
	} {
		if _, err := harness.Submit(context.Background(), command); !errors.Is(err, runstate.ErrInvalidCommand) {
			t.Fatalf("%s error = %v, want ErrInvalidCommand", name, err)
		}
	}
	if _, err := harness.Submit(context.Background(), runstate.Abort{
		ID: "abort", OperationID: started.OperationID, Reason: strings.Repeat("r", 17),
	}); !errors.Is(err, runstate.ErrInvalidCommand) {
		t.Fatalf("oversized abort reason error = %v, want ErrInvalidCommand", err)
	}
	close(release)
	waitForSettled(t, harness, started.Cursor)
	if err := runtime.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestValidateCommandIDExposesTheHarnessAdmissionBoundary(t *testing.T) {
	t.Parallel()

	defaults := runstate.DefaultInputLimits()
	if defaults.MaxCommandIDBytes != 4<<10 {
		t.Fatalf("default command id bound = %d, want 4 KiB", defaults.MaxCommandIDBytes)
	}
	if err := runstate.ValidateCommandID("external-command", runstate.InputLimits{}); err != nil {
		t.Fatalf("default command id validation: %v", err)
	}
	if err := runstate.ValidateCommandID("12345", runstate.InputLimits{MaxCommandIDBytes: 4}); !errors.Is(err, runstate.ErrInvalidCommand) {
		t.Fatalf("custom command id bound error = %v, want ErrInvalidCommand", err)
	}
}

func TestRuntimeValidateCommandIDUsesItsNormalizedConfiguration(t *testing.T) {
	t.Parallel()

	runtime, err := runstate.NewRuntime(
		runstate.NewScriptedEngine(),
		runstate.NewMemoryJournalStore(),
		runstate.RuntimeConfig{InputLimits: runstate.InputLimits{MaxCommandIDBytes: 4}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.ValidateCommandID("1234"); err != nil {
		t.Fatalf("four-byte command id: %v", err)
	}
	if err := runtime.ValidateCommandID("12345"); !errors.Is(err, runstate.ErrInvalidCommand) {
		t.Fatalf("runtime command id bound error = %v, want ErrInvalidCommand", err)
	}
}
