package runtime_test

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	runstate "github.com/alfredxw/denova/agent/runtime"
)

func TestRecoveryPreservesPendingInputUntilExplicitCancellation(t *testing.T) {
	t.Parallel()

	store := runstate.NewMemoryJournalStore()
	binding := testBindingAt("/book", "release-recovered-input")
	bindingRef := binding
	encodedBinding, err := json.Marshal(bindingRef)
	if err != nil {
		t.Fatal(err)
	}
	journal, err := store.OpenJournal(context.Background(), string(encodedBinding))
	if err != nil {
		t.Fatal(err)
	}
	operationID := runstate.OperationID("unfinished-operation")
	if _, err := journal.Append(context.Background(), 0, []runstate.EventPayload{
		runstate.CommandAcceptedEvent{CommandID: "start", CommandKind: "start_turn", OperationID: operationID, Fingerprint: "start"},
		runstate.OperationStartedEvent{OperationID: operationID},
		runstate.UserMessageCommittedEvent{Message: runstate.Message{
			ID: "message", Role: runstate.RoleUser, Content: "write",
			Input: runstate.UserInput{Text: "write", TurnSpecRef: "active-ref"}, Operation: operationID,
		}},
		runstate.CycleStartedEvent{OperationID: operationID, Cycle: 1, SnapshotID: "snapshot"},
		runstate.CommandAcceptedEvent{CommandID: "queued", CommandKind: "follow_up", OperationID: operationID, Fingerprint: "queued"},
		runstate.QueueEnqueuedEvent{Item: runstate.QueuedInput{
			CommandID: "queued", OperationID: operationID, Delivery: runstate.DeliveryFollowUp,
			Input: runstate.UserInput{Text: "more", TurnSpecRef: "queued-ref"},
		}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := journal.Close(); err != nil {
		t.Fatal(err)
	}

	engine := newPendingInputReleaseEngine()
	runtime, err := runstate.NewRuntime(engine, store, runstate.RuntimeConfig{})
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
	if _, err := harness.Submit(context.Background(), runstate.Abort{
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
	runtime, err := runstate.NewRuntime(engine, runstate.NewMemoryJournalStore(), runstate.RuntimeConfig{})
	if err != nil {
		t.Fatal(err)
	}
	harness, err := runtime.Open(context.Background(), testBinding("release-queued-input"))
	if err != nil {
		t.Fatal(err)
	}
	started, err := harness.Submit(context.Background(), runstate.StartTurn{
		ID: "start", Input: runstate.UserInput{Text: "write", TurnSpecRef: "active-ref"},
	})
	if err != nil {
		t.Fatal(err)
	}
	queued, err := harness.Submit(context.Background(), runstate.FollowUp{
		ID: "queued", OperationID: started.OperationID,
		Input: runstate.UserInput{Text: "more", TurnSpecRef: "queued-ref"},
	})
	if err != nil {
		t.Fatal(err)
	}
	observation, err := harness.Observe(context.Background(), queued.Cursor)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := harness.Submit(context.Background(), runstate.Abort{
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
			cancelled, ok := event.Payload.(runstate.QueueCancelledEvent)
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
	released      chan runstate.UserInput
	releaseCalled chan struct{}
	once          sync.Once
}

func newPendingInputReleaseEngine() *pendingInputReleaseEngine {
	return &pendingInputReleaseEngine{
		continueRun:   make(chan struct{}),
		released:      make(chan runstate.UserInput, 1),
		releaseCalled: make(chan struct{}),
	}
}

func (e *pendingInputReleaseEngine) NewEngine(context.Context, runstate.BindingRef) (runstate.Engine, error) {
	return e, nil
}

func (e *pendingInputReleaseEngine) Run(context.Context, runstate.EngineRequest, runstate.EngineEventSink) (runstate.EngineResult, error) {
	<-e.continueRun
	return runstate.EngineResult{Status: runstate.EngineCompleted}, nil
}

func (e *pendingInputReleaseEngine) ReleasePendingInput(_ context.Context, input runstate.UserInput) {
	e.released <- input
	e.once.Do(func() { close(e.releaseCalled) })
}
