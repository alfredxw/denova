package agentruntime_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"denova/internal/agentruntime"
)

func TestRecoveryMaterializesAcceptedInputWithoutRunningEngine(t *testing.T) {
	t.Parallel()

	store := agentruntime.NewMemoryJournalStore()
	binding := agentruntime.WritingBinding{Workspace: "/book", SessionID: "materialize-accepted"}
	seedAcceptedInput(t, store, binding, false, false)
	engine := newInputMaterializationEngine()

	runtime, err := agentruntime.NewRuntime(engine, store, agentruntime.RuntimeConfig{})
	if err != nil {
		t.Fatal(err)
	}
	harness, err := runtime.Open(context.Background(), binding)
	if err != nil {
		t.Fatalf("open recovered harness: %v", err)
	}
	status, err := harness.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if status.Phase != agentruntime.PhaseRunning || !status.RecoveryPaused || status.LastOperation != nil {
		t.Fatalf("recovered status = %#v, want recovery-paused Running", status)
	}
	commit := domainCommitForStage(t, status.DomainCommits, agentruntime.DomainCommitInput)
	if commit.Identity != acceptedInputIdentity() || commit.Hash != acceptedInputHash || commit.Revision != acceptedInputRevision {
		t.Fatalf("materialized input commit = %#v", commit)
	}
	if got := engine.materializeCalls.Load(); got != 1 {
		t.Fatalf("materialize calls = %d, want 1", got)
	}
	if got := engine.runCalls.Load(); got != 0 {
		t.Fatalf("engine calls during recovery = %d, want 0", got)
	}
	if err := runtime.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestRecoveryReconcilesInputWrittenBeforeReceiptWithoutReplayingWrite(t *testing.T) {
	t.Parallel()

	store := agentruntime.NewMemoryJournalStore()
	binding := agentruntime.WritingBinding{Workspace: "/book", SessionID: "materialize-written"}
	seedAcceptedInput(t, store, binding, true, false)
	engine := newInputMaterializationEngine()
	engine.canonical.Store(true)

	runtime, err := agentruntime.NewRuntime(engine, store, agentruntime.RuntimeConfig{})
	if err != nil {
		t.Fatal(err)
	}
	harness, err := runtime.Open(context.Background(), binding)
	if err != nil {
		t.Fatalf("open recovered harness: %v", err)
	}
	status, err := harness.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got := domainCommitForStage(t, status.DomainCommits, agentruntime.DomainCommitInput).Revision; got != acceptedInputRevision {
		t.Fatalf("reconciled input revision = %q, want %q", got, acceptedInputRevision)
	}
	if got := engine.materializeCalls.Load(); got != 0 {
		t.Fatalf("canonical input was replayed %d times", got)
	}
	if got := engine.runCalls.Load(); got != 0 {
		t.Fatalf("engine calls during recovery = %d, want 0", got)
	}
	if err := runtime.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestRecoveryMaterializationErrorLeavesAcceptedInputRetryable(t *testing.T) {
	t.Parallel()

	store := agentruntime.NewMemoryJournalStore()
	binding := agentruntime.WritingBinding{Workspace: "/book", SessionID: "materialize-retry"}
	seedAcceptedInput(t, store, binding, false, false)
	materializeErr := errors.New("canonical input store unavailable")
	failing := newInputMaterializationEngine()
	failing.materializeErr = materializeErr

	failedRuntime, err := agentruntime.NewRuntime(failing, store, agentruntime.RuntimeConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := failedRuntime.Open(context.Background(), binding); !errors.Is(err, materializeErr) {
		t.Fatalf("open error = %v, want %v", err, materializeErr)
	}
	if got := failing.runCalls.Load(); got != 0 {
		t.Fatalf("engine calls after materialization error = %d, want 0", got)
	}

	retry := newInputMaterializationEngine()
	retryRuntime, err := agentruntime.NewRuntime(retry, store, agentruntime.RuntimeConfig{})
	if err != nil {
		t.Fatal(err)
	}
	harness, err := retryRuntime.Open(context.Background(), binding)
	if err != nil {
		t.Fatalf("retry open: %v", err)
	}
	status, err := harness.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if status.Phase != agentruntime.PhaseRunning || !status.RecoveryPaused || status.LastOperation != nil {
		t.Fatalf("retry status = %#v, want recovery-paused Running", status)
	}
	if got := domainCommitForStage(t, status.DomainCommits, agentruntime.DomainCommitInput).Revision; got != acceptedInputRevision {
		t.Fatalf("retry input revision = %q", got)
	}
	if got := retry.runCalls.Load(); got != 0 {
		t.Fatalf("engine calls during retry recovery = %d, want 0", got)
	}
	if err := retryRuntime.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestRecoveryWithInputReceiptSettlesWithoutEngineOrCanonicalCalls(t *testing.T) {
	t.Parallel()

	store := agentruntime.NewMemoryJournalStore()
	binding := agentruntime.WritingBinding{Workspace: "/book", SessionID: "materialize-receipt"}
	seedAcceptedInput(t, store, binding, true, true)
	engine := newInputMaterializationEngine()

	runtime, err := agentruntime.NewRuntime(engine, store, agentruntime.RuntimeConfig{})
	if err != nil {
		t.Fatal(err)
	}
	harness, err := runtime.Open(context.Background(), binding)
	if err != nil {
		t.Fatalf("open recovered harness: %v", err)
	}
	status, err := harness.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if status.Phase != agentruntime.PhaseRunning || !status.RecoveryPaused || status.LastOperation != nil {
		t.Fatalf("recovered status = %#v, want recovery-paused Running", status)
	}
	if got := engine.effectCalls(); got != 0 {
		t.Fatalf("provider/canonical calls with durable receipt = %d, want 0", got)
	}
	if err := runtime.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestStartTurnMaterializesAcceptedInputBeforeEngine(t *testing.T) {
	t.Parallel()

	engine := &orderedInputMaterializationEngine{
		started: make(chan agentruntime.EngineRequest, 1),
		release: make(chan struct{}),
	}
	runtime, err := agentruntime.NewRuntime(engine, agentruntime.NewMemoryJournalStore(), agentruntime.RuntimeConfig{})
	if err != nil {
		t.Fatal(err)
	}
	harness, err := runtime.Open(context.Background(), agentruntime.WritingBinding{
		Workspace: "/book", SessionID: "materialize-before-engine",
	})
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := harness.Submit(context.Background(), agentruntime.StartTurn{
		ID: "command",
		Input: agentruntime.UserInput{
			Text:              "write",
			RestoreDescriptor: json.RawMessage(`{"version":1,"kind":"writing"}`),
		},
	})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	select {
	case request := <-engine.started:
		if request.Snapshot.CommandID != receipt.CommandID || request.Snapshot.OperationID != receipt.OperationID {
			t.Fatalf("engine request = %#v, receipt = %#v", request, receipt)
		}
		if !engine.materialized.Load() {
			t.Fatal("Engine.Run crossed the accepted-input boundary before canonical materialization")
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("Engine.Run was not started after canonical input receipt")
	}
	status, err := harness.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	commit := domainCommitForStage(t, status.DomainCommits, agentruntime.DomainCommitInput)
	if commit.Hash != acceptedInputHash || commit.Revision != acceptedInputRevision {
		t.Fatalf("input receipt before Engine.Run = %#v", commit)
	}
	close(engine.release)
	waitForSettled(t, harness, receipt.Cursor)
	if err := runtime.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestExactSubmitReplayRetriesPendingInputBeforeStartingEngine(t *testing.T) {
	t.Parallel()

	engine := newInputMaterializationEngine()
	engine.materializeErr = errors.New("input store unavailable")
	runtime, err := agentruntime.NewRuntime(engine, agentruntime.NewMemoryJournalStore(), agentruntime.RuntimeConfig{})
	if err != nil {
		t.Fatal(err)
	}
	harness, err := runtime.Open(context.Background(), agentruntime.WritingBinding{
		Workspace: "/book", SessionID: "materialize-submit-retry",
	})
	if err != nil {
		t.Fatal(err)
	}
	command := agentruntime.StartTurn{
		ID: "command",
		Input: agentruntime.UserInput{
			Text:              "write",
			RestoreDescriptor: json.RawMessage(`{"version":1,"kind":"writing"}`),
		},
	}
	first, err := harness.Submit(context.Background(), command)
	if !errors.Is(err, engine.materializeErr) {
		t.Fatalf("first submit error = %v, want %v", err, engine.materializeErr)
	}
	if first.CommandID != command.ID || first.OperationID == "" || first.Cursor == 0 {
		t.Fatalf("accepted receipt lost on materialization error: %#v", first)
	}
	if got := engine.runCalls.Load(); got != 0 {
		t.Fatalf("engine calls before input receipt = %d, want 0", got)
	}

	engine.materializeErr = nil
	replayed, err := harness.Submit(context.Background(), command)
	if err != nil {
		t.Fatalf("exact replay: %v", err)
	}
	if !replayed.Replayed || replayed.CommandID != first.CommandID || replayed.OperationID != first.OperationID || replayed.Cursor != first.Cursor {
		t.Fatalf("replayed receipt = %#v, first = %#v", replayed, first)
	}
	waitForSettled(t, harness, first.Cursor)
	if got := engine.materializeCalls.Load(); got != 2 {
		t.Fatalf("materialization attempts = %d, want failed attempt plus one retry", got)
	}
	if got := engine.runCalls.Load(); got != 1 {
		t.Fatalf("engine calls after exact replay = %d, want 1", got)
	}
	if err := runtime.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestPendingInputRejectsSameIdentityWithDifferentSemanticHash(t *testing.T) {
	t.Parallel()

	engine := newInputMaterializationEngine()
	engine.materializeErr = errors.New("input store unavailable")
	runtime, err := agentruntime.NewRuntime(engine, agentruntime.NewMemoryJournalStore(), agentruntime.RuntimeConfig{})
	if err != nil {
		t.Fatal(err)
	}
	harness, err := runtime.Open(context.Background(), agentruntime.WritingBinding{
		Workspace: "/book", SessionID: "materialize-hash-conflict",
	})
	if err != nil {
		t.Fatal(err)
	}
	command := agentruntime.StartTurn{
		ID: "command",
		Input: agentruntime.UserInput{
			Text:              "write",
			RestoreDescriptor: json.RawMessage(`{"version":1,"kind":"writing"}`),
		},
	}
	if _, err := harness.Submit(context.Background(), command); !errors.Is(err, engine.materializeErr) {
		t.Fatalf("first submit error = %v", err)
	}
	engine.mu.Lock()
	engine.hash = "sha256:different-input"
	engine.materializeErr = nil
	engine.mu.Unlock()
	if _, err := harness.Submit(context.Background(), command); !errors.Is(err, agentruntime.ErrDomainCommitRejected) {
		t.Fatalf("hash-conflicting replay error = %v, want ErrDomainCommitRejected", err)
	}
	if got := engine.runCalls.Load(); got != 0 {
		t.Fatalf("engine calls after semantic conflict = %d, want 0", got)
	}
	if err := runtime.Close(context.Background()); err == nil {
		t.Fatal("close should report the still-pending conflicting input")
	}
}

const (
	acceptedInputHash     = "sha256:accepted-input"
	acceptedInputRevision = "input:1"
)

func acceptedInputIdentity() agentruntime.DomainCommitIdentity {
	return agentruntime.DomainCommitIdentity{
		CommandID: "command", OperationID: "operation", Cycle: 1, Stage: agentruntime.DomainCommitInput,
	}
}

func seedAcceptedInput(
	t *testing.T,
	store agentruntime.JournalStore,
	binding agentruntime.Binding,
	withIntent bool,
	withReceipt bool,
) {
	t.Helper()
	ref, err := agentruntime.BindingReference(binding)
	if err != nil {
		t.Fatal(err)
	}
	key, err := json.Marshal(ref)
	if err != nil {
		t.Fatal(err)
	}
	journal, err := store.OpenJournal(context.Background(), string(key))
	if err != nil {
		t.Fatal(err)
	}
	identity := acceptedInputIdentity()
	payloads := []agentruntime.EventPayload{
		agentruntime.CommandAcceptedEvent{CommandID: identity.CommandID, CommandKind: "start_turn", OperationID: identity.OperationID, Fingerprint: "seed"},
		agentruntime.OperationStartedEvent{OperationID: identity.OperationID},
		agentruntime.UserMessageCommittedEvent{Message: agentruntime.Message{
			ID: "user", Role: agentruntime.RoleUser, Content: "write",
			Input:     agentruntime.UserInput{Text: "write", RestoreDescriptor: json.RawMessage(`{"version":1,"kind":"writing"}`)},
			Operation: identity.OperationID,
		}},
		agentruntime.CycleStartedEvent{OperationID: identity.OperationID, Cycle: identity.Cycle, SnapshotID: "snapshot"},
	}
	if withIntent {
		payloads = append(payloads, agentruntime.DomainCommitIntentAcceptedEvent{Identity: identity, Hash: acceptedInputHash})
	}
	if withReceipt {
		payloads = append(payloads, agentruntime.DomainCommitReceiptEvent{
			Identity: identity, Hash: acceptedInputHash, Revision: acceptedInputRevision,
		})
	}
	if _, err := journal.Append(context.Background(), 0, payloads); err != nil {
		_ = journal.Close()
		t.Fatal(err)
	}
	if err := journal.Close(); err != nil {
		t.Fatal(err)
	}
}

type inputMaterializationEngine struct {
	canonical        atomic.Bool
	planCalls        atomic.Int32
	reconcileCalls   atomic.Int32
	materializeCalls atomic.Int32
	runCalls         atomic.Int32
	materializeErr   error
	hash             string
	mu               sync.Mutex
}

func newInputMaterializationEngine() *inputMaterializationEngine {
	return &inputMaterializationEngine{}
}

func (e *inputMaterializationEngine) NewEngine(context.Context, agentruntime.BindingRef) (agentruntime.Engine, error) {
	return e, nil
}

func (e *inputMaterializationEngine) Run(context.Context, agentruntime.EngineRequest, agentruntime.EngineEventSink) (agentruntime.EngineResult, error) {
	e.runCalls.Add(1)
	return agentruntime.EngineResult{}, errors.New("recovery must not rerun the engine")
}

func (e *inputMaterializationEngine) PlanInputMaterialization(
	_ context.Context,
	request agentruntime.InputMaterializationRequest,
) (agentruntime.InputMaterializationPlan, error) {
	e.planCalls.Add(1)
	if request.Binding.Workspace != "/book" || request.Snapshot.CommandID != "command" || request.Snapshot.Input.Text != "write" {
		return agentruntime.InputMaterializationPlan{}, errors.New("runtime did not preserve accepted input identity")
	}
	e.mu.Lock()
	hash := e.hash
	e.mu.Unlock()
	if hash == "" {
		hash = acceptedInputHash
	}
	return agentruntime.InputMaterializationPlan{Required: true, Hash: hash}, nil
}

func (e *inputMaterializationEngine) MaterializeInput(
	_ context.Context,
	request agentruntime.InputMaterializationRequest,
	plan agentruntime.InputMaterializationPlan,
) (agentruntime.InputMaterializationReceipt, error) {
	e.materializeCalls.Add(1)
	e.mu.Lock()
	materializeErr := e.materializeErr
	e.mu.Unlock()
	if materializeErr != nil {
		return agentruntime.InputMaterializationReceipt{}, materializeErr
	}
	if request.Snapshot.CommandID != "command" || plan.Hash != acceptedInputHash {
		return agentruntime.InputMaterializationReceipt{}, errors.New("materialization request changed after acceptance")
	}
	e.canonical.Store(true)
	return agentruntime.InputMaterializationReceipt{Revision: acceptedInputRevision}, nil
}

func (e *inputMaterializationEngine) ReconcileDomainCommit(
	_ context.Context,
	request agentruntime.DomainCommitReconcileRequest,
) (agentruntime.DomainCommitReconcileResult, error) {
	e.reconcileCalls.Add(1)
	if request.Commit.Identity.CommandID != "command" || request.Commit.Identity.OperationID == "" ||
		request.Commit.Identity.Cycle != 1 || request.Commit.Identity.Stage != agentruntime.DomainCommitInput ||
		request.Commit.Hash != acceptedInputHash {
		return agentruntime.DomainCommitReconcileResult{}, errors.New("runtime reconciled the wrong accepted input")
	}
	if !e.canonical.Load() {
		return agentruntime.DomainCommitReconcileResult{Found: false}, nil
	}
	return agentruntime.DomainCommitReconcileResult{Found: true, Revision: acceptedInputRevision}, nil
}

func (e *inputMaterializationEngine) effectCalls() int32 {
	return e.reconcileCalls.Load() + e.materializeCalls.Load() + e.runCalls.Load()
}

type orderedInputMaterializationEngine struct {
	materialized atomic.Bool
	started      chan agentruntime.EngineRequest
	release      chan struct{}
}

func (e *orderedInputMaterializationEngine) NewEngine(context.Context, agentruntime.BindingRef) (agentruntime.Engine, error) {
	return e, nil
}

func (*orderedInputMaterializationEngine) PlanInputMaterialization(
	context.Context,
	agentruntime.InputMaterializationRequest,
) (agentruntime.InputMaterializationPlan, error) {
	return agentruntime.InputMaterializationPlan{Required: true, Hash: acceptedInputHash}, nil
}

func (e *orderedInputMaterializationEngine) MaterializeInput(
	context.Context,
	agentruntime.InputMaterializationRequest,
	agentruntime.InputMaterializationPlan,
) (agentruntime.InputMaterializationReceipt, error) {
	e.materialized.Store(true)
	return agentruntime.InputMaterializationReceipt{Revision: acceptedInputRevision}, nil
}

func (e *orderedInputMaterializationEngine) Run(
	_ context.Context,
	request agentruntime.EngineRequest,
	_ agentruntime.EngineEventSink,
) (agentruntime.EngineResult, error) {
	if !e.materialized.Load() {
		return agentruntime.EngineResult{}, errors.New("Engine.Run started before canonical input")
	}
	e.started <- request
	<-e.release
	return agentruntime.EngineResult{Status: agentruntime.EngineCompleted}, nil
}
