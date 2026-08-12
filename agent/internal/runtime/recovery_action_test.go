package runtime

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestAcceptedQueuedInputMaterializationFailureRetainsExactRecoveryAction(t *testing.T) {
	for _, delivery := range []DeliveryKind{DeliverySteer, DeliveryFollowUp, DeliveryNextTurn} {
		t.Run(string(delivery), func(t *testing.T) {
			store := NewMemoryJournalStore()
			binding := testBindingAt("/book", "materialize-"+string(delivery))
			ref := binding
			parentID := OperationID("parent-" + string(delivery))
			operationID := parentID
			if delivery == DeliveryNextTurn {
				operationID = OperationID("successor-" + string(delivery))
			}
			commandID := CommandID("recover-" + string(delivery))
			var command Command
			switch delivery {
			case DeliverySteer:
				command = Steer{ID: commandID, OperationID: parentID, Input: UserInput{Text: "retry me", TurnSpecRef: "retry-ref"}}
			case DeliveryFollowUp:
				command = FollowUp{ID: commandID, OperationID: parentID, Input: UserInput{Text: "retry me", TurnSpecRef: "retry-ref"}}
			case DeliveryNextTurn:
				command = NextTurn{ID: commandID, AfterOperationID: parentID, Input: UserInput{Text: "retry me", TurnSpecRef: "retry-ref"}}
			}
			input := commandInput(command)
			payloads := []EventPayload{
				CommandAcceptedEvent{CommandID: "start", CommandKind: "start_turn", OperationID: parentID, Fingerprint: "start"},
				OperationStartedEvent{OperationID: parentID},
				UserMessageCommittedEvent{Message: Message{ID: "parent", Role: RoleUser, Content: "parent", Input: UserInput{Text: "parent"}, Operation: parentID}},
				CycleStartedEvent{OperationID: parentID, Cycle: 1, SnapshotID: "parent-snapshot"},
				CommandAcceptedEvent{CommandID: commandID, CommandKind: string(delivery), OperationID: operationID, Fingerprint: fingerprintCommand(command)},
				QueueEnqueuedEvent{Item: QueuedInput{CommandID: commandID, OperationID: operationID, Delivery: delivery, Input: input}},
			}
			if delivery == DeliverySteer {
				later := FollowUp{ID: "later-follow-up", OperationID: parentID, Input: UserInput{Text: "later", TurnSpecRef: "later-ref"}}
				payloads = append(payloads,
					CommandAcceptedEvent{CommandID: later.ID, CommandKind: string(DeliveryFollowUp), OperationID: parentID, Fingerprint: fingerprintCommand(later)},
					QueueEnqueuedEvent{Item: QueuedInput{CommandID: later.ID, OperationID: parentID, Delivery: DeliveryFollowUp, Input: later.Input}},
				)
			}
			seedRuntimeEvents(t, store, ref, payloads)

			engine := &failOnceQueuedMaterializationEngine{commandID: commandID, ran: make(chan EngineRequest, 4)}
			runtime, err := NewRuntime(EngineFactoryFunc(func(context.Context, BindingRef) (Engine, error) { return engine, nil }), store, RuntimeConfig{})
			if err != nil {
				t.Fatal(err)
			}
			defer runtime.Close(context.Background())
			harness, err := runtime.Open(context.Background(), binding)
			if err != nil {
				t.Fatal(err)
			}
			action := RecoveryAction{Kind: delivery, CommandID: commandID, OperationID: operationID}
			if _, err := harness.RecoverAcceptedInput(context.Background(), action); err == nil {
				t.Fatal("first materialization unexpectedly succeeded")
			}
			status, err := harness.Status(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if !status.RecoveryPaused || status.InputRecovery == nil || status.InputRecovery.CommandID != commandID ||
				status.InputRecovery.OperationID != operationID || status.InputRecovery.Delivery != delivery {
				t.Fatalf("retryable materialization status = %#v", status)
			}
			if delivery == DeliverySteer {
				if _, err := harness.RecoverAcceptedInput(context.Background(), RecoveryAction{
					Kind: DeliveryFollowUp, CommandID: "later-follow-up", OperationID: parentID,
				}); !errors.Is(err, ErrRecoveryActionChanged) {
					t.Fatalf("later action error = %v, want recovery changed", err)
				}
			}
			receipt, err := harness.RecoverAcceptedInput(context.Background(), action)
			if err != nil || !receipt.Replayed {
				t.Fatalf("second materialization retry = receipt %#v err %v", receipt, err)
			}
			select {
			case request := <-engine.ran:
				if request.Snapshot.CommandID != commandID || request.Snapshot.Input.Text != "retry me" {
					t.Fatalf("retried engine request = %#v", request.Snapshot)
				}
			case <-time.After(500 * time.Millisecond):
				t.Fatal("materialized recovery did not start the exact accepted cycle")
			}
		})
	}
}

func TestAbortRetriesPendingInputMaterializationBeforeDurableSettlement(t *testing.T) {
	for _, test := range []struct {
		name              string
		seedAbort         bool
		wantFirstOpenFail bool
	}{
		{name: "same_abort_replay"},
		{name: "reopen_with_durable_abort", seedAbort: true, wantFirstOpenFail: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := NewMemoryJournalStore()
			binding := testBindingAt("/book", "abort-input-recovery-"+test.name)
			ref, err := BindingReference(binding)
			if err != nil {
				t.Fatal(err)
			}
			operationID := OperationID("operation-abort-input-recovery")
			followUp := FollowUp{
				ID: "accepted-follow-up", OperationID: operationID,
				Input: UserInput{Text: "accepted input", TurnSpecRef: "accepted-input-ref"},
			}
			abort := Abort{
				ID: "stable-recovery-abort", OperationID: operationID,
				Reason: "user explicitly aborted pending recovery",
			}
			payloads := []EventPayload{
				CommandAcceptedEvent{CommandID: "start", CommandKind: "start_turn", OperationID: operationID, Fingerprint: "start"},
				OperationStartedEvent{OperationID: operationID},
				UserMessageCommittedEvent{Message: Message{ID: "parent", Role: RoleUser, Content: "parent", Input: UserInput{Text: "parent"}, Operation: operationID}},
				CycleStartedEvent{OperationID: operationID, Cycle: 1, SnapshotID: "parent-snapshot"},
				CommandAcceptedEvent{CommandID: followUp.ID, CommandKind: string(DeliveryFollowUp), OperationID: operationID, Fingerprint: fingerprintCommand(followUp)},
				QueueEnqueuedEvent{Item: QueuedInput{CommandID: followUp.ID, OperationID: operationID, Delivery: DeliveryFollowUp, Input: followUp.Input}},
				SavePointCommittedEvent{OperationID: operationID, Cycle: 1},
				QueueConsumedEvent{CommandID: followUp.ID, Delivery: DeliveryFollowUp},
				UserMessageCommittedEvent{Message: Message{ID: "accepted", Role: RoleUser, Content: followUp.Input.Text, Input: followUp.Input, Operation: operationID}},
				CycleStartedEvent{OperationID: operationID, Cycle: 2, SnapshotID: "accepted-snapshot"},
				InputMaterializationRecoveryPendingEvent{OperationID: operationID, Cycle: 2, CommandID: followUp.ID, Delivery: DeliveryFollowUp},
			}
			if test.seedAbort {
				payloads = append(payloads,
					CommandAcceptedEvent{CommandID: abort.ID, CommandKind: "abort", OperationID: operationID, Fingerprint: fingerprintCommand(abort)},
					AbortRequestedEvent{OperationID: operationID, Reason: abort.Reason},
				)
			}
			seedRuntimeEvents(t, store, ref, payloads)

			engine := &failOnceQueuedMaterializationEngine{commandID: followUp.ID, ran: make(chan EngineRequest, 1)}
			runtime, err := NewRuntime(EngineFactoryFunc(func(context.Context, BindingRef) (Engine, error) { return engine, nil }), store, RuntimeConfig{})
			if err != nil {
				t.Fatal(err)
			}
			defer runtime.Close(context.Background())
			harness, openErr := runtime.Open(context.Background(), binding)
			if test.wantFirstOpenFail {
				if openErr == nil {
					t.Fatal("first reopen unexpectedly crossed the pending input outbox")
				}
				harness, openErr = runtime.Open(context.Background(), binding)
			}
			if openErr != nil {
				t.Fatal(openErr)
			}

			if !test.seedAbort {
				firstReceipt, submitErr := harness.Submit(context.Background(), abort)
				if submitErr == nil || firstReceipt.CommandID != abort.ID {
					t.Fatalf("first abort = receipt %#v err %v, want durable receipt plus dependency error", firstReceipt, submitErr)
				}
				secondReceipt, submitErr := harness.Submit(context.Background(), abort)
				if submitErr != nil || !secondReceipt.Replayed {
					t.Fatalf("same abort replay = receipt %#v err %v", secondReceipt, submitErr)
				}
			}

			status, err := harness.Status(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if status.Phase != PhaseIdle || status.RecoveryPaused || status.InputRecovery != nil ||
				status.LastOperation == nil || status.LastOperation.OperationID != operationID || status.LastOperation.Status != OperationAborted {
				t.Fatalf("settled abort status = %#v", status)
			}
			if engine.materializeCalls.Load() != 2 || engine.runCalls.Load() != 0 {
				t.Fatalf("materialize calls=%d engine runs=%d, want 2/0", engine.materializeCalls.Load(), engine.runCalls.Load())
			}
		})
	}
}

func commandInput(command Command) UserInput {
	switch typed := command.(type) {
	case Steer:
		return typed.Input
	case FollowUp:
		return typed.Input
	case NextTurn:
		return typed.Input
	default:
		return UserInput{}
	}
}

type failOnceQueuedMaterializationEngine struct {
	commandID        CommandID
	materializeCalls atomic.Int32
	runCalls         atomic.Int32
	ran              chan EngineRequest
}

func (e *failOnceQueuedMaterializationEngine) RestorePendingInput(context.Context, QueuedInput) error {
	return nil
}

func (e *failOnceQueuedMaterializationEngine) PlanInputMaterialization(_ context.Context, request InputMaterializationRequest) (InputMaterializationPlan, error) {
	if request.Snapshot.CommandID != e.commandID {
		return InputMaterializationPlan{}, nil
	}
	return InputMaterializationPlan{Required: true, Hash: "sha256:" + string(e.commandID)}, nil
}

func (e *failOnceQueuedMaterializationEngine) MaterializeInput(_ context.Context, _ InputMaterializationRequest, _ InputMaterializationPlan) (InputMaterializationReceipt, error) {
	if e.materializeCalls.Add(1) == 1 {
		return InputMaterializationReceipt{}, errors.New("canonical input temporarily unavailable")
	}
	return InputMaterializationReceipt{Revision: "input:retry"}, nil
}

func (*failOnceQueuedMaterializationEngine) ReconcileDomainCommit(context.Context, DomainCommitReconcileRequest) (DomainCommitReconcileResult, error) {
	return DomainCommitReconcileResult{Found: false}, nil
}

func (e *failOnceQueuedMaterializationEngine) Run(_ context.Context, request EngineRequest, _ EngineEventSink) (EngineResult, error) {
	e.runCalls.Add(1)
	e.ran <- request
	return EngineResult{Status: EngineCompleted}, nil
}

func TestRecoverAcceptedInputUsesDurableQueueWithoutCallerPayload(t *testing.T) {
	store := NewMemoryJournalStore()
	binding := testBindingAt("/book", "server-recovery")
	ref, err := BindingReference(binding)
	if err != nil {
		t.Fatal(err)
	}
	operationID := OperationID("operation-server-recovery")
	queued := Steer{
		ID: "queued-steer", OperationID: operationID,
		Input: UserInput{Text: "private accepted input", TurnSpecRef: "private-turn-ref"},
	}
	seedRuntimeEvents(t, store, ref, []EventPayload{
		CommandAcceptedEvent{CommandID: "start", CommandKind: "start_turn", OperationID: operationID, Fingerprint: "start"},
		OperationStartedEvent{OperationID: operationID},
		UserMessageCommittedEvent{Message: Message{ID: "user", Role: RoleUser, Content: "original", Input: UserInput{Text: "original"}, Operation: operationID}},
		CycleStartedEvent{OperationID: operationID, Cycle: 1, SnapshotID: "snapshot-original"},
		CommandAcceptedEvent{CommandID: queued.ID, CommandKind: string(DeliverySteer), OperationID: operationID, Fingerprint: fingerprintCommand(queued)},
		QueueEnqueuedEvent{Item: QueuedInput{CommandID: queued.ID, OperationID: operationID, Delivery: DeliverySteer, Input: queued.Input}},
	})
	engine := &recoveredQueueEngine{requests: make(chan EngineRequest, 1)}
	runtime, err := NewRuntime(EngineFactoryFunc(func(context.Context, BindingRef) (Engine, error) { return engine, nil }), store, RuntimeConfig{})
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close(context.Background())
	harness, err := runtime.Open(context.Background(), binding)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := harness.RecoverAcceptedInput(context.Background(), RecoveryAction{
		Kind: DeliverySteer, CommandID: queued.ID, OperationID: "wrong-operation",
	}); !errors.Is(err, ErrRecoveryActionChanged) {
		t.Fatalf("mismatched safe identity error = %v", err)
	}
	receipt, err := harness.RecoverAcceptedInput(context.Background(), RecoveryAction{
		Kind: DeliverySteer, CommandID: queued.ID, OperationID: operationID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !receipt.Replayed || receipt.CommandID != queued.ID || receipt.OperationID != operationID {
		t.Fatalf("recovery receipt = %#v", receipt)
	}
	select {
	case request := <-engine.requests:
		if request.Snapshot.Input.Text != queued.Input.Text || request.Snapshot.Input.TurnSpecRef != queued.Input.TurnSpecRef {
			t.Fatalf("engine did not receive exact durable input: %#v", request.Snapshot.Input)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("server-side recovery did not start the accepted input")
	}
}

func TestNewTransientCommandImmediatelyResumesRecoveryPausedOperation(t *testing.T) {
	store := NewMemoryJournalStore()
	binding := testBindingAt("/book", "new-transient-recovery")
	ref := binding
	operationID := OperationID("operation-new-transient")
	existing := FollowUp{ID: "existing-follow-up", OperationID: operationID, Input: UserInput{Text: "existing", TurnSpecRef: "existing-ref"}}
	seedRuntimeEvents(t, store, ref, []EventPayload{
		CommandAcceptedEvent{CommandID: "start", CommandKind: "start_turn", OperationID: operationID, Fingerprint: "start"},
		OperationStartedEvent{OperationID: operationID},
		UserMessageCommittedEvent{Message: Message{ID: "user", Role: RoleUser, Content: "original", Input: UserInput{Text: "original"}, Operation: operationID}},
		CycleStartedEvent{OperationID: operationID, Cycle: 1, SnapshotID: "snapshot-original"},
		CommandAcceptedEvent{CommandID: existing.ID, CommandKind: string(DeliveryFollowUp), OperationID: operationID, Fingerprint: fingerprintCommand(existing)},
		QueueEnqueuedEvent{Item: QueuedInput{CommandID: existing.ID, OperationID: operationID, Delivery: DeliveryFollowUp, Input: existing.Input}},
	})
	engine := newStagedRecoveryEngine("new-steer")
	runtime, err := NewRuntime(EngineFactoryFunc(func(context.Context, BindingRef) (Engine, error) { return engine, nil }), store, RuntimeConfig{})
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close(context.Background())
	harness, err := runtime.Open(context.Background(), binding)
	if err != nil {
		t.Fatal(err)
	}
	steer := Steer{ID: "new-steer", OperationID: operationID, Input: UserInput{Text: "take a new direction", TurnSpecRef: "new-ref"}}
	receipt, err := harness.Submit(context.Background(), steer)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Replayed {
		t.Fatalf("new command reported replay: %#v", receipt)
	}
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		requests := engine.requestsSnapshot()
		if len(requests) > 0 {
			if requests[0].Snapshot.CommandID != steer.ID || requests[0].Snapshot.Cycle != 2 {
				t.Fatalf("first resumed request = %#v", requests[0].Snapshot)
			}
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("new transient command required a second replay")
}

func TestNewNextTurnAbandonsRecoveryPausedParentAndStartsSuccessor(t *testing.T) {
	store := NewMemoryJournalStore()
	binding := testBindingAt("/book", "new-next-recovery")
	ref := binding
	parentID := OperationID("operation-parent")
	existing := Steer{ID: "existing-steer", OperationID: parentID, Input: UserInput{Text: "existing", TurnSpecRef: "existing-ref"}}
	seedRuntimeEvents(t, store, ref, []EventPayload{
		CommandAcceptedEvent{CommandID: "start", CommandKind: "start_turn", OperationID: parentID, Fingerprint: "start"},
		OperationStartedEvent{OperationID: parentID},
		UserMessageCommittedEvent{Message: Message{ID: "user", Role: RoleUser, Content: "original", Input: UserInput{Text: "original"}, Operation: parentID}},
		CycleStartedEvent{OperationID: parentID, Cycle: 1, SnapshotID: "snapshot-original"},
		CommandAcceptedEvent{CommandID: existing.ID, CommandKind: string(DeliverySteer), OperationID: parentID, Fingerprint: fingerprintCommand(existing)},
		QueueEnqueuedEvent{Item: QueuedInput{CommandID: existing.ID, OperationID: parentID, Delivery: DeliverySteer, Input: existing.Input}},
	})
	engine := &recoveredQueueEngine{requests: make(chan EngineRequest, 1)}
	runtime, err := NewRuntime(EngineFactoryFunc(func(context.Context, BindingRef) (Engine, error) { return engine, nil }), store, RuntimeConfig{})
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close(context.Background())
	harness, err := runtime.Open(context.Background(), binding)
	if err != nil {
		t.Fatal(err)
	}
	next := NextTurn{ID: "new-next", AfterOperationID: parentID, Input: UserInput{Text: "start the successor", TurnSpecRef: "next-ref"}}
	receipt, err := harness.Submit(context.Background(), next)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case request := <-engine.requests:
		if request.Snapshot.OperationID != receipt.OperationID || request.Snapshot.OperationID == parentID || request.Snapshot.CommandID != next.ID {
			t.Fatalf("successor request = %#v receipt=%#v", request.Snapshot, receipt)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("accepted NextTurn did not start its successor")
	}
	status, err := harness.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	foundParent := false
	for _, summary := range status.RecentOperations {
		if summary.OperationID == parentID {
			foundParent = summary.Status == OperationInterrupted
		}
	}
	if !foundParent {
		t.Fatalf("uncertain parent was not durably interrupted: %#v", status.RecentOperations)
	}
}

func TestReopenedRunningStartWaitsForFreshRecoveryDecision(t *testing.T) {
	tests := []struct {
		name       string
		command    func(OperationID) Command
		wantRun    bool
		wantKind   DeliveryKind
		wantStatus OperationStatus
	}{
		{
			name: "steer",
			command: func(operationID OperationID) Command {
				return Steer{ID: "fresh-steer", OperationID: operationID, Input: UserInput{Text: "change direction", TurnSpecRef: "fresh-steer-ref"}}
			},
			wantRun: true, wantKind: DeliverySteer,
		},
		{
			name: "follow_up",
			command: func(operationID OperationID) Command {
				return FollowUp{ID: "fresh-follow-up", OperationID: operationID, Input: UserInput{Text: "continue safely", TurnSpecRef: "fresh-follow-up-ref"}}
			},
			wantRun: true, wantKind: DeliveryFollowUp,
		},
		{
			name: "abort",
			command: func(operationID OperationID) Command {
				return Abort{ID: "fresh-abort", OperationID: operationID, Reason: "user abandoned recovered start"}
			},
			wantStatus: OperationAborted,
		},
		{
			name: "next_turn",
			command: func(operationID OperationID) Command {
				return NextTurn{ID: "fresh-next", AfterOperationID: operationID, Input: UserInput{Text: "start successor", TurnSpecRef: "fresh-next-ref"}}
			},
			wantRun: true, wantKind: DeliveryNextTurn, wantStatus: OperationInterrupted,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := NewMemoryJournalStore()
			binding := testBindingAt("/book", "fresh-recovery-"+test.name)
			ref, err := BindingReference(binding)
			if err != nil {
				t.Fatal(err)
			}
			operationID := OperationID("operation-fresh-" + test.name)
			seedRuntimeEvents(t, store, ref, []EventPayload{
				CommandAcceptedEvent{CommandID: "crashed-start", CommandKind: "start_turn", OperationID: operationID, Fingerprint: "start"},
				OperationStartedEvent{OperationID: operationID},
				UserMessageCommittedEvent{Message: Message{ID: "user", Role: RoleUser, Content: "original", Input: UserInput{Text: "original"}, Operation: operationID}},
				CycleStartedEvent{OperationID: operationID, Cycle: 1, SnapshotID: "snapshot-original"},
			})
			engine := &recoveredQueueEngine{requests: make(chan EngineRequest, 1)}
			runtime, err := NewRuntime(EngineFactoryFunc(func(context.Context, BindingRef) (Engine, error) { return engine, nil }), store, RuntimeConfig{})
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
			if status.Phase != PhaseRunning || !status.RecoveryPaused || status.ActiveOperation != operationID || engine.runCalls.Load() != 0 {
				t.Fatalf("reopened status = %#v engine_runs=%d", status, engine.runCalls.Load())
			}
			observeCtx, stopObserving := context.WithCancel(context.Background())
			defer stopObserving()
			observation, err := harness.Observe(observeCtx, status.Cursor)
			if err != nil {
				t.Fatal(err)
			}

			receipt, err := harness.Submit(context.Background(), test.command(operationID))
			if err != nil {
				t.Fatal(err)
			}
			if receipt.Replayed {
				t.Fatalf("fresh recovery decision reported replay: %#v", receipt)
			}
			if test.wantRun {
				if test.wantKind == DeliverySteer || test.wantKind == DeliveryFollowUp {
					seenSavePoint := false
					for !seenSavePoint {
						select {
						case event := <-observation.Events:
							switch payload := event.Payload.(type) {
							case SavePointCommittedEvent:
								seenSavePoint = payload.OperationID == operationID && payload.Cycle == 1
							case QueueConsumedEvent:
								if payload.CommandID == receipt.CommandID {
									t.Fatal("fresh transient was consumed before the recovered parent savepoint")
								}
							}
						case <-time.After(500 * time.Millisecond):
							t.Fatal("fresh transient did not commit the recovered parent savepoint")
						}
					}
				}
				select {
				case request := <-engine.requests:
					if request.Snapshot.CommandID != receipt.CommandID {
						t.Fatalf("engine command = %q, want %q", request.Snapshot.CommandID, receipt.CommandID)
					}
					if test.wantKind == DeliveryNextTurn {
						if request.Snapshot.OperationID != receipt.OperationID || request.Snapshot.OperationID == operationID || request.Snapshot.Cycle != 1 {
							t.Fatalf("successor snapshot = %#v receipt=%#v", request.Snapshot, receipt)
						}
					} else if request.Snapshot.OperationID != operationID || request.Snapshot.Cycle != 2 {
						t.Fatalf("resumed transient snapshot = %#v", request.Snapshot)
					}
				case <-time.After(500 * time.Millisecond):
					t.Fatalf("fresh %s did not start its server-derived cycle", test.name)
				}
			}
			if test.wantStatus != "" {
				status, err = harness.Status(context.Background())
				if err != nil {
					t.Fatal(err)
				}
				found := false
				for _, summary := range status.RecentOperations {
					if summary.OperationID == operationID && summary.Status == test.wantStatus {
						found = true
						break
					}
				}
				if !found {
					t.Fatalf("parent status %q not recorded: %#v", test.wantStatus, status.RecentOperations)
				}
			}
		})
	}
}

func TestStatusSnapshotPreservesProjectionTruncationMetadata(t *testing.T) {
	state := newHarnessState(testBinding("projection-truncation"))
	state.phase = PhaseRunning
	state.activeOperation = "operation-1"
	state.activeCycle = 1
	state.activeContent.WriteString("界界界")
	state.activeThinking.WriteString("thinking")
	state.queue = []QueuedInput{{
		CommandID: "follow", OperationID: "operation-1", Delivery: DeliveryFollowUp,
		Input: UserInput{Text: "queued-message"},
	}}
	state.lastOperation = &OperationSummary{
		CommandID: "old", OperationID: "old-operation", Status: OperationInterrupted,
		Reason: "reason-that-is-longer-than-the-retained-operation-summary-limit" + string(make([]byte, 16<<10)),
	}

	snapshot := state.statusSnapshot(4)
	if !snapshot.ActiveOutput.ContentTruncated || !snapshot.ActiveOutput.ThinkingTruncated ||
		len(snapshot.Queue) != 1 || !snapshot.Queue[0].InputTextTruncated ||
		snapshot.LastOperation == nil || !snapshot.LastOperation.ReasonTruncated {
		t.Fatalf("truncation metadata = %#v", snapshot)
	}
}
