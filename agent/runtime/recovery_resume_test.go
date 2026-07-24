package runtime

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestAcceptedTransientInputWaitsForExactReplayAfterCrash(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name     string
		delivery DeliveryKind
		command  func(OperationID, UserInput) Command
	}{
		{
			name: "steer", delivery: DeliverySteer,
			command: func(operationID OperationID, input UserInput) Command {
				return Steer{ID: "queued-steer", OperationID: operationID, Input: input}
			},
		},
		{
			name: "follow-up", delivery: DeliveryFollowUp,
			command: func(operationID OperationID, input UserInput) Command {
				return FollowUp{ID: "queued-follow-up", OperationID: operationID, Input: input}
			},
		},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			store := NewMemoryJournalStore()
			binding := testBindingAt("/book", "recover-"+test.name)
			ref, err := BindingReference(binding)
			if err != nil {
				t.Fatal(err)
			}
			operationID := OperationID("operation-accepted")
			input := UserInput{Text: "recover this input", TurnSpecRef: "turn-ref"}
			command := test.command(operationID, input)
			seedRuntimeEvents(t, store, ref, []EventPayload{
				CommandAcceptedEvent{CommandID: "start", CommandKind: "start_turn", OperationID: operationID, Fingerprint: "start"},
				OperationStartedEvent{OperationID: operationID},
				UserMessageCommittedEvent{Message: Message{
					ID: "user", Role: RoleUser, Content: "original", Input: UserInput{Text: "original"}, Operation: operationID,
				}},
				CycleStartedEvent{OperationID: operationID, Cycle: 1, SnapshotID: "snapshot-original"},
				CommandAcceptedEvent{
					CommandID: command.commandID(), CommandKind: string(test.delivery),
					OperationID: operationID, Fingerprint: fingerprintCommand(command),
				},
				QueueEnqueuedEvent{Item: QueuedInput{
					CommandID: command.commandID(), OperationID: operationID,
					Delivery: test.delivery, Input: input,
				}},
			})

			engine := &recoveredQueueEngine{requests: make(chan EngineRequest, 2)}
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
			if status.Phase != PhaseRunning || !status.RecoveryPaused || len(status.Queue) != 1 || status.Queue[0].CommandID != command.commandID() {
				t.Fatalf("accepted input was not preserved in a recovery pause: %#v", status)
			}
			if got := engine.runCalls.Load(); got != 0 {
				t.Fatalf("open recovery ran engine %d times, want zero", got)
			}

			receipt, err := harness.Submit(context.Background(), command)
			if err != nil {
				t.Fatal(err)
			}
			if !receipt.Replayed || receipt.OperationID != operationID {
				t.Fatalf("exact replay receipt = %#v", receipt)
			}
			select {
			case request := <-engine.requests:
				if request.Snapshot.CommandID != command.commandID() || request.Snapshot.OperationID != operationID ||
					request.Snapshot.Cycle != 2 || request.Snapshot.Input.Text != input.Text {
					t.Fatalf("recovered cycle request = %#v", request)
				}
			case <-time.After(500 * time.Millisecond):
				t.Fatal("exact replay did not resume the accepted input")
			}
			waitForTerminalOperation(t, harness, operationID)

			if _, err := harness.Submit(context.Background(), command); err != nil {
				t.Fatal(err)
			}
			if got := engine.runCalls.Load(); got != 1 {
				t.Fatalf("exact command was delivered %d times, want once", got)
			}
		})
	}
}

