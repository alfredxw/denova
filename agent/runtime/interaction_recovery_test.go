package runtime

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

type recoveredInteractionEngine struct {
	requests chan EngineRequest
}

func (engine *recoveredInteractionEngine) ResolveInteraction(_ context.Context, request InteractionResolveRequest) (json.RawMessage, error) {
	return cloneRawMessage(request.Response), nil
}

func (engine *recoveredInteractionEngine) Run(_ context.Context, request EngineRequest, emit EngineEventSink) (EngineResult, error) {
	engine.requests <- request
	if err := emit(EngineAssistantFinal{Content: "resumed", State: json.RawMessage(`{"resumed":true}`)}); err != nil {
		return EngineResult{}, err
	}
	return EngineResult{Status: EngineCompleted}, nil
}

func TestRecoveredInteractionResumesOnlyAfterDurableResolution(t *testing.T) {
	store := NewMemoryJournalStore()
	binding := testBindingAt("/book", "interaction-recovery")
	ref, err := BindingReference(binding)
	if err != nil {
		t.Fatal(err)
	}
	operationID := OperationID("interaction-operation")
	request := json.RawMessage(`{"id":"ask-call","kind":"ask"}`)
	seedRuntimeEvents(t, store, ref, []EventPayload{
		CommandAcceptedEvent{CommandID: "start", CommandKind: "start_turn", OperationID: operationID, Fingerprint: "start"},
		OperationStartedEvent{OperationID: operationID},
		UserMessageCommittedEvent{Message: Message{ID: "user", Role: RoleUser, Content: "question", Input: UserInput{Text: "question"}, Operation: operationID}},
		CycleStartedEvent{OperationID: operationID, Cycle: 1, SnapshotID: "snapshot"},
		EngineStateCommittedEvent{State: json.RawMessage(`{"checkpoint":true}`), Descriptor: describePayload(json.RawMessage(`{"checkpoint":true}`))},
		InteractionRequestedEvent{
			ID: "ask-call", OperationID: operationID, Cycle: 1, ToolCallID: "tool-call",
			Request: request, Descriptor: describePayload(request),
		},
	})
	engine := &recoveredInteractionEngine{requests: make(chan EngineRequest, 1)}
	runtime, err := NewRuntime(EngineFactoryFunc(func(context.Context, BindingRef) (Engine, error) {
		return engine, nil
	}), store, RuntimeConfig{})
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close(context.Background())
	harness, err := runtime.Open(context.Background(), binding)
	if err != nil {
		t.Fatal(err)
	}
	status, err := harness.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !status.RecoveryPaused || len(status.Interactions) != 1 || status.Interactions[0].Resolved {
		t.Fatalf("recovered status = %#v", status)
	}
	select {
	case <-engine.requests:
		t.Fatal("opening recovery reran Engine before an exact response")
	default:
	}
	response := json.RawMessage(`{"answers":[{"question_id":"q","text":"answer"}]}`)
	command := ResolveInteraction{
		ID: "resolve", OperationID: operationID, InteractionID: "ask-call", Response: response,
	}
	receipt, err := harness.Submit(context.Background(), command)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.OperationID != operationID {
		t.Fatalf("receipt = %#v", receipt)
	}
	select {
	case resumed := <-engine.requests:
		if len(resumed.Snapshot.Interactions) != 1 || !resumed.Snapshot.Interactions[0].Resolved ||
			string(resumed.Snapshot.Interactions[0].Response) != string(response) {
			t.Fatalf("resumed snapshot = %#v", resumed.Snapshot)
		}
	case <-time.After(time.Second):
		t.Fatal("durable Interaction resolution did not resume Engine")
	}
	waitForTerminalOperation(t, harness, operationID)
	replayed, err := harness.Submit(context.Background(), command)
	if err != nil || !replayed.Replayed {
		t.Fatalf("resolution replay = %#v err=%v", replayed, err)
	}
}

var _ Engine = (*recoveredInteractionEngine)(nil)
var _ EngineInteractionResolver = (*recoveredInteractionEngine)(nil)
