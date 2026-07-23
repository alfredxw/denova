package agentruntime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

type recordingJournalStore struct {
	inner   *MemoryJournalStore
	mu      sync.Mutex
	batches [][]EventPayload
}

func (s *recordingJournalStore) OpenJournal(ctx context.Context, key string) (Journal, error) {
	inner, err := s.inner.OpenJournal(ctx, key)
	if err != nil {
		return nil, err
	}
	return &recordingJournal{Journal: inner, store: s}, nil
}

type recordingJournal struct {
	Journal
	store *recordingJournalStore
}

func (j *recordingJournal) Append(ctx context.Context, expected Cursor, payloads []EventPayload) ([]Event, error) {
	events, err := j.Journal.Append(ctx, expected, payloads)
	if err == nil {
		j.store.mu.Lock()
		j.store.batches = append(j.store.batches, append([]EventPayload(nil), payloads...))
		j.store.mu.Unlock()
	}
	return events, err
}

type liveHostEffectEngine struct {
	mu              sync.Mutex
	reconciliations []HostEffectID
}

type outputCommitFailingHostEffectEngine struct {
	liveHostEffectEngine
}

func (e *outputCommitFailingHostEffectEngine) NewEngine(context.Context, BindingRef) (Engine, error) {
	return e, nil
}

func (e *outputCommitFailingHostEffectEngine) Run(_ context.Context, request EngineRequest, emit EngineEventSink) (EngineResult, error) {
	const callID = "tool-call-before-failed-output"
	if err := emit(EngineToolStarted{CallID: callID, Name: "mutate"}); err != nil {
		return EngineResult{}, err
	}
	effect, err := NewToolHostEffect(
		request.Binding, request.Snapshot.OperationID, request.Snapshot.Cycle,
		callID, 0, HostEffectToolMutationCommitted, json.RawMessage(`{"version":1,"mutation":"must-not-transfer"}`),
	)
	if err != nil {
		return EngineResult{}, err
	}
	if err := emit(EngineToolFinished{CallID: callID, Name: "mutate", HostEffects: []HostEffect{effect}}); err != nil {
		return EngineResult{}, err
	}
	identity := DomainCommitIdentity{
		CommandID: request.Snapshot.CommandID, OperationID: request.Snapshot.OperationID,
		Cycle: request.Snapshot.Cycle, Stage: DomainCommitOutput,
	}
	if err := emit(EngineDomainCommitIntent{Identity: identity, Hash: "sha256:failed-output"}); err != nil {
		return EngineResult{}, err
	}
	return EngineResult{}, errors.New("canonical output commit failed")
}