func TestRecoveredTransientQueuePausesBeforeEachUnavailableInput(t *testing.T) {
	t.Parallel()

	store := NewMemoryJournalStore()
	binding := testBindingAt("/book", "recover-multiple-transient-inputs")
	ref, err := BindingReference(binding)
	if err != nil {
		t.Fatal(err)
	}
	operationID := OperationID("operation-multiple-inputs")
	steer := Steer{
		ID: "queued-steer", OperationID: operationID,
		Input: UserInput{Text: "change direction", TurnSpecRef: "steer-turn-ref"},
	}
	followUp := FollowUp{
		ID: "queued-follow-up", OperationID: operationID,
		Input: UserInput{Text: "then continue", TurnSpecRef: "follow-up-turn-ref"},
	}
	seedRuntimeEvents(t, store, ref, []EventPayload{
		CommandAcceptedEvent{CommandID: "start", CommandKind: "start_turn", OperationID: operationID, Fingerprint: "start"},
		OperationStartedEvent{OperationID: operationID},
		UserMessageCommittedEvent{Message: Message{
			ID: "user", Role: RoleUser, Content: "original", Input: UserInput{Text: "original"}, Operation: operationID,
		}},
		CycleStartedEvent{OperationID: operationID, Cycle: 1, SnapshotID: "snapshot-original"},
		CommandAcceptedEvent{
			CommandID: steer.ID, CommandKind: string(DeliverySteer),
			OperationID: operationID, Fingerprint: fingerprintCommand(steer),
		},
		QueueEnqueuedEvent{Item: QueuedInput{
			CommandID: steer.ID, OperationID: operationID,
			Delivery: DeliverySteer, Input: steer.Input,
		}},
		CommandAcceptedEvent{
			CommandID: followUp.ID, CommandKind: string(DeliveryFollowUp),
			OperationID: operationID, Fingerprint: fingerprintCommand(followUp),
		},
		QueueEnqueuedEvent{Item: QueuedInput{
			CommandID: followUp.ID, OperationID: operationID,
			Delivery: DeliveryFollowUp, Input: followUp.Input,
		}},
	})

	engine := newStagedRecoveryEngine(steer.ID)
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
	if _, err := harness.Submit(context.Background(), steer); err != nil {
		t.Fatal(err)
	}
	var status StatusSnapshot
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		status, err = harness.Status(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if status.InputRecovery != nil {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if status.Phase != PhaseRunning || !status.RecoveryPaused || status.ActiveCycle != 3 || len(status.Queue) != 0 ||
		status.InputRecovery == nil || status.InputRecovery.CommandID != followUp.ID ||
		status.InputRecovery.OperationID != operationID || status.InputRecovery.Delivery != DeliveryFollowUp {
		t.Fatalf("second accepted input was not retained by its exact recovery marker: %#v", status)
	}
	if got := engine.requestsSnapshot(); len(got) != 1 || got[0].Snapshot.CommandID != steer.ID {
		t.Fatalf("engine requests before second exact replay = %#v", got)
	}

	engine.allow(followUp.ID)
	if _, err := harness.RecoverAcceptedInput(context.Background(), RecoveryAction{
		Kind: DeliveryFollowUp, CommandID: followUp.ID, OperationID: operationID,
	}); err != nil {
		t.Fatal(err)
	}
	waitForTerminalOperation(t, harness, operationID)
	requests := engine.requestsSnapshot()
	if len(requests) != 2 || requests[0].Snapshot.CommandID != steer.ID || requests[1].Snapshot.CommandID != followUp.ID {
		t.Fatalf("recovered command delivery order = %#v", requests)
	}
}

func TestStructuralRecoveryResumesOnlyOnExactCommandReplay(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name    string
		kind    StructuralOperationKind
		command Command
		ref     ContextCompactionRef
	}{
		{
			name: "compact", kind: StructuralCompactContext,
			ref: ContextCompactionRef{
				SpecRef: "compact-spec", Source: "session.messages", Purpose: "checkpoint",
				Resource: "session", ExpectedRevision: "context:7",
			},
		},
		{
			name: "remove", kind: StructuralRemoveCompaction,
			ref: ContextCompactionRef{
				SpecRef: "remove-spec", Source: "session.compaction", Purpose: "restore history",
				Resource: "session", ExpectedRevision: "context:8", CompactionID: "cc-1",
			},
		},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			commandID := CommandID("structural-" + test.name)
			switch test.kind {
			case StructuralCompactContext:
				test.command = CompactIfNeeded{ID: commandID, Ref: test.ref}
			case StructuralRemoveCompaction:
				test.command = RemoveCompaction{ID: commandID, Ref: test.ref}
			}
			store := NewMemoryJournalStore()
			binding := testBindingAt("/book", "structural-"+test.name)
			ref, err := BindingReference(binding)
			if err != nil {
				t.Fatal(err)
			}
			operationID := OperationID("operation-" + test.name)
			snapshot := StructuralOperationSnapshot{
				Binding: ref, CommandID: commandID, OperationID: operationID,
				Cycle: 1, Kind: test.kind, Ref: test.ref, ContextCursor: 2,
			}
			commandKind := "compact_context"
			if test.kind == StructuralRemoveCompaction {
				commandKind = "remove_compaction"
			}
			seedRuntimeEvents(t, store, ref, []EventPayload{
				CommandAcceptedEvent{CommandID: commandID, CommandKind: commandKind, OperationID: operationID, Fingerprint: fingerprintCommand(test.command)},
				OperationStartedEvent{OperationID: operationID, Phase: PhaseCompacting, Structural: &snapshot},
			})

			engine := &recoveredStructuralEngine{requests: make(chan StructuralEngineRequest, 2)}
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
			if status.Phase != PhaseCompacting || !status.RecoveryPaused || status.ActiveStructural == nil {
				t.Fatalf("structural operation was not left retryable: %#v", status)
			}
			if got := engine.structuralCalls.Load(); got != 0 {
				t.Fatalf("open recovery ran structural effect %d times", got)
			}

			receipt, err := harness.Submit(context.Background(), test.command)
			if err != nil {
				t.Fatal(err)
			}
			if !receipt.Replayed || receipt.OperationID != operationID {
				t.Fatalf("structural replay receipt = %#v", receipt)
			}
			select {
			case request := <-engine.requests:
				if !reflect.DeepEqual(request.Snapshot, snapshot) {
					t.Fatalf("resumed structural snapshot = %#v, want %#v", request.Snapshot, snapshot)
				}
			case <-time.After(500 * time.Millisecond):
				t.Fatal("exact structural replay did not resume")
			}
			waitForTerminalOperation(t, harness, operationID)
			if _, err := harness.Submit(context.Background(), test.command); err != nil {
				t.Fatal(err)
			}
			if got := engine.structuralCalls.Load(); got != 1 {
				t.Fatalf("structural command executed %d times, want once", got)
			}
		})
	}
}

