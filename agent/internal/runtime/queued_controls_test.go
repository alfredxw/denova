package runtime_test

import (
	"context"
	"testing"

	runstate "github.com/alfredxw/denova/agent/internal/runtime"
)

func TestSteerQueuedPreemptsIntoTheAcceptedFollowUp(t *testing.T) {
	t.Parallel()

	engine := runstate.NewScriptedEngine(
		runstate.EngineScript{WaitForControl: runstate.EngineControlPreempt},
		runstate.EngineScript{
			Events: []runstate.EngineEvent{runstate.EngineAssistantFinal{Content: "steered answer"}},
			Result: runstate.EngineResult{Status: runstate.EngineCompleted},
		},
	)
	runtime, err := runstate.NewRuntime(engine, runstate.NewMemoryJournalStore(), runstate.RuntimeConfig{})
	if err != nil {
		t.Fatal(err)
	}
	harness, err := runtime.Open(context.Background(), testBinding("steer-queued"))
	if err != nil {
		t.Fatal(err)
	}
	started, err := harness.Submit(context.Background(), runstate.StartTurn{
		ID: "start", Input: runstate.UserInput{Text: "initial"},
	})
	if err != nil {
		t.Fatal(err)
	}
	queued, err := harness.Submit(context.Background(), runstate.FollowUp{
		ID: "queued", OperationID: started.OperationID, Input: runstate.UserInput{Text: "new direction"},
	})
	if err != nil {
		t.Fatal(err)
	}
	steered, err := harness.Submit(context.Background(), runstate.SteerQueued{
		ID: "steer-queued", OperationID: started.OperationID, TargetCommandID: queued.CommandID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if steered.OperationID != started.OperationID {
		t.Fatalf("steer queued operation = %q, want %q", steered.OperationID, started.OperationID)
	}

	waitForSettled(t, harness, steered.Cursor)
	observation, err := harness.Observe(context.Background(), 0)
	if err != nil {
		t.Fatal(err)
	}
	if got := messageTexts(observation.Snapshot.Messages); len(got) != 3 || got[0] != "initial" || got[1] != "new direction" || got[2] != "steered answer" {
		t.Fatalf("messages after steering queued input = %#v", got)
	}
	requests := engine.Requests()
	if len(requests) != 2 || requests[1].Snapshot.CommandID != queued.CommandID || requests[1].Snapshot.Input.Text != "new direction" {
		t.Fatalf("engine requests = %#v", requests)
	}
}

func TestCancelQueuedRemovesOnlyTheTargetInput(t *testing.T) {
	t.Parallel()

	engine := runstate.NewScriptedEngine(runstate.EngineScript{WaitForControl: runstate.EngineControlAbort})
	runtime, err := runstate.NewRuntime(engine, runstate.NewMemoryJournalStore(), runstate.RuntimeConfig{})
	if err != nil {
		t.Fatal(err)
	}
	harness, err := runtime.Open(context.Background(), testBinding("cancel-queued"))
	if err != nil {
		t.Fatal(err)
	}
	started, err := harness.Submit(context.Background(), runstate.StartTurn{
		ID: "start", Input: runstate.UserInput{Text: "initial"},
	})
	if err != nil {
		t.Fatal(err)
	}
	queued, err := harness.Submit(context.Background(), runstate.FollowUp{
		ID: "queued", OperationID: started.OperationID, Input: runstate.UserInput{Text: "discard me"},
	})
	if err != nil {
		t.Fatal(err)
	}
	cancelled, err := harness.Submit(context.Background(), runstate.CancelQueued{
		ID: "cancel-queued", OperationID: started.OperationID, TargetCommandID: queued.CommandID,
		Reason: "user_deleted",
	})
	if err != nil {
		t.Fatal(err)
	}
	status, err := harness.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(status.Queue) != 0 {
		t.Fatalf("queue after cancellation = %#v", status.Queue)
	}
	replayed, err := harness.Submit(context.Background(), runstate.CancelQueued{
		ID: "cancel-queued", OperationID: started.OperationID, TargetCommandID: queued.CommandID,
		Reason: "user_deleted",
	})
	if err != nil {
		t.Fatalf("replay cancel queued: %v", err)
	}
	if !replayed.Replayed || replayed.Cursor != cancelled.Cursor {
		t.Fatalf("replayed cancellation = %#v, first = %#v", replayed, cancelled)
	}

	aborted, err := harness.Submit(context.Background(), runstate.Abort{
		ID: "abort", OperationID: started.OperationID, Reason: "test_cleanup",
	})
	if err != nil {
		t.Fatal(err)
	}
	waitForSettled(t, harness, aborted.Cursor)
}
