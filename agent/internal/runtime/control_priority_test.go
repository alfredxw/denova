package runtime_test

import (
	"context"
	"errors"
	"testing"
	"time"

	runstate "github.com/alfredxw/denova/agent/internal/runtime"
)

func TestAcceptedAbortRejectsLateAssistantFinal(t *testing.T) {
	t.Parallel()

	engine := &lateFinalEngine{release: make(chan struct{})}
	runtime, err := runstate.NewRuntime(engine, runstate.NewMemoryJournalStore(), runstate.RuntimeConfig{})
	if err != nil {
		t.Fatal(err)
	}
	harness, err := runtime.Open(context.Background(), testBinding("abort-late-final"))
	if err != nil {
		t.Fatal(err)
	}
	started, err := harness.Submit(context.Background(), runstate.StartTurn{
		ID: "start", Input: runstate.UserInput{Text: "write"},
	})
	if err != nil {
		t.Fatal(err)
	}
	aborted, err := harness.Submit(context.Background(), runstate.Abort{
		ID: "abort", OperationID: started.OperationID,
	})
	if err != nil {
		t.Fatal(err)
	}
	close(engine.release)
	settled := waitForEventType[runstate.OperationSettledEvent](t, harness, aborted.Cursor)
	if settled.Status != runstate.OperationAborted {
		t.Fatalf("settled status = %q, want aborted", settled.Status)
	}
	observation, err := harness.ObserveFromNow(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got := messageTexts(observation.Snapshot.Messages); len(got) != 1 || got[0] != "write" {
		t.Fatalf("messages after accepted abort = %#v, want only the user message", got)
	}
}

func TestAcceptedAbortDominatesLateEngineCompletion(t *testing.T) {
	t.Parallel()

	release := make(chan struct{})
	engine := runstate.NewScriptedEngine(
		runstate.EngineScript{Continue: release, Result: runstate.EngineResult{Status: runstate.EngineCompleted}},
		runstate.EngineScript{Result: runstate.EngineResult{Status: runstate.EngineCompleted}},
	)
	runtime, err := runstate.NewRuntime(engine, runstate.NewMemoryJournalStore(), runstate.RuntimeConfig{})
	if err != nil {
		t.Fatal(err)
	}
	harness, err := runtime.Open(context.Background(), testBindingAt("/book", "abort-dominates"))
	if err != nil {
		t.Fatal(err)
	}
	started, err := harness.Submit(context.Background(), runstate.StartTurn{ID: "start", Input: runstate.UserInput{Text: "write"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := harness.Submit(context.Background(), runstate.FollowUp{
		ID: "queued", OperationID: started.OperationID, Input: runstate.UserInput{Text: "more"},
	}); err != nil {
		t.Fatal(err)
	}
	aborted, err := harness.Submit(context.Background(), runstate.Abort{ID: "abort", OperationID: started.OperationID})
	if err != nil {
		t.Fatal(err)
	}
	close(release)

	settled := waitForEventType[runstate.OperationSettledEvent](t, harness, aborted.Cursor)
	if settled.Status != runstate.OperationAborted {
		t.Fatalf("settled status = %q, want aborted", settled.Status)
	}
	if got := len(engine.Requests()); got != 1 {
		t.Fatalf("engine runs = %d, want one; an accepted abort must cancel queued cycles", got)
	}
}

func TestCloseBindingDominatesLateEngineCompletion(t *testing.T) {
	t.Parallel()

	release := make(chan struct{})
	engine := runstate.NewScriptedEngine(
		runstate.EngineScript{Continue: release, Result: runstate.EngineResult{Status: runstate.EngineCompleted}},
		runstate.EngineScript{Result: runstate.EngineResult{Status: runstate.EngineCompleted}},
	)
	store := runstate.NewMemoryJournalStore()
	runtime, err := runstate.NewRuntime(engine, store, runstate.RuntimeConfig{})
	if err != nil {
		t.Fatal(err)
	}
	binding := testBindingAt("/book", "close-dominates")
	harness, err := runtime.Open(context.Background(), binding)
	if err != nil {
		t.Fatal(err)
	}
	started, err := harness.Submit(context.Background(), runstate.StartTurn{ID: "start", Input: runstate.UserInput{Text: "write"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := harness.Submit(context.Background(), runstate.FollowUp{
		ID: "queued", OperationID: started.OperationID, Input: runstate.UserInput{Text: "more"},
	}); err != nil {
		t.Fatal(err)
	}
	observation, err := harness.Observe(context.Background(), started.Cursor)
	if err != nil {
		t.Fatal(err)
	}
	closed := make(chan error, 1)
	runExternalErrorTestGoroutine(closed, "close binding with queued control", func() error {
		return runtime.CloseBinding(context.Background(), binding)
	})
	waitForAbortRequested(t, observation)
	close(release)
	if err := <-closed; err != nil {
		t.Fatalf("close binding: %v", err)
	}
	if got := len(engine.Requests()); got != 1 {
		t.Fatalf("engine runs = %d, want one; closing must not start queued work", got)
	}

	reopened, err := runtime.Open(context.Background(), binding)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	replayed, err := reopened.Observe(context.Background(), 0)
	if err != nil {
		t.Fatal(err)
	}
	var status runstate.OperationStatus
	for _, event := range rangeDurableEvents(t, replayed, replayed.Snapshot.Cursor) {
		if settled, ok := event.Payload.(runstate.OperationSettledEvent); ok && settled.OperationID == started.OperationID {
			status = settled.Status
		}
	}
	if status != runstate.OperationAborted {
		t.Fatalf("persisted close status = %q, want aborted", status)
	}
}

func TestNextTurnRejectsSecondPendingSuccessor(t *testing.T) {
	t.Parallel()

	release := make(chan struct{})
	runtime, err := runstate.NewRuntime(
		runstate.NewScriptedEngine(runstate.EngineScript{Continue: release}),
		runstate.NewMemoryJournalStore(),
		runstate.RuntimeConfig{},
	)
	if err != nil {
		t.Fatal(err)
	}
	harness, err := runtime.Open(context.Background(), testGameBinding("/game", "story", "branch"))
	if err != nil {
		t.Fatal(err)
	}
	started, err := harness.Submit(context.Background(), runstate.StartTurn{ID: "start", Input: runstate.UserInput{Text: "look"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := harness.Submit(context.Background(), runstate.NextTurn{
		ID: "first-next", AfterOperationID: started.OperationID, Input: runstate.UserInput{Text: "open"},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := harness.Submit(context.Background(), runstate.NextTurn{
		ID: "second-next", AfterOperationID: started.OperationID, Input: runstate.UserInput{Text: "leave"},
	}); !errors.Is(err, runstate.ErrQueueConflict) {
		t.Fatalf("second next-turn error = %v, want ErrQueueConflict", err)
	}
	close(release)
}

func waitForAbortRequested(t *testing.T, observation runstate.Observation) {
	t.Helper()
	timer := time.NewTimer(500 * time.Millisecond)
	defer timer.Stop()
	for {
		select {
		case event, ok := <-observation.Events:
			if !ok {
				t.Fatal("observation closed before abort request")
			}
			if _, ok := event.Payload.(runstate.AbortRequestedEvent); ok {
				return
			}
		case err := <-observation.Errors:
			if err != nil {
				t.Fatalf("observation error: %v", err)
			}
		case <-timer.C:
			t.Fatal("timed out waiting for abort request")
		}
	}
}

type lateFinalEngine struct {
	release chan struct{}
}

func (e *lateFinalEngine) NewEngine(context.Context, runstate.BindingRef) (runstate.Engine, error) {
	return e, nil
}

func (e *lateFinalEngine) Run(_ context.Context, _ runstate.EngineRequest, emit runstate.EngineEventSink) (runstate.EngineResult, error) {
	<-e.release
	if err := emit(runstate.EngineAssistantFinal{Content: "must not commit"}); err != nil {
		return runstate.EngineResult{}, err
	}
	return runstate.EngineResult{Status: runstate.EngineCompleted}, nil
}