func TestStructuralRecoverySettlesCanonicalCommitBeforeReplay(t *testing.T) {
	t.Parallel()

	store := NewMemoryJournalStore()
	binding := testBindingAt("/book", "structural-canonical")
	ref, err := BindingReference(binding)
	if err != nil {
		t.Fatal(err)
	}
	command := CompactIfNeeded{ID: "structural-canonical", Ref: ContextCompactionRef{
		SpecRef: "canonical-spec", Source: "session.messages", Purpose: "checkpoint",
		Resource: "session", ExpectedRevision: "context:7",
	}}
	operationID := OperationID("operation-canonical")
	snapshot := StructuralOperationSnapshot{
		Binding: ref, CommandID: command.ID, OperationID: operationID,
		Cycle: 1, Kind: StructuralCompactContext, Ref: command.Ref, ContextCursor: 2,
	}
	identity := DomainCommitIdentity{CommandID: command.ID, OperationID: operationID, Cycle: 1, Stage: DomainCommitOutput}
	seedRuntimeEvents(t, store, ref, []EventPayload{
		CommandAcceptedEvent{CommandID: command.ID, CommandKind: "compact_context", OperationID: operationID, Fingerprint: fingerprintCommand(command)},
		OperationStartedEvent{OperationID: operationID, Phase: PhaseCompacting, Structural: &snapshot},
		DomainCommitIntentAcceptedEvent{Identity: identity, Hash: "sha256:canonical"},
	})
	engine := &recoveredStructuralEngine{
		requests: make(chan StructuralEngineRequest, 1),
		reconcile: func(request DomainCommitReconcileRequest) (DomainCommitReconcileResult, error) {
			if request.Structural == nil || !reflect.DeepEqual(*request.Structural, snapshot) || request.Commit.Identity != identity || request.Commit.Hash != "sha256:canonical" {
				return DomainCommitReconcileResult{}, errors.New("wrong canonical reconciliation identity")
			}
			return DomainCommitReconcileResult{Found: true, Revision: "context:8"}, nil
		},
	}
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
	if status.Phase != PhaseIdle || status.LastOperation == nil || status.LastOperation.Status != OperationSucceeded {
		t.Fatalf("canonical structural commit was not settled: %#v", status)
	}
	if _, err := harness.Submit(context.Background(), command); err != nil {
		t.Fatal(err)
	}
	if got := engine.structuralCalls.Load(); got != 0 {
		t.Fatalf("canonical structural commit was rerun %d times", got)
	}
}

