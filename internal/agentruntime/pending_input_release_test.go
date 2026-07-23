package agentruntime_test

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"denova/internal/agentruntime"
)

func TestRecoveryPreservesPendingInputUntilExplicitCancellation(t *testing.T) {
	t.Parallel()

	store := agentruntime.NewMemoryJournalStore()
	binding := agentruntime.WritingBinding{Workspace: "/book", SessionID: "release-recovered-input"}
	bindingRef := agentruntime.BindingRef{
		Kind: agentruntime.BindingWriting, Profile: agentruntime.ProfileWriting,
		Workspace: "/book", SessionID: "release-recovered-input",
	}
	encodedBinding, err := json.Marshal(bindingRef)
	if err != nil {
		t.Fatal(err)
	}
	journal, err := store.OpenJournal(context.Background(), string(encodedBinding))
	if err != nil {
		t.Fatal(err)
	}
	operationID := agentruntime.OperationID("unfinished-operation")
	if _, err := journal.Append(context.Background(), 0, []agentruntime.EventPayload{
		agentruntime.CommandAcceptedEvent{CommandID: "start", CommandKind: "start_turn", OperationID: operationID, Fingerprint: "start"},
		agentruntime.OperationStartedEvent{OperationID: operationID},
		agentruntime.UserMessageCommittedEvent{Message: agentruntime.Message{
			ID: "message", Role: agentruntime.RoleUser, Content: "write",
			Input: agentruntime.UserInput{Text: "write", TurnSpecRef: "active-ref"}, Operation: operationID,
		}},
		agentruntime.CycleStartedEvent{OperationID: operationID, Cycle: 1, SnapshotID: "snapshot"},
		agentruntime.CommandAcceptedEvent{CommandID: "queued", CommandKind: "follow_up", OperationID: operationID, Fingerprint: "queued"},
		agentruntime.QueueEnqueuedEvent{Item: agentruntime.QueuedInput{
			CommandID: "queued", OperationID: operationID, Delivery: agentruntime.DeliveryFollowUp,
			Input: agentruntime.UserInput{Text: "more", TurnSpecRef: "queued-ref"},
		}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := journal.Close(); err != nil {
		t.Fatal(err)
	}

	engine := newPendingInputReleaseEngine()
	runtime, err := agentruntime.NewRuntime(engine, store, agentruntime.RuntimeConfig{})
	if err != nil {
		t.Fatal(err)
	}
	harness, err := runtime.Open(context.Background(), binding)
	if err != nil {
		t.Fatalf("recover: %v", err)
	}
	select {
	case input := <-engine.released:
		t.Fatalf("recovery released accepted input before exact replay or cancellation: %+v", input)
	default:
	}
	if _, err := harness.Submit(context.Background(), agentruntime.Abort{
		ID: "cancel-recovered-queue", OperationID: operationID, Reason: "caller cancelled recovered input",
	}); err != nil {
		t.Fatal(err)
	}
	select {
	case input := <-engine.released:
		if input.TurnSpecRef != "queued-ref" {
			t.Fatalf("released explicitly cancelled input = %+v", input)
		}
	default:
		t.Fatal("explicit cancellation did not release its durable queued input")
	}
}

func TestQueueCancellationReleasesPendingInputBeforePublish(t *testing.T) {
	t.Parallel()

	engine := newPendingInputReleaseEngine()
	runtime, err := agentruntime.NewRuntime(engine, agentruntime.NewMemoryJournalStore(), agentruntime.RuntimeConfig{})
	if err != nil {
		t.Fatal(err)
	}
	harness, err := runtime.Open(context.Background(), agentruntime.WritingBinding{
		Workspace: "/book", SessionID: "release-queued-input",
	})
	if err != nil {
		t.Fatal(err)
	}
	started, err := harness.Submit(context.Background(), agentruntime.StartTurn{
		ID: "start", Input: agentruntime.UserInput{Text: "write", TurnSpecRef: "active-ref"},
	})
	if err != nil {
		t.Fatal(err)
	}
	queued, err := harness.Submit(context.Background(), agentruntime.FollowUp{
		ID: "queued", OperationID: started.OperationID,
		Input: agentruntime.UserInput{Text: "more", TurnSpecRef: "queued-ref"},
	})
	if err != nil {
		t.Fatal(err)
	}
	observation, err := harness.Observe(context.Background(), queued.Cursor)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := harness.Submit(context.Background(), agentruntime.Abort{
		ID: "abort", OperationID: started.OperationID,
	}); err != nil {
		t.Fatal(err)
	}
	close(engine.continueRun)

	select {
	case input := <-engine.released:
		if input.TurnSpecRef != "queued-ref" || input.Text != "more" {
			t.Fatalf("released input = %+v", input)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("queued input was not released")
	}

	for {
		select {
		case event, ok := <-observation.Events:
			if !ok {
				t.Fatal("observation closed before queue cancellation")
			}
			cancelled, ok := event.Payload.(agentruntime.QueueCancelledEvent)
			if !ok || cancelled.CommandID != "queued" {
				continue
			}
			select {
			case <-engine.releaseCalled:
				return
			default:
				t.Fatal("QueueCancelled was published before its pending input was released")
			}
		case err := <-observation.Errors:
			if err != nil {
				t.Fatalf("observation error: %v", err)
			}
		case <-time.After(500 * time.Millisecond):
			t.Fatal("timed out waiting for QueueCancelled")
		}
	}
}

type pendingInputReleaseEngine struct {
	continueRun   chan struct{}
	released      chan agentruntime.UserInput
	releaseCalled chan struct{}
	once          sync.Once
}

func newPendingInputReleaseEngine() *pendingInputReleaseEngine {
	return &pendingInputReleaseEngine{
		continueRun:   make(chan struct{}),
		released:      make(chan agentruntime.UserInput, 1),
		releaseCalled: make(chan struct{}),
	}
}

func (e *pendingInputReleaseEngine) NewEngine(context.Context, agentruntime.BindingRef) (agentruntime.Engine, error) {
	return e, nil
}

func (e *pendingInputReleaseEngine) Run(context.Context, agentruntime.EngineRequest, agentruntime.EngineEventSink) (agentruntime.EngineResult, error) {
	<-e.continueRun
	return agentruntime.EngineResult{Status: agentruntime.EngineCompleted}, nil
}

func (e *pendingInputReleaseEngine) ReleasePendingInput(_ context.Context, input agentruntime.UserInput) {
	e.released <- input
	e.once.Do(func() { close(e.releaseCalled) })
}
