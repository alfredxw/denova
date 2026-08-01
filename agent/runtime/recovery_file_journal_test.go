package runtime_test

import (
	"context"
	"encoding/json"
	runstate "github.com/alfredxw/denova/agent/runtime"
	filejournal "github.com/alfredxw/denova/agent/runtime/filejournal"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestFileJournalReopensInputMaterializationRecoveryMarkers(t *testing.T) {
	root := filepath.Join(t.TempDir(), "journals")
	store, err := filejournal.NewStore(root)
	if err != nil {
		t.Fatal(err)
	}
	binding := testBindingAt("/book", "file-input-recovery")
	ref, err := runstate.BindingReference(binding)
	if err != nil {
		t.Fatal(err)
	}
	operationID := runstate.OperationID("operation-file-input-recovery")
	commandID := runstate.CommandID("follow-up-file-input-recovery")
	acceptedInput := runstate.UserInput{Text: "accepted", TurnSpecRef: "durable-follow-up-descriptor"}
	accepted := runstate.FollowUp{ID: commandID, OperationID: operationID, Input: acceptedInput}
	later := runstate.FollowUp{
		ID: "later-cancelled-follow-up", OperationID: operationID,
		Input: runstate.UserInput{Text: "later", TurnSpecRef: "later-descriptor"},
	}
	seedFileRuntimeEvents(t, store, ref, []runstate.EventPayload{
		runstate.CommandAcceptedEvent{CommandID: "start", CommandKind: "start_turn", OperationID: operationID, Fingerprint: "start"},
		runstate.OperationStartedEvent{OperationID: operationID},
		runstate.UserMessageCommittedEvent{Message: runstate.Message{ID: "parent", Role: runstate.RoleUser, Content: "parent", Input: runstate.UserInput{Text: "parent"}, Operation: operationID}},
		runstate.CycleStartedEvent{OperationID: operationID, Cycle: 1, SnapshotID: "parent-snapshot"},
		runstate.CommandAcceptedEvent{CommandID: commandID, CommandKind: string(runstate.DeliveryFollowUp), OperationID: operationID, Fingerprint: fileRecoveryCommandFingerprint(t, accepted)},
		runstate.QueueEnqueuedEvent{Item: runstate.QueuedInput{CommandID: commandID, OperationID: operationID, Delivery: runstate.DeliveryFollowUp, Input: acceptedInput}},
		runstate.SavePointCommittedEvent{OperationID: operationID, Cycle: 1},
		runstate.QueueConsumedEvent{CommandID: commandID, Delivery: runstate.DeliveryFollowUp},
		runstate.UserMessageCommittedEvent{Message: runstate.Message{ID: "accepted", Role: runstate.RoleUser, Content: "accepted", Input: acceptedInput, Operation: operationID}},
		runstate.CycleStartedEvent{OperationID: operationID, Cycle: 2, SnapshotID: "accepted-snapshot"},
		runstate.InputMaterializationRecoveryPendingEvent{OperationID: operationID, Cycle: 2, CommandID: commandID, Delivery: runstate.DeliveryFollowUp},
		// Retain only this later cancelled receipt in the actor's hot cache. The
		// exact recovery below must use FileJournal's durable command index for
		// commandID while rebuilding the accepted input from the durable snapshot.
		runstate.CommandAcceptedEvent{CommandID: later.ID, CommandKind: string(runstate.DeliveryFollowUp), OperationID: operationID, Fingerprint: fileRecoveryCommandFingerprint(t, later)},
		runstate.QueueEnqueuedEvent{Item: runstate.QueuedInput{CommandID: later.ID, OperationID: operationID, Delivery: runstate.DeliveryFollowUp, Input: later.Input}},
		runstate.QueueCancelledEvent{CommandID: later.ID, Reason: "test retained-command eviction"},
	})

	reopenedStore, err := filejournal.NewStore(root)
	if err != nil {
		t.Fatal(err)
	}
	engine := newFileInputRecoveryEngine()
	runtime, err := runstate.NewRuntime(
		runstate.EngineFactoryFunc(func(context.Context, runstate.BindingRef) (runstate.Engine, error) { return engine, nil }),
		reopenedStore,
		runstate.RuntimeConfig{RetainedCommandLimit: 1},
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
		status.InputRecovery.OperationID != operationID || status.InputRecovery.Cycle != 2 || status.InputRecovery.Delivery != runstate.DeliveryFollowUp {
		t.Fatalf("actor reopened pending marker = %#v", status)
	}
	receipt, err := harness.RecoverAcceptedInput(context.Background(), runstate.RecoveryAction{
		Kind: runstate.DeliveryFollowUp, CommandID: commandID, OperationID: operationID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !receipt.Replayed || receipt.CommandID != commandID || receipt.OperationID != operationID {
		t.Fatalf("cold recovery receipt = %#v", receipt)
	}
	wantInput := func(name string, got runstate.UserInput) {
		t.Helper()
		if got.Text != acceptedInput.Text || got.TurnSpecRef != acceptedInput.TurnSpecRef {
			t.Fatalf("%s rebuilt input = %#v, want %#v", name, got, acceptedInput)
		}
	}
	select {
	case restored := <-engine.restored:
		if restored.CommandID != commandID || restored.OperationID != operationID || restored.Delivery != runstate.DeliveryFollowUp {
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
	waitForOperationSettled(t, harness, 0, operationID)
	status, err = harness.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if status.Phase != runstate.PhaseIdle || status.RecoveryPaused || status.InputRecovery != nil ||
		status.LastOperation == nil || status.LastOperation.OperationID != operationID || status.LastOperation.Status != runstate.OperationSucceeded {
		t.Fatalf("actor recovery terminal status = %#v", status)
	}
	if engine.restoreCalls.Load() != 1 || engine.materializeCalls.Load() != 1 || engine.runCalls.Load() != 1 {
		t.Fatalf("actor recovery calls restore=%d materialize=%d run=%d, want 1/1/1",
			engine.restoreCalls.Load(), engine.materializeCalls.Load(), engine.runCalls.Load())
	}
	if err := runtime.Close(context.Background()); err != nil {
		t.Fatal(err)
	}

	settledStore, err := filejournal.NewStore(root)
	if err != nil {
		t.Fatal(err)
	}
	settledRuntime, err := runstate.NewRuntime(runstate.NewScriptedEngine(), settledStore, runstate.RuntimeConfig{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = settledRuntime.Close(context.Background()) })
	settledHarness, err := settledRuntime.Open(context.Background(), binding)
	if err != nil {
		t.Fatal(err)
	}
	resumed, err := settledHarness.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if resumed.InputRecovery != nil || resumed.RecoveryPaused || resumed.Phase != runstate.PhaseIdle ||
		resumed.LastOperation == nil || resumed.LastOperation.OperationID != operationID || resumed.LastOperation.Status != runstate.OperationSucceeded {
		t.Fatalf("reopened resumed marker = %#v", resumed)
	}
}

type fileInputRecoveryEngine struct {
	restored     chan runstate.QueuedInput
	materialized chan runstate.InputMaterializationRequest
	ran          chan runstate.EngineRequest

	restoreCalls     atomic.Int32
	materializeCalls atomic.Int32
	runCalls         atomic.Int32
}

func newFileInputRecoveryEngine() *fileInputRecoveryEngine {
	return &fileInputRecoveryEngine{
		restored: make(chan runstate.QueuedInput, 1), materialized: make(chan runstate.InputMaterializationRequest, 1),
		ran: make(chan runstate.EngineRequest, 1),
	}
}

func (e *fileInputRecoveryEngine) RestorePendingInput(_ context.Context, input runstate.QueuedInput) error {
	e.restoreCalls.Add(1)
	e.restored <- input
	return nil
}

func (*fileInputRecoveryEngine) PlanInputMaterialization(_ context.Context, request runstate.InputMaterializationRequest) (runstate.InputMaterializationPlan, error) {
	return runstate.InputMaterializationPlan{Required: true, Hash: "sha256:file:" + string(request.Snapshot.CommandID)}, nil
}

func (e *fileInputRecoveryEngine) MaterializeInput(
	_ context.Context,
	request runstate.InputMaterializationRequest,
	_ runstate.InputMaterializationPlan,
) (runstate.InputMaterializationReceipt, error) {
	e.materializeCalls.Add(1)
	e.materialized <- request
	return runstate.InputMaterializationReceipt{Revision: "file-input:cycle-2"}, nil
}

func (e *fileInputRecoveryEngine) Run(_ context.Context, request runstate.EngineRequest, _ runstate.EngineEventSink) (runstate.EngineResult, error) {
	e.runCalls.Add(1)
	e.ran <- request
	return runstate.EngineResult{Status: runstate.EngineCompleted}, nil
}

func seedFileRuntimeEvents(t *testing.T, store runstate.JournalStore, ref runstate.BindingRef, payloads []runstate.EventPayload) {
	t.Helper()
	key, err := json.Marshal(ref)
	if err != nil {
		t.Fatal(err)
	}
	journal, err := store.OpenJournal(context.Background(), string(key))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := journal.Append(context.Background(), 0, payloads); err != nil {
		_ = journal.Close()
		t.Fatal(err)
	}
	if err := journal.Close(); err != nil {
		t.Fatal(err)
	}
}

func fileRecoveryCommandFingerprint(t *testing.T, command runstate.Command) string {
	t.Helper()
	fingerprint, err := runstate.CommandFingerprint(command)
	if err != nil {
		t.Fatal(err)
	}
	return fingerprint
}