func TestCloseRecoveryPausedStructuralOperationSettlesWithoutEngineCallback(t *testing.T) {
	t.Parallel()

	store := NewMemoryJournalStore()
	binding := testBindingAt("/book", "close-paused-structural")
	ref, err := BindingReference(binding)
	if err != nil {
		t.Fatal(err)
	}
	command := CompactIfNeeded{ID: "close-paused-structural", Ref: ContextCompactionRef{
		SpecRef: "close-paused-spec", Source: "session.messages", Purpose: "checkpoint",
		Resource: "close-paused-structural", ExpectedRevision: "context:7",
	}}
	operationID := OperationID("close-paused-operation")
	snapshot := StructuralOperationSnapshot{
		Binding: ref, CommandID: command.ID, OperationID: operationID,
		Cycle: 1, Kind: StructuralCompactContext, Ref: command.Ref, ContextCursor: 2,
	}
	seedRuntimeEvents(t, store, ref, []EventPayload{
		CommandAcceptedEvent{CommandID: command.ID, CommandKind: "compact_context", OperationID: operationID, Fingerprint: fingerprintCommand(command)},
		OperationStartedEvent{OperationID: operationID, Phase: PhaseCompacting, Structural: &snapshot},
	})
	engine := &recoveredStructuralEngine{requests: make(chan StructuralEngineRequest, 1)}
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
	if err != nil || !status.RecoveryPaused || status.Phase != PhaseCompacting {
		t.Fatalf("precondition status = %#v err=%v", status, err)
	}
	closed := make(chan error, 1)
	runInternalErrorTestGoroutine(closed, "close recovery-paused binding", func() error {
		return runtime.CloseBinding(context.Background(), binding)
	})
	select {
	case err := <-closed:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("closing a recovery-paused structural operation waited for a nonexistent engine callback")
	}
	if got := engine.structuralCalls.Load(); got != 0 {
		t.Fatalf("close invoked structural engine %d times", got)
	}
}

func TestStructuralRecoveryResumesIntentProvenAbsentFromCanonicalStore(t *testing.T) {
	t.Parallel()

	store := NewMemoryJournalStore()
	binding := testBindingAt("/book", "structural-not-committed")
	ref, err := BindingReference(binding)
	if err != nil {
		t.Fatal(err)
	}
	command := RemoveCompaction{ID: "remove-not-committed", Ref: ContextCompactionRef{
		SpecRef: "remove-not-committed-spec", Source: "session.compaction", Purpose: "restore history",
		Resource: "session", ExpectedRevision: "context:8", CompactionID: "cc-1",
	}}
	operationID := OperationID("operation-not-committed")
	snapshot := StructuralOperationSnapshot{
		Binding: ref, CommandID: command.ID, OperationID: operationID,
		Cycle: 1, Kind: StructuralRemoveCompaction, Ref: command.Ref, ContextCursor: 2,
	}
	identity := DomainCommitIdentity{CommandID: command.ID, OperationID: operationID, Cycle: 1, Stage: DomainCommitOutput}
	seedRuntimeEvents(t, store, ref, []EventPayload{
		CommandAcceptedEvent{CommandID: command.ID, CommandKind: "remove_compaction", OperationID: operationID, Fingerprint: fingerprintCommand(command)},
		OperationStartedEvent{OperationID: operationID, Phase: PhaseCompacting, Structural: &snapshot},
		DomainCommitIntentAcceptedEvent{Identity: identity, Hash: "sha256:not-committed"},
	})
	engine := &recoveredStructuralEngine{
		requests: make(chan StructuralEngineRequest, 1),
		reconcile: func(DomainCommitReconcileRequest) (DomainCommitReconcileResult, error) {
			return DomainCommitReconcileResult{Found: false}, nil
		},
		run: func(_ StructuralEngineRequest, emit EngineEventSink) error {
			if err := emit(EngineDomainCommitIntent{Identity: identity, Hash: "sha256:not-committed"}); err != nil {
				return err
			}
			return emit(EngineDomainCommitReceipt{
				Identity: identity, Hash: "sha256:not-committed", Revision: "context:9",
			})
		},
	}
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
	if status.Phase != PhaseCompacting || !status.RecoveryPaused {
		t.Fatalf("not-committed structural intent was not left resumable: %#v", status)
	}
	if _, err := harness.Submit(context.Background(), command); err != nil {
		t.Fatal(err)
	}
	waitForTerminalOperation(t, harness, operationID)
	status, err = harness.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if status.LastOperation == nil || status.LastOperation.Status != OperationSucceeded ||
		status.LastDomainCommit == nil || status.LastDomainCommit.Revision != "context:9" {
		t.Fatalf("resumed structural commit = %#v", status)
	}
	if got := engine.structuralCalls.Load(); got != 1 {
		t.Fatalf("resumed structural calls = %d, want one", got)
	}
}

