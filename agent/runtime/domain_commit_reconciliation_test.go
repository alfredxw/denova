package runtime_test

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"sync/atomic"
	"testing"

	runstate "github.com/alfredxw/denova/agent/runtime"
)

func TestRecoveryReconcilesCanonicalDomainCommitBeforeSettling(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name       string
		stage      runstate.DomainCommitStage
		wantStatus runstate.OperationStatus
	}{
		{name: "output settles successfully", stage: runstate.DomainCommitOutput, wantStatus: runstate.OperationSucceeded},
		{name: "input keeps receipt on interruption", stage: runstate.DomainCommitInput, wantStatus: runstate.OperationInterrupted},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			store := runstate.NewMemoryJournalStore()
			binding := testBindingAt("/book", "reconcile-"+string(test.stage))
			identity := seedPendingDomainCommit(t, store, binding, test.stage, "sha256:canonical")
			engine := &domainCommitReconcileEngine{reconcile: func(_ context.Context, request runstate.DomainCommitReconcileRequest) (runstate.DomainCommitReconcileResult, error) {
				if request.Binding.Label("workspace") != "/book" || request.Binding.Key != binding.Key || request.Commit.Identity != identity || request.Commit.Hash != "sha256:canonical" {
					return runstate.DomainCommitReconcileResult{}, errors.New("runtime did not query the exact pending commit")
				}
				return runstate.DomainCommitReconcileResult{Found: true, Revision: "canonical:9"}, nil
			}}
			runtime, err := runstate.NewRuntime(engine, store, runstate.RuntimeConfig{})
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
			if test.stage == runstate.DomainCommitInput {
				if status.Phase != runstate.PhaseRunning || !status.RecoveryPaused || status.LastOperation != nil {
					t.Fatalf("recovered status = %#v, want recovery-paused Running", status)
				}
			} else if status.Phase != runstate.PhaseIdle || status.LastOperation == nil || status.LastOperation.Status != test.wantStatus {
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

	store := runstate.NewMemoryJournalStore()
	binding := testBindingAt("/book", "reconcile-not-found")
	seedPendingDomainCommit(t, store, binding, runstate.DomainCommitOutput, "sha256:missing")
	engine := &domainCommitReconcileEngine{reconcile: func(context.Context, runstate.DomainCommitReconcileRequest) (runstate.DomainCommitReconcileResult, error) {
		return runstate.DomainCommitReconcileResult{Found: false}, nil
	}}
	runtime, err := runstate.NewRuntime(engine, store, runstate.RuntimeConfig{})
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
	if status.Phase != runstate.PhaseRunning || !status.RecoveryPaused || status.LastOperation != nil {
		t.Fatalf("recovered status = %#v, want recovery-paused Running", status)
	}
	if got := domainCommitForStage(t, status.DomainCommits, runstate.DomainCommitOutput).Revision; got != "" {
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

	store := runstate.NewMemoryJournalStore()
	binding := testBindingAt("/book", "reconcile-input-abort")
	seedPendingDomainCommit(t, store, binding, runstate.DomainCommitInput, "sha256:input")
	appendAbortToPendingDomainCommit(t, store, binding, "user stopped")
	engine := &domainCommitReconcileEngine{reconcile: func(context.Context, runstate.DomainCommitReconcileRequest) (runstate.DomainCommitReconcileResult, error) {
		return runstate.DomainCommitReconcileResult{Found: true, Revision: "canonical:input"}, nil
	}}
	runtime, err := runstate.NewRuntime(engine, store, runstate.RuntimeConfig{})
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
	if status.LastOperation == nil || status.LastOperation.Status != runstate.OperationAborted || status.LastOperation.Reason != "user stopped" {
		t.Fatalf("recovered abort = %#v", status.LastOperation)
	}
	if got := domainCommitForStage(t, status.DomainCommits, runstate.DomainCommitInput).Revision; got != "canonical:input" {
		t.Fatalf("input receipt revision = %q", got)
	}
	if err := runtime.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestRecoveryQueryErrorLeavesOperationRetryable(t *testing.T) {
	t.Parallel()

	store := runstate.NewMemoryJournalStore()
	binding := testBindingAt("/book", "reconcile-error")
	seedPendingDomainCommit(t, store, binding, runstate.DomainCommitOutput, "sha256:uncertain")
	queryErr := errors.New("canonical store unavailable")
	failedRuntime, err := runstate.NewRuntime(&domainCommitReconcileEngine{reconcile: func(context.Context, runstate.DomainCommitReconcileRequest) (runstate.DomainCommitReconcileResult, error) {
		return runstate.DomainCommitReconcileResult{}, queryErr
	}}, store, runstate.RuntimeConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := failedRuntime.Open(context.Background(), binding); !errors.Is(err, queryErr) {
		t.Fatalf("open error = %v, want %v", err, queryErr)
	}

	// A later open must still see the pending intent. If the failed query wrote
	// an interrupted terminal event, this exact canonical receipt cannot win.
	retryEngine := &domainCommitReconcileEngine{reconcile: func(context.Context, runstate.DomainCommitReconcileRequest) (runstate.DomainCommitReconcileResult, error) {
		return runstate.DomainCommitReconcileResult{Found: true, Revision: "canonical:retry"}, nil
	}}
	retryRuntime, err := runstate.NewRuntime(retryEngine, store, runstate.RuntimeConfig{})
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
	if status.LastOperation == nil || status.LastOperation.Status != runstate.OperationSucceeded {
		t.Fatalf("retry status = %#v, want succeeded", status)
	}
	if got := domainCommitForStage(t, status.DomainCommits, runstate.DomainCommitOutput).Revision; got != "canonical:retry" {
		t.Fatalf("retry revision = %q", got)
	}
	if err := retryRuntime.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestFileJournalPersistsReconciledReceiptAcrossReopen(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "journals")
	store, err := runstate.NewFileJournalStore(root)
	if err != nil {
		t.Fatal(err)
	}
	binding := testBindingAt("/book", "file-reconcile")
	seedPendingDomainCommit(t, store, binding, runstate.DomainCommitOutput, "sha256:file")
	firstEngine := &domainCommitReconcileEngine{reconcile: func(context.Context, runstate.DomainCommitReconcileRequest) (runstate.DomainCommitReconcileResult, error) {
		return runstate.DomainCommitReconcileResult{Found: true, Revision: "canonical:file"}, nil
	}}
	firstRuntime, err := runstate.NewRuntime(firstEngine, store, runstate.RuntimeConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := firstRuntime.Open(context.Background(), binding); err != nil {
		t.Fatalf("first open: %v", err)
	}
	if err := firstRuntime.Close(context.Background()); err != nil {
		t.Fatal(err)
	}

	reopenedStore, err := runstate.NewFileJournalStore(root)
	if err != nil {
		t.Fatal(err)
	}
	reopenedEngine := &domainCommitReconcileEngine{reconcile: func(context.Context, runstate.DomainCommitReconcileRequest) (runstate.DomainCommitReconcileResult, error) {
		return runstate.DomainCommitReconcileResult{}, errors.New("persisted receipt should not be queried again")
	}}
	reopenedRuntime, err := runstate.NewRuntime(reopenedEngine, reopenedStore, runstate.RuntimeConfig{})
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
	if status.LastOperation == nil || status.LastOperation.Status != runstate.OperationSucceeded {
		t.Fatalf("reopened status = %#v", status)
	}
	if got := domainCommitForStage(t, status.DomainCommits, runstate.DomainCommitOutput).Revision; got != "canonical:file" {
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
	store, err := runstate.NewFileJournalStore(root)
	if err != nil {
		t.Fatal(err)
	}
	binding := testBindingAt("/book", "file-not-found-abandonment")
	seedPendingDomainCommit(t, store, binding, runstate.DomainCommitOutput, "sha256:file-not-found")
	firstEngine := &domainCommitReconcileEngine{reconcile: func(context.Context, runstate.DomainCommitReconcileRequest) (runstate.DomainCommitReconcileResult, error) {
		return runstate.DomainCommitReconcileResult{Found: false}, nil
	}}
	firstRuntime, err := runstate.NewRuntime(firstEngine, store, runstate.RuntimeConfig{})
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
	abandoned := domainCommitForStage(t, status.DomainCommits, runstate.DomainCommitOutput)
	if !status.RecoveryPaused || !abandoned.Abandoned || abandoned.Revision != "" || firstEngine.calls.Load() != 1 {
		t.Fatalf("authoritative not-found status = %#v calls=%d", status, firstEngine.calls.Load())
	}
	if _, err := harness.Submit(context.Background(), runstate.Abort{
		ID: "abort-after-file-abandonment", OperationID: status.ActiveOperation, Reason: "finish abandoned recovery",
	}); err != nil {
		t.Fatal(err)
	}
	status, err = harness.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if status.Phase != runstate.PhaseIdle || status.LastOperation == nil || status.LastOperation.Status != runstate.OperationAborted {
		t.Fatalf("abort after abandonment = %#v", status)
	}
	if err := firstRuntime.Close(context.Background()); err != nil {
		t.Fatal(err)
	}

	reopenedStore, err := runstate.NewFileJournalStore(root)
	if err != nil {
		t.Fatal(err)
	}
	reopenedEngine := &domainCommitReconcileEngine{reconcile: func(context.Context, runstate.DomainCommitReconcileRequest) (runstate.DomainCommitReconcileResult, error) {
		return runstate.DomainCommitReconcileResult{}, errors.New("durable abandonment must not query canonical state again")
	}}
	reopenedRuntime, err := runstate.NewRuntime(reopenedEngine, reopenedStore, runstate.RuntimeConfig{})
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
	if reopenedEngine.calls.Load() != 0 || reopenedStatus.Phase != runstate.PhaseIdle ||
		reopenedStatus.LastOperation == nil || reopenedStatus.LastOperation.Status != runstate.OperationAborted {
		t.Fatalf("reopened abandonment status = %#v calls=%d", reopenedStatus, reopenedEngine.calls.Load())
	}
	if persisted := domainCommitForStage(t, reopenedStatus.DomainCommits, runstate.DomainCommitOutput); !persisted.Abandoned || persisted.Revision != "" {
		t.Fatalf("persisted abandonment = %#v", persisted)
	}
	if err := reopenedRuntime.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
}

type domainCommitReconcileEngine struct {
	reconcile func(context.Context, runstate.DomainCommitReconcileRequest) (runstate.DomainCommitReconcileResult, error)
	calls     atomic.Int32
}

func (e *domainCommitReconcileEngine) NewEngine(context.Context, runstate.BindingRef) (runstate.Engine, error) {
	return e, nil
}

func (*domainCommitReconcileEngine) Run(context.Context, runstate.EngineRequest, runstate.EngineEventSink) (runstate.EngineResult, error) {
	return runstate.EngineResult{}, errors.New("recovery must not rerun the engine")
}

func (e *domainCommitReconcileEngine) ReconcileDomainCommit(ctx context.Context, request runstate.DomainCommitReconcileRequest) (runstate.DomainCommitReconcileResult, error) {
	e.calls.Add(1)
	return e.reconcile(ctx, request)
}

func seedPendingDomainCommit(
	t *testing.T,
	store runstate.JournalStore,
	binding runstate.BindingRef,
	stage runstate.DomainCommitStage,
	hash string,
) runstate.DomainCommitIdentity {
	t.Helper()
	if err := runstate.ValidateBindingRef(binding); err != nil {
		t.Fatal(err)
	}
	key, err := json.Marshal(binding)
	if err != nil {
		t.Fatal(err)
	}
	journal, err := store.OpenJournal(context.Background(), string(key))
	if err != nil {
		t.Fatal(err)
	}
	identity := runstate.DomainCommitIdentity{
		CommandID: "command", OperationID: "operation", Cycle: 1, Stage: stage,
	}
	if _, err := journal.Append(context.Background(), 0, []runstate.EventPayload{
		runstate.CommandAcceptedEvent{CommandID: identity.CommandID, CommandKind: "start_turn", OperationID: identity.OperationID, Fingerprint: "seed"},
		runstate.OperationStartedEvent{OperationID: identity.OperationID},
		runstate.UserMessageCommittedEvent{Message: runstate.Message{
			ID: "user", Role: runstate.RoleUser, Content: "write",
			Input: runstate.UserInput{Text: "write"}, Operation: identity.OperationID,
		}},
		runstate.CycleStartedEvent{OperationID: identity.OperationID, Cycle: identity.Cycle, SnapshotID: "snapshot"},
		runstate.DomainCommitIntentAcceptedEvent{Identity: identity, Hash: hash},
	}); err != nil {
		_ = journal.Close()
		t.Fatal(err)
	}
	if err := journal.Close(); err != nil {
		t.Fatal(err)
	}
	return identity
}

func appendAbortToPendingDomainCommit(t *testing.T, store runstate.JournalStore, binding runstate.BindingRef, reason string) {
	t.Helper()
	if err := runstate.ValidateBindingRef(binding); err != nil {
		t.Fatal(err)
	}
	key, err := json.Marshal(binding)
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
	if _, err := journal.Append(context.Background(), runstate.Cursor(len(events)), []runstate.EventPayload{
		runstate.AbortRequestedEvent{OperationID: "operation", Reason: reason},
	}); err != nil {
		_ = journal.Close()
		t.Fatal(err)
	}
	if err := journal.Close(); err != nil {
		t.Fatal(err)
	}
}

func domainCommitForStage(t *testing.T, commits []runstate.DomainCommitState, stage runstate.DomainCommitStage) runstate.DomainCommitState {
	t.Helper()
	for _, commit := range commits {
		if commit.Identity.Stage == stage {
			return commit
		}
	}
	t.Fatalf("domain commit stage %q not found in %#v", stage, commits)
	return runstate.DomainCommitState{}
}
