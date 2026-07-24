package runtime

import (
	"context"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestFileJournalReopensInputMaterializationRecoveryMarkers(t *testing.T) {
	root := filepath.Join(t.TempDir(), "journals")
	store, err := NewFileJournalStore(root)
	if err != nil {
		t.Fatal(err)
	}
	binding := testBindingAt("/book", "file-input-recovery")
	ref, err := BindingReference(binding)
	if err != nil {
		t.Fatal(err)
	}
	operationID := OperationID("operation-file-input-recovery")
	commandID := CommandID("follow-up-file-input-recovery")
	acceptedInput := UserInput{Text: "accepted", TurnSpecRef: "durable-follow-up-descriptor"}
	accepted := FollowUp{ID: commandID, OperationID: operationID, Input: acceptedInput}
	later := FollowUp{
		ID: "later-cancelled-follow-up", OperationID: operationID,
		Input: UserInput{Text: "later", TurnSpecRef: "later-descriptor"},
	}
	seedRuntimeEvents(t, store, ref, []EventPayload{
		CommandAcceptedEvent{CommandID: "start", CommandKind: "start_turn", OperationID: operationID, Fingerprint: "start"},
		OperationStartedEvent{OperationID: operationID},
		UserMessageCommittedEvent{Message: Message{ID: "parent", Role: RoleUser, Content: "parent", Input: UserInput{Text: "parent"}, Operation: operationID}},
		CycleStartedEvent{OperationID: operationID, Cycle: 1, SnapshotID: "parent-snapshot"},
		CommandAcceptedEvent{CommandID: commandID, CommandKind: string(DeliveryFollowUp), OperationID: operationID, Fingerprint: fingerprintCommand(accepted)},
		QueueEnqueuedEvent{Item: QueuedInput{CommandID: commandID, OperationID: operationID, Delivery: DeliveryFollowUp, Input: acceptedInput}},
		SavePointCommittedEvent{OperationID: operationID, Cycle: 1},
		QueueConsumedEvent{CommandID: commandID, Delivery: DeliveryFollowUp},
		UserMessageCommittedEvent{Message: Message{ID: "accepted", Role: RoleUser, Content: "accepted", Input: acceptedInput, Operation: operationID}},
		CycleStartedEvent{OperationID: operationID, Cycle: 2, SnapshotID: "accepted-snapshot"},
		InputMaterializationRecoveryPendingEvent{OperationID: operationID, Cycle: 2, CommandID: commandID, Delivery: DeliveryFollowUp},
		// Retain only this later cancelled receipt in the actor's hot cache. The
		// exact recovery below must use FileJournal's durable command index for
		// commandID while rebuilding the accepted input from the durable snapshot.
		CommandAcceptedEvent{CommandID: later.ID, CommandKind: string(DeliveryFollowUp), OperationID: operationID, Fingerprint: fingerprintCommand(later)},
		QueueEnqueuedEvent{Item: QueuedInput{CommandID: later.ID, OperationID: operationID, Delivery: DeliveryFollowUp, Input: later.Input}},
		QueueCancelledEvent{CommandID: later.ID, Reason: "test retained-command eviction"},
	})

	reopenedStore, err := NewFileJournalStore(root)
	if err != nil {
		t.Fatal(err)
	}
	engine := newFileInputRecoveryEngine()
	runtime, err := NewRuntime(
		EngineFactoryFunc(func(context.Context, BindingRef) (Engine, error) { return engine, nil }),
		reopenedStore,
		RuntimeConfig{RetainedCommandLimit: 1},
	)
	if err != nil {
		t.Fatal(err)
	}
	harness, err := runtime.Open(context.Background(), binding)
	if err != nil {
		t.Fatal(err)
	}
	status, err := harness.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !status.RecoveryPaused || status.InputRecovery == nil || status.InputRecovery.CommandID != commandID ||
		status.InputRecovery.OperationID != operationID || status.InputRecovery.Cycle != 2 || status.InputRecovery.Delivery != DeliveryFollowUp {
		t.Fatalf("actor reopened pending marker = %#v", status)
	}
	receipt, err := harness.RecoverAcceptedInput(context.Background(), RecoveryAction{
		Kind: DeliveryFollowUp, CommandID: commandID, OperationID: operationID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !receipt.Replayed || receipt.CommandID != commandID || receipt.OperationID != operationID {
		t.Fatalf("cold recovery receipt = %#v", receipt)
	}
	wantInput := func(name string, got UserInput) {
		t.Helper()
		if got.Text != acceptedInput.Text || got.TurnSpecRef != acceptedInput.TurnSpecRef {
			t.Fatalf("%s rebuilt input = %#v, want %#v", name, got, acceptedInput)
		}
	}
	select {
	case restored := <-engine.restored:
		if restored.CommandID != commandID || restored.OperationID != operationID || restored.Delivery != DeliveryFollowUp {
			t.Fatalf("restored queued identity = %#v", restored)
		}
		wantInput("restorer", restored.Input)
	case <-time.After(500 * time.Millisecond):
		t.Fatal("cold recovery did not restore the durable input descriptor")
	}
	select {
	case materialized := <-engine.materialized:
		if materialized.Snapshot.CommandID != commandID || materialized.Snapshot.OperationID != operationID || materialized.Snapshot.Cycle != 2 {
			t.Fatalf("materialized snapshot identity = %#v", materialized.Snapshot)
		}
		wantInput("materializer", materialized.Snapshot.Input)
	case <-time.After(500 * time.Millisecond):
		t.Fatal("cold recovery did not materialize the durable input")
	}
	select {
	case ran := <-engine.ran:
		if ran.Snapshot.CommandID != commandID || ran.Snapshot.OperationID != operationID || ran.Snapshot.Cycle != 2 {
			t.Fatalf("engine snapshot identity = %#v", ran.Snapshot)
		}
		wantInput("engine", ran.Snapshot.Input)
	case <-time.After(500 * time.Millisecond):
		t.Fatal("cold recovery did not start the exact accepted cycle")
	}
	waitForTerminalOperation(t, harness, operationID)
	status, err = harness.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if status.Phase != PhaseIdle || status.RecoveryPaused || status.InputRecovery != nil ||
		status.LastOperation == nil || status.LastOperation.OperationID != operationID || status.LastOperation.Status != OperationSucceeded {
		t.Fatalf("actor recovery terminal status = %#v", status)
	}
	if engine.restoreCalls.Load() != 1 || engine.materializeCalls.Load() != 1 || engine.runCalls.Load() != 1 {
		t.Fatalf("actor recovery calls restore=%d materialize=%d run=%d, want 1/1/1",
			engine.restoreCalls.Load(), engine.materializeCalls.Load(), engine.runCalls.Load())
	}
	if err := runtime.Close(context.Background()); err != nil {
		t.Fatal(err)
	}

	resumed, reopenedJournal := reopenFileRecoveryState(t, root, ref)
	defer reopenedJournal.Close()
	if resumed.inputRecovery != nil || resumed.recoveryPaused || resumed.phase != PhaseIdle ||
		resumed.lastOperation == nil || resumed.lastOperation.OperationID != operationID || resumed.lastOperation.Status != OperationSucceeded {
		t.Fatalf("reopened resumed marker = %#v", resumed.statusSnapshot(1<<20))
	}
}

type fileInputRecoveryEngine struct {
	restored     chan QueuedInput
	materialized chan InputMaterializationRequest
	ran          chan EngineRequest

	restoreCalls     atomic.Int32
	materializeCalls atomic.Int32
	runCalls         atomic.Int32
}

func newFileInputRecoveryEngine() *fileInputRecoveryEngine {
	return &fileInputRecoveryEngine{
		restored: make(chan QueuedInput, 1), materialized: make(chan InputMaterializationRequest, 1),
		ran: make(chan EngineRequest, 1),
	}
}

func (e *fileInputRecoveryEngine) RestorePendingInput(_ context.Context, input QueuedInput) error {
	e.restoreCalls.Add(1)
	e.restored <- input
	return nil
}

func (*fileInputRecoveryEngine) PlanInputMaterialization(_ context.Context, request InputMaterializationRequest) (InputMaterializationPlan, error) {
	return InputMaterializationPlan{Required: true, Hash: "sha256:file:" + string(request.Snapshot.CommandID)}, nil
}

func (e *fileInputRecoveryEngine) MaterializeInput(
	_ context.Context,
	request InputMaterializationRequest,
	_ InputMaterializationPlan,
) (InputMaterializationReceipt, error) {
	e.materializeCalls.Add(1)
	e.materialized <- request
	return InputMaterializationReceipt{Revision: "file-input:cycle-2"}, nil
}

func (e *fileInputRecoveryEngine) Run(_ context.Context, request EngineRequest, _ EngineEventSink) (EngineResult, error) {
	e.runCalls.Add(1)
	e.ran <- request
	return EngineResult{Status: EngineCompleted}, nil
}

func reopenFileRecoveryState(t *testing.T, root string, ref BindingRef) (harnessState, Journal) {
	t.Helper()
	store, err := NewFileJournalStore(root)
	if err != nil {
		t.Fatal(err)
	}
	journal, err := store.OpenJournal(context.Background(), bindingJournalKey(ref))
	if err != nil {
		t.Fatal(err)
	}
	state := newHarnessState(ref)
	if _, err := replayJournalState(context.Background(), journal, func(event Event) error { return state.reduce(event) }); err != nil {
		journal.Close()
		t.Fatal(err)
	}
	return state, journal
}