func TestOperationRecoveryPauseEventCodecRoundTrips(t *testing.T) {
	t.Parallel()

	want := OperationRecoveryPausedEvent{
		OperationID: "operation", Cycle: 3,
		Reason: "accepted follow-up remains queued for exact replay",
	}
	encoded, err := encodeDurableEvent(Event{Cursor: 9, Durability: EventDurable, Payload: want})
	if err != nil {
		t.Fatal(err)
	}
	if encoded.Type != "operation.recovery_paused" {
		t.Fatalf("encoded recovery event type = %q", encoded.Type)
	}
	decoded, err := decodeDurableEvent(encoded)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := decoded.Payload.(OperationRecoveryPausedEvent)
	if !ok || got != want || decoded.Cursor != 9 {
		t.Fatalf("decoded recovery pause = %#v", decoded)
	}
}

func TestRecoveryOutboxEventsCodecRoundTrip(t *testing.T) {
	t.Parallel()

	identity := DomainCommitIdentity{
		CommandID: "recovered-command", OperationID: "recovered-operation",
		Cycle: 4, Stage: DomainCommitOutput,
	}
	for _, test := range []struct {
		name      string
		eventType string
		payload   EventPayload
	}{
		{
			name: "input pending", eventType: "input_materialization.recovery_pending",
			payload: InputMaterializationRecoveryPendingEvent{
				OperationID: identity.OperationID, Cycle: identity.Cycle,
				CommandID: identity.CommandID, Delivery: DeliveryFollowUp,
			},
		},
		{
			name: "input resumed", eventType: "input_materialization.recovery_resumed",
			payload: InputMaterializationRecoveryResumedEvent{OperationID: identity.OperationID, Cycle: identity.Cycle},
		},
		{
			name: "domain abandonment", eventType: "domain_commit.reconciliation_abandoned",
			payload: DomainCommitReconciliationAbandonedEvent{
				Identity: identity, Hash: "sha256:not-found", Reason: "canonical receipt was authoritatively absent",
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			encoded, err := encodeDurableEvent(Event{Cursor: 17, Durability: EventDurable, Payload: test.payload})
			if err != nil {
				t.Fatal(err)
			}
			if encoded.Type != test.eventType {
				t.Fatalf("encoded event type = %q, want %q", encoded.Type, test.eventType)
			}
			decoded, err := decodeDurableEvent(encoded)
			if err != nil {
				t.Fatal(err)
			}
			if decoded.Cursor != 17 || decoded.Durability != EventDurable || !reflect.DeepEqual(decoded.Payload, test.payload) {
				t.Fatalf("decoded event = %#v, want %#v", decoded, test.payload)
			}
		})
	}
}

type recoveredQueueEngine struct {
	requests chan EngineRequest
	runCalls atomic.Int32
}

func (e *recoveredQueueEngine) Run(_ context.Context, request EngineRequest, _ EngineEventSink) (EngineResult, error) {
	e.runCalls.Add(1)
	e.requests <- request
	return EngineResult{Status: EngineCompleted}, nil
}

func (*recoveredQueueEngine) RestorePendingInput(context.Context, QueuedInput) error { return nil }

type stagedRecoveryEngine struct {
	mu       sync.Mutex
	allowed  map[CommandID]bool
	requests []EngineRequest
}

