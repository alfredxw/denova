package agentruntime_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"denova/internal/agentruntime"
)

func TestAcceptedAbortRejectsLateAssistantFinal(t *testing.T) {
	t.Parallel()

	engine := &lateFinalEngine{release: make(chan struct{})}
	runtime, err := agentruntime.NewRuntime(engine, agentruntime.NewMemoryJournalStore(), agentruntime.RuntimeConfig{})
	if err != nil {
		t.Fatal(err)
	}
	harness, err := runtime.Open(context.Background(), agentruntime.WritingBinding{
		Workspace: "/book", SessionID: "abort-late-final",
	})
	if err != nil {
		t.Fatal(err)
	}
	started, err := harness.Submit(context.Background(), agentruntime.StartTurn{
		ID: "start", Input: agentruntime.UserInput{Text: "write"},
	})
	if err != nil {
		t.Fatal(err)
	}
	aborted, err := harness.Submit(context.Background(), agentruntime.Abort{
		ID: "abort", OperationID: started.OperationID,
	})
	if err != nil {
		t.Fatal(err)
	}
	close(engine.release)
	settled := waitForEventType[agentruntime.OperationSettledEvent](t, harness, aborted.Cursor)
	if settled.Status != agentruntime.OperationAborted {
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
	engine := agentruntime.NewScriptedEngine(
		agentruntime.EngineScript{Continue: release, Result: agentruntime.EngineResult{Status: agentruntime.EngineCompleted}},
		agentruntime.EngineScript{Result: agentruntime.EngineResult{Status: agentruntime.EngineCompleted}},
	)
	runtime, err := agentruntime.NewRuntime(engine, agentruntime.NewMemoryJournalStore(), agentruntime.RuntimeConfig{})
	if err != nil {
		t.Fatal(err)
	}
	harness, err := runtime.Open(context.Background(), agentruntime.WritingBinding{Workspace: "/book", SessionID: "abort-dominates"})
	if err != nil {
		t.Fatal(err)
	}
	started, err := harness.Submit(context.Background(), agentruntime.StartTurn{ID: "start", Input: agentruntime.UserInput{Text: "write"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := harness.Submit(context.Background(), agentruntime.FollowUp{
		ID: "queued", OperationID: started.OperationID, Input: agentruntime.UserInput{Text: "more"},
	}); err != nil {
		t.Fatal(err)
	}
	aborted, err := harness.Submit(context.Background(), agentruntime.Abort{ID: "abort", OperationID: started.OperationID})
	if err != nil {
		t.Fatal(err)
	}
	close(release)

	settled := waitForEventType[agentruntime.OperationSettledEvent](t, harness, aborted.Cursor)
	if settled.Status != agentruntime.OperationAborted {
		t.Fatalf("settled status = %q, want aborted", settled.Status)
	}
	if got := len(engine.Requests()); got != 1 {
		t.Fatalf("engine runs = %d, want one; an accepted abort must cancel queued cycles", got)
	}
}

func TestCloseBindingDominatesLateEngineCompletion(t *testing.T) {
	t.Parallel()

	release := make(chan struct{})
	engine := agentruntime.NewScriptedEngine(
		agentruntime.EngineScript{Continue: release, Result: agentruntime.EngineResult{Status: agentruntime.EngineCompleted}},
		agentruntime.EngineScript{Result: agentruntime.EngineResult{Status: agentruntime.EngineCompleted}},
	)
	store := agentruntime.NewMemoryJournalStore()
	runtime, err := agentruntime.NewRuntime(engine, store, agentruntime.RuntimeConfig{})
	if err != nil {
		t.Fatal(err)
	}
	binding := agentruntime.WritingBinding{Workspace: "/book", SessionID: "close-dominates"}
	harness, err := runtime.Open(context.Background(), binding)
	if err != nil {
		t.Fatal(err)
	}
	started, err := harness.Submit(context.Background(), agentruntime.StartTurn{ID: "start", Input: agentruntime.UserInput{Text: "write"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := harness.Submit(context.Background(), agentruntime.FollowUp{
		ID: "queued", OperationID: started.OperationID, Input: agentruntime.UserInput{Text: "more"},
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
	var status agentruntime.OperationStatus
	for _, event := range rangeDurableEvents(t, replayed, replayed.Snapshot.Cursor) {
		if settled, ok := event.Payload.(agentruntime.OperationSettledEvent); ok && settled.OperationID == started.OperationID {
			status = settled.Status
		}
	}
	if status != agentruntime.OperationAborted {
		t.Fatalf("persisted close status = %q, want aborted", status)
	}
}

func TestNextTurnRejectsSecondPendingSuccessor(t *testing.T) {
	t.Parallel()

	release := make(chan struct{})
	runtime, err := agentruntime.NewRuntime(
		agentruntime.NewScriptedEngine(agentruntime.EngineScript{Continue: release}),
		agentruntime.NewMemoryJournalStore(),
		agentruntime.RuntimeConfig{},
	)
	if err != nil {
		t.Fatal(err)
	}
	harness, err := runtime.Open(context.Background(), agentruntime.GameBinding{
		Workspace: "/game", StoryID: "story", BranchID: "branch",
	})
	if err != nil {
		t.Fatal(err)
	}
	started, err := harness.Submit(context.Background(), agentruntime.StartTurn{ID: "start", Input: agentruntime.UserInput{Text: "look"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := harness.Submit(context.Background(), agentruntime.NextTurn{
		ID: "first-next", AfterOperationID: started.OperationID, Input: agentruntime.UserInput{Text: "open"},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := harness.Submit(context.Background(), agentruntime.NextTurn{
		ID: "second-next", AfterOperationID: started.OperationID, Input: agentruntime.UserInput{Text: "leave"},
	}); !errors.Is(err, agentruntime.ErrQueueConflict) {
		t.Fatalf("second next-turn error = %v, want ErrQueueConflict", err)
	}
	close(release)
}

func waitForAbortRequested(t *testing.T, observation agentruntime.Observation) {
	t.Helper()
	timer := time.NewTimer(500 * time.Millisecond)
	defer timer.Stop()
	for {
		select {
		case event, ok := <-observation.Events:
			if !ok {
				t.Fatal("observation closed before abort request")
			}
			if _, ok := event.Payload.(agentruntime.AbortRequestedEvent); ok {
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

func (e *lateFinalEngine) NewEngine(context.Context, agentruntime.BindingRef) (agentruntime.Engine, error) {
	return e, nil
}

func (e *lateFinalEngine) Run(_ context.Context, _ agentruntime.EngineRequest, emit agentruntime.EngineEventSink) (agentruntime.EngineResult, error) {
	<-e.release
	if err := emit(agentruntime.EngineAssistantFinal{Content: "must not commit"}); err != nil {
		return agentruntime.EngineResult{}, err
	}
	return agentruntime.EngineResult{Status: agentruntime.EngineCompleted}, nil
}
