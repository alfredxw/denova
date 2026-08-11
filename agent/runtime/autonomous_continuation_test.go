package runtime

import (
	"context"
	"testing"
	"time"
)

type autonomousContinuationEngineFunc func(context.Context, EngineRequest, EngineEventSink) (EngineResult, error)

func (run autonomousContinuationEngineFunc) Run(ctx context.Context, request EngineRequest, emit EngineEventSink) (EngineResult, error) {
	return run(ctx, request, emit)
}

func TestEngineContinuationStaysInsideOneOperation(t *testing.T) {
	continuation := EngineContinuation{
		CommandID: "goal-continuation-1", Input: UserInput{Text: "continue the goal"}, Autonomous: true,
	}
	engine := NewScriptedEngine(
		EngineScript{
			Events: []EngineEvent{EngineAssistantFinal{Content: "progress", Continuation: &continuation}},
			Result: EngineResult{Status: EngineCompleted},
		},
		EngineScript{
			Events: []EngineEvent{EngineAssistantFinal{Content: "goal completed"}},
			Result: EngineResult{Status: EngineCompleted},
		},
	)
	runtime, err := NewRuntime(engine, NewMemoryJournalStore(), RuntimeConfig{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	harness, err := runtime.Open(context.Background(), testBinding("autonomous-continuation-live"))
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := harness.Submit(context.Background(), StartTurn{ID: "start", Input: UserInput{Text: "do the goal"}})
	if err != nil {
		t.Fatal(err)
	}
	observation, err := harness.Observe(context.Background(), receipt.Cursor-1)
	if err != nil {
		t.Fatal(err)
	}
	cycles := make([]int, 0, 2)
	for {
		select {
		case event := <-observation.Events:
			switch payload := event.Payload.(type) {
			case CycleStartedEvent:
				if payload.OperationID == receipt.OperationID {
					cycles = append(cycles, payload.Cycle)
				}
			case OperationSettledEvent:
				if payload.OperationID == receipt.OperationID {
					if payload.Status != OperationSucceeded {
						t.Fatalf("settlement = %#v", payload)
					}
					if len(cycles) != 2 || cycles[0] != 1 || cycles[1] != 2 {
						t.Fatalf("cycles = %v, want [1 2]", cycles)
					}
					return
				}
			}
		case <-time.After(time.Second):
			t.Fatal("operation did not settle")
		}
	}
}

func TestRecoveredEngineContinuationResumesWithoutHostReplay(t *testing.T) {
	store := NewMemoryJournalStore()
	binding := testBinding("autonomous-continuation-recovery")
	operationID := OperationID("goal-operation")
	input := UserInput{Text: "continue the goal", RestoreDescriptor: []byte(`{"version":1}`)}
	command := FollowUp{ID: "goal-continuation-recovered", OperationID: operationID, Input: input}
	fingerprint, err := CommandFingerprint(command)
	if err != nil {
		t.Fatal(err)
	}
	seedRuntimeEvents(t, store, binding, []EventPayload{
		CommandAcceptedEvent{CommandID: "start", CommandKind: "start_turn", OperationID: operationID, Fingerprint: "start"},
		OperationStartedEvent{OperationID: operationID},
		UserMessageCommittedEvent{Message: Message{ID: "user", Role: RoleUser, Content: "do the goal", Input: UserInput{Text: "do the goal"}, Operation: operationID}},
		CycleStartedEvent{OperationID: operationID, Cycle: 1, SnapshotID: "cycle-1"},
		AssistantMessageCommittedEvent{Message: Message{ID: "assistant", Role: RoleAssistant, Content: "progress", Operation: operationID}},
		CommandAcceptedEvent{CommandID: command.ID, CommandKind: "engine_continuation", OperationID: operationID, Fingerprint: fingerprint},
		QueueEnqueuedEvent{Item: QueuedInput{
			CommandID: command.ID, OperationID: operationID, Delivery: DeliveryFollowUp,
			Input: input, Autonomous: true,
		}},
	})
	runs := make(chan EngineRequest, 1)
	engine := autonomousContinuationEngineFunc(func(_ context.Context, request EngineRequest, emit EngineEventSink) (EngineResult, error) {
		runs <- request
		if err := emit(EngineAssistantFinal{Content: "completed after recovery"}); err != nil {
			return EngineResult{}, err
		}
		return EngineResult{Status: EngineCompleted}, nil
	})
	runtime, err := NewRuntime(EngineFactoryFunc(func(context.Context, BindingRef) (Engine, error) {
		return engine, nil
	}), store, RuntimeConfig{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	harness, err := runtime.Open(context.Background(), binding)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case request := <-runs:
		if request.Snapshot.OperationID != operationID || request.Snapshot.Cycle != 2 ||
			request.Snapshot.CommandID != command.ID || request.Snapshot.Input.Text != input.Text {
			t.Fatalf("recovered continuation = %#v", request.Snapshot)
		}
	case <-time.After(time.Second):
		t.Fatal("recovered autonomous continuation did not resume")
	}
	waitForTerminalOperation(t, harness, operationID)
}