func newStagedRecoveryEngine(commandIDs ...CommandID) *stagedRecoveryEngine {
	engine := &stagedRecoveryEngine{allowed: make(map[CommandID]bool)}
	for _, commandID := range commandIDs {
		engine.allowed[commandID] = true
	}
	return engine
}

func (e *stagedRecoveryEngine) allow(commandID CommandID) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.allowed[commandID] = true
}

func (e *stagedRecoveryEngine) RestorePendingInput(_ context.Context, input QueuedInput) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.allowed[input.CommandID] {
		return errors.New("pending input runtime dependency is unavailable")
	}
	return nil
}

func (e *stagedRecoveryEngine) Run(_ context.Context, request EngineRequest, _ EngineEventSink) (EngineResult, error) {
	e.mu.Lock()
	e.requests = append(e.requests, request)
	e.mu.Unlock()
	return EngineResult{Status: EngineCompleted}, nil
}

func (e *stagedRecoveryEngine) requestsSnapshot() []EngineRequest {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]EngineRequest(nil), e.requests...)
}

type recoveredStructuralEngine struct {
	requests        chan StructuralEngineRequest
	structuralCalls atomic.Int32
	reconcile       func(DomainCommitReconcileRequest) (DomainCommitReconcileResult, error)
	run             func(StructuralEngineRequest, EngineEventSink) error
}

func (*recoveredStructuralEngine) Run(context.Context, EngineRequest, EngineEventSink) (EngineResult, error) {
	return EngineResult{}, errors.New("turn engine must not run")
}

func (e *recoveredStructuralEngine) RunStructural(_ context.Context, request StructuralEngineRequest, emit EngineEventSink) (EngineResult, error) {
	e.structuralCalls.Add(1)
	e.requests <- request
	if e.run != nil {
		if err := e.run(request, emit); err != nil {
			return EngineResult{}, err
		}
	}
	return EngineResult{Status: EngineCompleted}, nil
}

func (e *recoveredStructuralEngine) ReconcileDomainCommit(_ context.Context, request DomainCommitReconcileRequest) (DomainCommitReconcileResult, error) {
	if e.reconcile == nil {
		return DomainCommitReconcileResult{}, nil
	}
	return e.reconcile(request)
}

func seedRuntimeEvents(t *testing.T, store JournalStore, ref BindingRef, payloads []EventPayload) {
	t.Helper()
	journal, err := store.OpenJournal(context.Background(), bindingJournalKey(ref))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := journal.Append(context.Background(), 0, payloads); err != nil {
		t.Fatal(err)
	}
	if err := journal.Close(); err != nil {
		t.Fatal(err)
	}
}

func waitForTerminalOperation(t *testing.T, harness *Harness, operationID OperationID) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	observation, err := harness.ObserveFromNow(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if observation.Snapshot.LastOperation != nil && observation.Snapshot.LastOperation.OperationID == operationID {
		return
	}
	deadline := time.NewTimer(500 * time.Millisecond)
	defer deadline.Stop()
	for {
		select {
		case event, ok := <-observation.Events:
			if !ok {
				t.Fatalf("operation %s observation closed before settlement", operationID)
			}
			switch payload := event.Payload.(type) {
			case OperationSettledEvent:
				if payload.OperationID == operationID {
					return
				}
			case OperationInterruptedEvent:
				if payload.OperationID == operationID {
					return
				}
			}
		case observeErr := <-observation.Errors:
			if observeErr != nil {
				t.Fatal(observeErr)
			}
		case <-deadline.C:
			t.Fatalf("operation %s did not settle", operationID)
		}
	}
}

func waitForRecoveryPause(t *testing.T, observation Observation, operationID OperationID, cycle int) {
	t.Helper()
	deadline := time.NewTimer(500 * time.Millisecond)
	defer deadline.Stop()
	for {
		select {
		case event, ok := <-observation.Events:
			if !ok {
				t.Fatalf("operation %s observation closed before recovery pause", operationID)
			}
			paused, ok := event.Payload.(OperationRecoveryPausedEvent)
			if ok && paused.OperationID == operationID && paused.Cycle == cycle {
				return
			}
		case observeErr := <-observation.Errors:
			if observeErr != nil {
				t.Fatal(observeErr)
			}
		case <-deadline.C:
			t.Fatalf("operation %s cycle %d did not recovery-pause", operationID, cycle)
		}
	}
}