func TestHostEffectDoesNotTransferWhenExactOutputCommitFails(t *testing.T) {
	engine := &outputCommitFailingHostEffectEngine{}
	store := &recordingJournalStore{inner: NewMemoryJournalStore()}
	runtime, err := NewRuntime(engine, store, RuntimeConfig{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	harness, err := runtime.Open(context.Background(), WritingBinding{Workspace: "/book", SessionID: "host-effect-failed-output"})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	observation, err := harness.ObserveFromNow(ctx)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := harness.Submit(context.Background(), StartTurn{ID: "start", Input: UserInput{Text: "mutate"}})
	if err != nil {
		t.Fatal(err)
	}
	for {
		select {
		case event := <-observation.Events:
			settled, ok := event.Payload.(OperationSettledEvent)
			if !ok || settled.OperationID != receipt.OperationID {
				continue
			}
			if settled.Status != OperationFailed {
				t.Fatalf("failed output settlement = %#v", settled)
			}
			status, statusErr := harness.Status(context.Background())
			if statusErr != nil {
				t.Fatal(statusErr)
			}
			if len(status.PendingHostEffects) != 0 {
				t.Fatalf("failed output left host effects hanging: %#v", status.PendingHostEffects)
			}
			engine.mu.Lock()
			reconciliations := append([]HostEffectID(nil), engine.reconciliations...)
			engine.mu.Unlock()
			if len(reconciliations) != 0 {
				t.Fatalf("failed output transferred host effects: %v", reconciliations)
			}
			store.mu.Lock()
			batches := append([][]EventPayload(nil), store.batches...)
			store.mu.Unlock()
			var abandoned *HostEffectAbandonedEvent
			for _, batch := range batches {
				for _, payload := range batch {
					if event, ok := payload.(HostEffectAbandonedEvent); ok {
						cloned := event
						abandoned = &cloned
					}
				}
			}
			if abandoned == nil || abandoned.ID == "" || abandoned.Reason == "" {
				t.Fatalf("failed output has no durable HostEffect abandonment: %#v", abandoned)
			}
			return
		case <-ctx.Done():
			t.Fatalf("timed out waiting for failed output settlement: %v", ctx.Err())
		}
	}
}

func (e *liveHostEffectEngine) NewEngine(context.Context, BindingRef) (Engine, error) { return e, nil }

func (e *liveHostEffectEngine) Run(_ context.Context, request EngineRequest, emit EngineEventSink) (EngineResult, error) {
	const callID = "tool-call"
	if err := emit(EngineToolStarted{CallID: callID, Name: "mutate"}); err != nil {
		return EngineResult{}, err
	}
	effect, err := NewToolHostEffect(
		request.Binding, request.Snapshot.OperationID, request.Snapshot.Cycle,
		callID, 0, HostEffectToolMutationCommitted, json.RawMessage(`{"version":1,"mutation":"applied"}`),
	)
	if err != nil {
		return EngineResult{}, err
	}
	if err := emit(EngineToolFinished{CallID: callID, Name: "mutate", HostEffects: []HostEffect{effect}}); err != nil {
		return EngineResult{}, err
	}
	identity := DomainCommitIdentity{
		CommandID: request.Snapshot.CommandID, OperationID: request.Snapshot.OperationID,
		Cycle: request.Snapshot.Cycle, Stage: DomainCommitOutput,
	}
	if err := emit(EngineDomainCommitIntent{Identity: identity, Hash: "sha256:committed-output"}); err != nil {
		return EngineResult{}, err
	}
	if err := emit(EngineDomainCommitReceipt{Identity: identity, Hash: "sha256:committed-output", Revision: "session:1"}); err != nil {
		return EngineResult{}, err
	}
	return EngineResult{Status: EngineCompleted}, nil
}

func (e *liveHostEffectEngine) ReconcileHostEffect(_ context.Context, effect HostEffect) error {
	e.mu.Lock()
	e.reconciliations = append(e.reconciliations, effect.ID)
	e.mu.Unlock()
	return nil
}

func TestToolFinishAndHostEffectShareOneDurableTransaction(t *testing.T) {
	store := &recordingJournalStore{inner: NewMemoryJournalStore()}
	engine := &liveHostEffectEngine{}
	runtime, err := NewRuntime(engine, store, RuntimeConfig{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	harness, err := runtime.Open(context.Background(), WritingBinding{Workspace: "/book", SessionID: "host-effect-transaction"})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	observation, err := harness.ObserveFromNow(ctx)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := harness.Submit(context.Background(), StartTurn{ID: "start", Input: UserInput{Text: "mutate"}})
	if err != nil {
		t.Fatal(err)
	}
	var sawDisplayFinish bool
	for {
		select {
		case event := <-observation.Events:
			switch payload := event.Payload.(type) {
			case ToolCallFinishedEvent:
				if len(payload.HostEffects) != 1 || len(payload.HostEffects[0].Payload) != 0 {
					t.Fatalf("display tool finish leaked host payload: %#v", payload)
				}
				sawDisplayFinish = true
			case OperationSettledEvent:
				if payload.OperationID == receipt.OperationID {
					if payload.Status != OperationSucceeded || !sawDisplayFinish {
						t.Fatalf("host-effect settlement = %#v display_finish=%v", payload, sawDisplayFinish)
					}
					goto settled
				}
			}
		case <-ctx.Done():
			t.Fatalf("timed out waiting for host-effect settlement: %v", ctx.Err())
		}
	}

settled:

	store.mu.Lock()
	batches := append([][]EventPayload(nil), store.batches...)
	store.mu.Unlock()
	var finished *ToolCallFinishedEvent
	var acknowledged bool
	for _, batch := range batches {
		for _, payload := range batch {
			switch payload := payload.(type) {
			case ToolCallFinishedEvent:
				cloned := payload
				finished = &cloned
			case HostEffectAcknowledgedEvent:
				acknowledged = true
			}
		}
	}
	if finished == nil || len(finished.HostEffects) != 1 || len(finished.HostEffects[0].Payload) == 0 {
		t.Fatalf("durable tool finish did not carry its host effect: %#v", finished)
	}
	if !acknowledged {
		t.Fatal("successful host reconciliation was not durably acknowledged")
	}
	engine.mu.Lock()
	reconciliations := append([]HostEffectID(nil), engine.reconciliations...)
	engine.mu.Unlock()
	if len(reconciliations) != 1 || reconciliations[0] != finished.HostEffects[0].ID {
		t.Fatalf("host reconciliations = %v, effect = %q", reconciliations, finished.HostEffects[0].ID)
	}
}

type retryingHostEffectEngine struct {
	mu       sync.Mutex
	attempts int
	applied  map[HostEffectID]int
}

func (e *retryingHostEffectEngine) NewEngine(context.Context, BindingRef) (Engine, error) {
	return e, nil
}

func (*retryingHostEffectEngine) Run(context.Context, EngineRequest, EngineEventSink) (EngineResult, error) {
	return EngineResult{}, fmt.Errorf("recovered engine must not run implicitly")
}

func (e *retryingHostEffectEngine) ReconcileHostEffect(_ context.Context, effect HostEffect) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.attempts++
	if _, exists := e.applied[effect.ID]; !exists {
		e.applied[effect.ID] = 1
	}
	if e.attempts == 1 {
		return errors.New("ack result lost after host commit")
	}
	return nil
}

func TestColdOpenRetriesUnackedHostEffectIdempotentlyBeforeOperationRecovery(t *testing.T) {
	store := NewMemoryJournalStore()
	ref := BindingRef{Kind: BindingWriting, Profile: ProfileWriting, Workspace: "/book", SessionID: "host-effect-cold-retry"}
	encodedRef, err := json.Marshal(ref)
	if err != nil {
		t.Fatal(err)
	}
	journal, err := store.OpenJournal(context.Background(), string(encodedRef))
	if err != nil {
		t.Fatal(err)
	}
	effect, err := NewToolHostEffect(ref, "operation", 1, "call", 0, HostEffectToolMutationCommitted, json.RawMessage(`{"version":1,"mutation":"once"}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := journal.Append(context.Background(), 0, []EventPayload{
		CommandAcceptedEvent{CommandID: "start", CommandKind: "start_turn", OperationID: "operation", Fingerprint: "fingerprint"},
		OperationStartedEvent{OperationID: "operation"},
		UserMessageCommittedEvent{Message: Message{ID: "user", Role: RoleUser, Content: "mutate", Input: UserInput{Text: "mutate"}, Operation: "operation"}},
		CycleStartedEvent{OperationID: "operation", Cycle: 1, SnapshotID: "snapshot"},
		ToolCallStartedEvent{Call: ToolCallState{CallID: "call", Name: "mutate", OperationID: "operation", Cycle: 1}},
		ToolCallFinishedEvent{CallID: "call", Name: "mutate", HostEffects: []HostEffect{effect}},
		DomainCommitIntentAcceptedEvent{
			Identity: DomainCommitIdentity{CommandID: "start", OperationID: "operation", Cycle: 1, Stage: DomainCommitOutput},
			Hash:     "sha256:committed-output",
		},
		DomainCommitReceiptEvent{
			Identity: DomainCommitIdentity{CommandID: "start", OperationID: "operation", Cycle: 1, Stage: DomainCommitOutput},
			Hash:     "sha256:committed-output", Revision: "session:1",
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := journal.Close(); err != nil {
		t.Fatal(err)
	}

	engine := &retryingHostEffectEngine{applied: make(map[HostEffectID]int)}
	runtime, err := NewRuntime(engine, store, RuntimeConfig{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	binding := WritingBinding{Workspace: "/book", SessionID: "host-effect-cold-retry"}
	if _, err := runtime.Open(context.Background(), binding); !errors.Is(err, ErrHostEffectRequired) {
		t.Fatalf("first cold open error = %v, want host-effect reconciliation retry", err)
	}
	harness, err := runtime.Open(context.Background(), binding)
	if err != nil {
		t.Fatalf("second cold open: %v", err)
	}
	status, err := harness.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(status.PendingHostEffects) != 0 || status.Phase != PhaseIdle ||
		status.LastOperation == nil || status.LastOperation.Status != OperationSucceeded {
		t.Fatalf("host effect was not acked before exact output recovery settled: %#v", status)
	}
	engine.mu.Lock()
	attempts := engine.attempts
	applied := engine.applied[effect.ID]
	engine.mu.Unlock()
	if attempts != 2 || applied != 1 {
		t.Fatalf("reconcile attempts=%d host applications=%d, want retry with one application", attempts, applied)
	}
}

func TestPendingHostEffectFencesExactReplayFreshCommandAndInputRecovery(t *testing.T) {
	ref := BindingRef{Kind: BindingWriting, Profile: ProfileWriting, Workspace: "/book", SessionID: "host-effect-fence"}
	effect, err := NewToolHostEffect(ref, "operation", 1, "call", 0, HostEffectToolMutationCommitted, json.RawMessage(`{"version":1}`))
	if err != nil {
		t.Fatal(err)
	}
	state := newHarnessState(ref)
	state.pendingHostEffects[effect.ID] = effect
	state.pendingHostEffectOrder = append(state.pendingHostEffectOrder, effect.ID)
	exact := StartTurn{ID: "accepted", Input: UserInput{Text: "write"}}
	fingerprint, err := CommandFingerprint(exact)
	if err != nil {
		t.Fatal(err)
	}
	state.receipts[exact.ID] = Receipt{CommandID: exact.ID, OperationID: "operation", Cursor: 1}
	state.fingerprints[exact.ID] = fingerprint
	harness := &Harness{inputLimits: DefaultInputLimits()}

	for name, command := range map[string]Command{
		"exact replay":  exact,
		"fresh command": StartTurn{ID: "fresh", Input: UserInput{Text: "write"}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := harness.handleSubmit(&state, context.Background(), command); !errors.Is(err, ErrHostEffectRequired) {
				t.Fatalf("submit error = %v, want ErrHostEffectRequired", err)
			}
		})
	}
	if _, err := harness.handleRecoverAcceptedInput(&state, context.Background(), RecoveryAction{
		Kind: DeliveryFollowUp, CommandID: "accepted", OperationID: "operation",
	}); !errors.Is(err, ErrHostEffectRequired) {
		t.Fatalf("recovery error = %v, want ErrHostEffectRequired", err)
	}
}

type oversizedHostEffectEngine struct{}

func (e *oversizedHostEffectEngine) NewEngine(context.Context, BindingRef) (Engine, error) {
	return e, nil
}

func (e *oversizedHostEffectEngine) Run(_ context.Context, request EngineRequest, emit EngineEventSink) (EngineResult, error) {
	const callID = "oversized-effect"
	if err := emit(EngineToolStarted{CallID: callID, Name: "mutate"}); err != nil {
		return EngineResult{}, err
	}
	effect, err := NewToolHostEffect(
		request.Binding, request.Snapshot.OperationID, request.Snapshot.Cycle, callID, 0,
		HostEffectToolMutationCommitted, json.RawMessage(`{"payload":"larger-than-the-binding-budget"}`),
	)
	if err != nil {
		return EngineResult{}, err
	}
	// Deliberately ignore the sink error to prove the actor-owned boundary, not
	// adapter cooperation, prevents a false successful completion.
	_ = emit(EngineToolFinished{CallID: callID, Name: "mutate", HostEffects: []HostEffect{effect}})
	return EngineResult{Status: EngineCompleted}, nil
}

func TestOversizedHostEffectSettlesTypedIncomplete(t *testing.T) {
	runtime, err := NewRuntime(&oversizedHostEffectEngine{}, NewMemoryJournalStore(), RuntimeConfig{
		MemoryLimits: BindingMemoryLimits{MaxHostEffectBytes: 16},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	harness, err := runtime.Open(context.Background(), WritingBinding{Workspace: "/book", SessionID: "host-effect-budget"})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	observation, err := harness.ObserveFromNow(ctx)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := harness.Submit(context.Background(), StartTurn{ID: "start", Input: UserInput{Text: "mutate"}})
	if err != nil {
		t.Fatal(err)
	}
	var boundary *ByteBudgetExceededEvent
	for {
		select {
		case event := <-observation.Events:
			switch payload := event.Payload.(type) {
			case ByteBudgetExceededEvent:
				cloned := payload
				boundary = &cloned
			case OperationSettledEvent:
				if payload.OperationID != receipt.OperationID {
					continue
				}
				if payload.Status != OperationIncomplete || boundary == nil || boundary.Scope != ByteBudgetHostEffect {
					t.Fatalf("settlement=%#v boundary=%#v, want typed host-effect incompleteness", payload, boundary)
				}
				return
			}
		case <-ctx.Done():
			t.Fatalf("timed out waiting for incomplete host-effect settlement: %v", ctx.Err())
		}
	}
}
