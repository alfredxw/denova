package agentruntime_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"denova/internal/agentruntime"
)

func TestCommandEnvelopeIsBoundedBeforeAdmission(t *testing.T) {
	t.Parallel()

	release := make(chan struct{})
	engine := agentruntime.NewScriptedEngine(agentruntime.EngineScript{
		Continue: release,
		Result:   agentruntime.EngineResult{Status: agentruntime.EngineCompleted},
	})
	runtime, err := agentruntime.NewRuntime(engine, agentruntime.NewMemoryJournalStore(), agentruntime.RuntimeConfig{
		InputLimits: agentruntime.InputLimits{
			MaxCommandIDBytes:   8,
			MaxOperationIDBytes: 8,
			MaxAbortReasonBytes: 16,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	harness, err := runtime.Open(context.Background(), agentruntime.WritingBinding{
		Workspace: "/book", SessionID: "bounded-command-envelope",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := harness.Submit(context.Background(), agentruntime.StartTurn{
		ID: "command-id-too-long", Input: agentruntime.UserInput{Text: "write"},
	}); !errors.Is(err, agentruntime.ErrInvalidCommand) {
		t.Fatalf("oversized command id error = %v, want ErrInvalidCommand", err)
	}
	if got := len(engine.Requests()); got != 0 {
		t.Fatalf("engine calls after oversized command id = %d, want 0", got)
	}

	started, err := harness.Submit(context.Background(), agentruntime.StartTurn{
		ID: "start", Input: agentruntime.UserInput{Text: "write"},
	})
	if err != nil {
		t.Fatal(err)
	}
	for name, command := range map[string]agentruntime.Command{
		"steer target": agentruntime.Steer{
			ID: "steer", OperationID: "operation-id-too-long", Input: agentruntime.UserInput{Text: "redirect"},
		},
		"follow-up target": agentruntime.FollowUp{
			ID: "follow", OperationID: "operation-id-too-long", Input: agentruntime.UserInput{Text: "more"},
		},
		"next-turn target": agentruntime.NextTurn{
			ID: "next", AfterOperationID: "operation-id-too-long", Input: agentruntime.UserInput{Text: "next"},
		},
		"abort target": agentruntime.Abort{
			ID: "abort", OperationID: "operation-id-too-long", Reason: "stop",
		},
	} {
		if _, err := harness.Submit(context.Background(), command); !errors.Is(err, agentruntime.ErrInvalidCommand) {
			t.Fatalf("%s error = %v, want ErrInvalidCommand", name, err)
		}
	}
	if _, err := harness.Submit(context.Background(), agentruntime.Abort{
		ID: "abort", OperationID: started.OperationID, Reason: strings.Repeat("r", 17),
	}); !errors.Is(err, agentruntime.ErrInvalidCommand) {
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

	defaults := agentruntime.DefaultInputLimits()
	if defaults.MaxCommandIDBytes != 4<<10 {
		t.Fatalf("default command id bound = %d, want 4 KiB", defaults.MaxCommandIDBytes)
	}
	if err := agentruntime.ValidateCommandID("external-command", agentruntime.InputLimits{}); err != nil {
		t.Fatalf("default command id validation: %v", err)
	}
	if err := agentruntime.ValidateCommandID("12345", agentruntime.InputLimits{MaxCommandIDBytes: 4}); !errors.Is(err, agentruntime.ErrInvalidCommand) {
		t.Fatalf("custom command id bound error = %v, want ErrInvalidCommand", err)
	}
}

func TestRuntimeValidateCommandIDUsesItsNormalizedConfiguration(t *testing.T) {
	t.Parallel()

	runtime, err := agentruntime.NewRuntime(
		agentruntime.NewScriptedEngine(),
		agentruntime.NewMemoryJournalStore(),
		agentruntime.RuntimeConfig{InputLimits: agentruntime.InputLimits{MaxCommandIDBytes: 4}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.ValidateCommandID("1234"); err != nil {
		t.Fatalf("four-byte command id: %v", err)
	}
	if err := runtime.ValidateCommandID("12345"); !errors.Is(err, agentruntime.ErrInvalidCommand) {
		t.Fatalf("runtime command id bound error = %v, want ErrInvalidCommand", err)
	}
}
