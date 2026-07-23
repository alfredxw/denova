package agentruntime_test

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"sync/atomic"
	"testing"

	"denova/internal/agentruntime"
)

func TestRecoveryReconcilesCanonicalDomainCommitBeforeSettling(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name       string
		stage      agentruntime.DomainCommitStage
		wantStatus agentruntime.OperationStatus
	}{
		{name: "output settles successfully", stage: agentruntime.DomainCommitOutput, wantStatus: agentruntime.OperationSucceeded},
		{name: "input keeps receipt on interruption", stage: agentruntime.DomainCommitInput, wantStatus: agentruntime.OperationInterrupted},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			store := agentruntime.NewMemoryJournalStore()
			binding := agentruntime.WritingBinding{Workspace: "/book", SessionID: "reconcile-" + string(test.stage)}
			identity := seedPendingDomainCommit(t, store, binding, test.stage, "sha256:canonical")
			engine := &domainCommitReconcileEngine{reconcile: func(_ context.Context, request agentruntime.DomainCommitReconcileRequest) (agentruntime.DomainCommitReconcileResult, error) {
				if request.Binding.Workspace != "/book" || request.Binding.SessionID != binding.SessionID || request.Commit.Identity != identity || request.Commit.Hash != "sha256:canonical" {
					return agentruntime.DomainCommitReconcileResult{}, errors.New("runtime did not query the exact pending commit")
				}
				return agentruntime.DomainCommitReconcileResult{Found: true, Revision: "canonical:9"}, nil
			}}
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
			if test.stage == agentruntime.DomainCommitInput {
				if status.Phase != agentruntime.PhaseRunning || !status.RecoveryPaused || status.LastOperation != nil {
					t.Fatalf("recovered status = %#v, want recovery-paused Running", status)
				}
			} else if status.Phase != agentruntime.PhaseIdle || status.LastOperation == nil || status.LastOperation.Status != test.wantStatus {
				t.Fatalf("recovered status = %#v, want idle/%s", status, test.wantStatus)
			}
			commit := domainCommitForStage(t, status.DomainCommits, test.stage)
			if commit.Identity != identity || commit.Hash != "sha256:canonical" || commit.Revision != "canonical:9" {
				t.Fatalf("reconciled commit = %#v", commit)
			}
			if got := engine.calls.Load(); got != 1 {
				t.Fatalf("reconcile calls = %d, want 1", got)
			}
			if err := runtime.Close(context.Background()); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestRecoveryInterruptsWhenCanonicalCommitIsNotFound(t *testing.T) {
	t.Parallel()

	store := agentruntime.NewMemoryJournalStore()
	binding := agentruntime.WritingBinding{Workspace: "/book", SessionID: "reconcile-not-found"}
	seedPendingDomainCommit(t, store, binding, agentruntime.DomainCommitOutput, "sha256:missing")
	engine := &domainCommitReconcileEngine{reconcile: func(context.Context, agentruntime.DomainCommitReconcileRequest) (agentruntime.DomainCommitReconcileResult, error) {
		return agentruntime.DomainCommitReconcileResult{Found: false}, nil
	}}
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
	if got := domainCommitForStage(t, status.DomainCommits, agentruntime.DomainCommitOutput).Revision; got != "" {
		t.Fatalf("missing canonical commit revision = %q, want empty", got)
	}
	if got := engine.calls.Load(); got != 1 {
		t.Fatalf("reconcile calls = %d, want 1", got)
	}
	if err := runtime.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestRecoveryKeepsInputReceiptButPreservesAcceptedAbort(t *testing.T) {
	t.Parallel()

	store := agentruntime.NewMemoryJournalStore()
	binding := agentruntime.WritingBinding{Workspace: "/book", SessionID: "reconcile-input-abort"}
	seedPendingDomainCommit(t, store, binding, agentruntime.DomainCommitInput, "sha256:input")
	appendAbortToPendingDomainCommit(t, store, binding, "user stopped")
	engine := &domainCommitReconcileEngine{reconcile: func(context.Context, agentruntime.DomainCommitReconcileRequest) (agentruntime.DomainCommitReconcileResult, error) {
		return agentruntime.DomainCommitReconcileResult{Found: true, Revision: "canonical:input"}, nil
	}}
	runtime, err := agentruntime.NewRuntime(engine, store, agentruntime.RuntimeConfig{})
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
	if status.LastOperation == nil || status.LastOperation.Status != agentruntime.OperationAborted || status.LastOperation.Reason != "user stopped" {
		t.Fatalf("recovered abort = %#v", status.LastOperation)
	}
	if got := domainCommitForStage(t, status.DomainCommits, agentruntime.DomainCommitInput).Revision; got != "canonical:input" {
		t.Fatalf("input receipt revision = %q", got)
	}
	if err := runtime.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestRecoveryQueryErrorLeavesOperationRetryable(t *testing.T) {
	t.Parallel()

	store := agentruntime.NewMemoryJournalStore()
	binding := agentruntime.WritingBinding{Workspace: "/book", SessionID: "reconcile-error"}
	seedPendingDomainCommit(t, store, binding, agentruntime.DomainCommitOutput, "sha256:uncertain")
	queryErr := errors.New("canonical store unavailable")
	failedRuntime, err := agentruntime.NewRuntime(&domainCommitReconcileEngine{reconcile: func(context.Context, agentruntime.DomainCommitReconcileRequest) (agentruntime.DomainCommitReconcileResult, error) {
		return agentruntime.DomainCommitReconcileResult{}, queryErr
	}}, store, agentruntime.RuntimeConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := failedRuntime.Open(context.Background(), binding); !errors.Is(err, queryErr) {
		t.Fatalf("open error = %v, want %v", err, queryErr)
	}

	// A later open must still see the pending intent. If the failed query wrote
	// an interrupted terminal event, this exact canonical receipt cannot win.
	retryEngine := &domainCommitReconcileEngine{reconcile: func(context.Context, agentruntime.DomainCommitReconcileRequest) (agentruntime.DomainCommitReconcileResult, error) {
		return agentruntime.DomainCommitReconcileResult{Found: true, Revision: "canonical:retry"}, nil
	}}
	retryRuntime, err := agentruntime.NewRuntime(retryEngine, store, agentruntime.RuntimeConfig{})
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
	if status.LastOperation == nil || status.LastOperation.Status != agentruntime.OperationSucceeded {
		t.Fatalf("retry status = %#v, want succeeded", status)
	}
	if got := domainCommitForStage(t, status.DomainCommits, agentruntime.DomainCommitOutput).Revision; got != "canonical:retry" {
		t.Fatalf("retry revision = %q", got)
	}
	if err := retryRuntime.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestFileJournalPersistsReconciledReceiptAcrossReopen(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "journals")
	store, err := agentruntime.NewFileJournalStore(root)
	if err != nil {
		t.Fatal(err)
	}
	binding := agentruntime.WritingBinding{Workspace: "/book", SessionID: "file-reconcile"}
	seedPendingDomainCommit(t, store, binding, agentruntime.DomainCommitOutput, "sha256:file")
	firstEngine := &domainCommitReconcileEngine{reconcile: func(context.Context, agentruntime.DomainCommitReconcileRequest) (agentruntime.DomainCommitReconcileResult, error) {
		return agentruntime.DomainCommitReconcileResult{Found: true, Revision: "canonical:file"}, nil
	}}
	firstRuntime, err := agentruntime.NewRuntime(firstEngine, store, agentruntime.RuntimeConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := firstRuntime.Open(context.Background(), binding); err != nil {
		t.Fatalf("first open: %v", err)
	}
	if err := firstRuntime.Close(context.Background()); err != nil {
		t.Fatal(err)
	}

	reopenedStore, err := agentruntime.NewFileJournalStore(root)
	if err != nil {
		t.Fatal(err)
	}
	reopenedEngine := &domainCommitReconcileEngine{reconcile: func(context.Context, agentruntime.DomainCommitReconcileRequest) (agentruntime.DomainCommitReconcileResult, error) {
		return agentruntime.DomainCommitReconcileResult{}, errors.New("persisted receipt should not be queried again")
	}}
	reopenedRuntime, err := agentruntime.NewRuntime(reopenedEngine, reopenedStore, agentruntime.RuntimeConfig{})
	if err != nil {
		t.Fatal(err)
	}
	harness, err := reopenedRuntime.Open(context.Background(), binding)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	status, err := harness.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if status.LastOperation == nil || status.LastOperation.Status != agentruntime.OperationSucceeded {
		t.Fatalf("reopened status = %#v", status)
	}
	if got := domainCommitForStage(t, status.DomainCommits, agentruntime.DomainCommitOutput).Revision; got != "canonical:file" {
		t.Fatalf("persisted revision = %q", got)
	}
	if got := reopenedEngine.calls.Load(); got != 0 {
		t.Fatalf("reconcile calls after durable receipt = %d, want 0", got)
	}
	if err := reopenedRuntime.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestFileJournalPersistsAuthoritativeNotFoundAbandonmentAcrossAbortAndReopen(t *testing.T) {
	root := filepath.Join(t.TempDir(), "journals")
	store, err := agentruntime.NewFileJournalStore(root)
	if err != nil {
		t.Fatal(err)
	}
	binding := agentruntime.WritingBinding{Workspace: "/book", SessionID: "file-not-found-abandonment"}
	seedPendingDomainCommit(t, store, binding, agentruntime.DomainCommitOutput, "sha256:file-not-found")
	firstEngine := &domainCommitReconcileEngine{reconcile: func(context.Context, agentruntime.DomainCommitReconcileRequest) (agentruntime.DomainCommitReconcileResult, error) {
		return agentruntime.DomainCommitReconcileResult{Found: false}, nil
	}}
	firstRuntime, err := agentruntime.NewRuntime(firstEngine, store, agentruntime.RuntimeConfig{})
	if err != nil {
		t.Fatal(err)
	}
	harness, err := firstRuntime.Open(context.Background(), binding)
	if err != nil {
		t.Fatal(err)
	}
	status, err := harness.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	abandoned := domainCommitForStage(t, status.DomainCommits, agentruntime.DomainCommitOutput)
	if !status.RecoveryPaused || !abandoned.Abandoned || abandoned.Revision != "" || firstEngine.calls.Load() != 1 {
		t.Fatalf("authoritative not-found status = %#v calls=%d", status, firstEngine.calls.Load())
	}
	if _, err := harness.Submit(context.Background(), agentruntime.Abort{
		ID: "abort-after-file-abandonment", OperationID: status.ActiveOperation, Reason: "finish abandoned recovery",
	}); err != nil {
		t.Fatal(err)
	}
	status, err = harness.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if status.Phase != agentruntime.PhaseIdle || status.LastOperation == nil || status.LastOperation.Status != agentruntime.OperationAborted {
		t.Fatalf("abort after abandonment = %#v", status)
	}
	if err := firstRuntime.Close(context.Background()); err != nil {
		t.Fatal(err)
	}

	reopenedStore, err := agentruntime.NewFileJournalStore(root)
	if err != nil {
		t.Fatal(err)
	}
	reopenedEngine := &domainCommitReconcileEngine{reconcile: func(context.Context, agentruntime.DomainCommitReconcileRequest) (agentruntime.DomainCommitReconcileResult, error) {
		return agentruntime.DomainCommitReconcileResult{}, errors.New("durable abandonment must not query canonical state again")
	}}
	reopenedRuntime, err := agentruntime.NewRuntime(reopenedEngine, reopenedStore, agentruntime.RuntimeConfig{})
	if err != nil {
		t.Fatal(err)
	}
	reopenedHarness, err := reopenedRuntime.Open(context.Background(), binding)
	if err != nil {
		t.Fatal(err)
	}
	reopenedStatus, err := reopenedHarness.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if reopenedEngine.calls.Load() != 0 || reopenedStatus.Phase != agentruntime.PhaseIdle ||
		reopenedStatus.LastOperation == nil || reopenedStatus.LastOperation.Status != agentruntime.OperationAborted {
		t.Fatalf("reopened abandonment status = %#v calls=%d", reopenedStatus, reopenedEngine.calls.Load())
	}
	if persisted := domainCommitForStage(t, reopenedStatus.DomainCommits, agentruntime.DomainCommitOutput); !persisted.Abandoned || persisted.Revision != "" {
		t.Fatalf("persisted abandonment = %#v", persisted)
	}
	if err := reopenedRuntime.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
}

type domainCommitReconcileEngine struct {
	reconcile func(context.Context, agentruntime.DomainCommitReconcileRequest) (agentruntime.DomainCommitReconcileResult, error)
	calls     atomic.Int32
}

func (e *domainCommitReconcileEngine) NewEngine(context.Context, agentruntime.BindingRef) (agentruntime.Engine, error) {
	return e, nil
}

func (*domainCommitReconcileEngine) Run(context.Context, agentruntime.EngineRequest, agentruntime.EngineEventSink) (agentruntime.EngineResult, error) {
	return agentruntime.EngineResult{}, errors.New("recovery must not rerun the engine")
}

func (e *domainCommitReconcileEngine) ReconcileDomainCommit(ctx context.Context, request agentruntime.DomainCommitReconcileRequest) (agentruntime.DomainCommitReconcileResult, error) {
	e.calls.Add(1)
	return e.reconcile(ctx, request)
}

func seedPendingDomainCommit(
	t *testing.T,
	store agentruntime.JournalStore,
	binding agentruntime.Binding,
	stage agentruntime.DomainCommitStage,
	hash string,
) agentruntime.DomainCommitIdentity {
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
	identity := agentruntime.DomainCommitIdentity{
		CommandID: "command", OperationID: "operation", Cycle: 1, Stage: stage,
	}
	if _, err := journal.Append(context.Background(), 0, []agentruntime.EventPayload{
		agentruntime.CommandAcceptedEvent{CommandID: identity.CommandID, CommandKind: "start_turn", OperationID: identity.OperationID, Fingerprint: "seed"},
		agentruntime.OperationStartedEvent{OperationID: identity.OperationID},
		agentruntime.UserMessageCommittedEvent{Message: agentruntime.Message{
			ID: "user", Role: agentruntime.RoleUser, Content: "write",
			Input: agentruntime.UserInput{Text: "write"}, Operation: identity.OperationID,
		}},
		agentruntime.CycleStartedEvent{OperationID: identity.OperationID, Cycle: identity.Cycle, SnapshotID: "snapshot"},
		agentruntime.DomainCommitIntentAcceptedEvent{Identity: identity, Hash: hash},
	}); err != nil {
		_ = journal.Close()
		t.Fatal(err)
	}
	if err := journal.Close(); err != nil {
		t.Fatal(err)
	}
	return identity
}

func appendAbortToPendingDomainCommit(t *testing.T, store agentruntime.JournalStore, binding agentruntime.Binding, reason string) {
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
	events, err := journal.Load(context.Background())
	if err != nil {
		_ = journal.Close()
		t.Fatal(err)
	}
	if _, err := journal.Append(context.Background(), agentruntime.Cursor(len(events)), []agentruntime.EventPayload{
		agentruntime.AbortRequestedEvent{OperationID: "operation", Reason: reason},
	}); err != nil {
		_ = journal.Close()
		t.Fatal(err)
	}
	if err := journal.Close(); err != nil {
		t.Fatal(err)
	}
}

func domainCommitForStage(t *testing.T, commits []agentruntime.DomainCommitState, stage agentruntime.DomainCommitStage) agentruntime.DomainCommitState {
	t.Helper()
	for _, commit := range commits {
		if commit.Identity.Stage == stage {
			return commit
		}
	}
	t.Fatalf("domain commit stage %q not found in %#v", stage, commits)
	return agentruntime.DomainCommitState{}
}
