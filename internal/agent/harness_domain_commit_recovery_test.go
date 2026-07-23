package agent

import (
	"context"
	"errors"
	"testing"

	runstate "denova/internal/agent/runtime"
)

func TestBindingHarnessEngineDelegatesExactDomainCommitQuery(t *testing.T) {
	t.Parallel()

	binding := runstate.BindingRef{
		Kind: runstate.BindingWriting, Profile: runstate.ProfileWriting,
		Workspace: "/book", SessionID: "session",
	}
	want := runstate.DomainCommitReconcileRequest{
		Binding: binding,
		Commit: runstate.DomainCommitState{
			Identity: runstate.DomainCommitIdentity{
				CommandID: "command", OperationID: "operation", Cycle: 1, Stage: runstate.DomainCommitOutput,
			},
			Hash: "sha256:exact",
		},
	}
	owner := newHarnessEngine(newTurnExecutor(DefaultLoopPolicy()))
	owner.domainCommitReconciler = func(_ context.Context, got runstate.DomainCommitReconcileRequest) (runstate.DomainCommitReconcileResult, error) {
		if got.Binding != want.Binding || got.Commit != want.Commit {
			return runstate.DomainCommitReconcileResult{}, errors.New("query changed across adapter")
		}
		return runstate.DomainCommitReconcileResult{Found: true, Revision: "canonical:1"}, nil
	}
	engine, err := owner.NewEngine(context.Background(), binding)
	if err != nil {
		t.Fatal(err)
	}
	reconciler := engine.(runstate.EngineDomainCommitReconciler)
	result, err := reconciler.ReconcileDomainCommit(context.Background(), want)
	if err != nil || !result.Found || result.Revision != "canonical:1" {
		t.Fatalf("adapter result = %#v err=%v", result, err)
	}

	wrong := want
	wrong.Binding.SessionID = "other"
	if _, err := reconciler.ReconcileDomainCommit(context.Background(), wrong); !errors.Is(err, ErrHarnessBindingMismatch) {
		t.Fatalf("binding mismatch error = %v, want %v", err, ErrHarnessBindingMismatch)
	}
}

func TestWithHarnessDomainCommitReconcilerRejectsNil(t *testing.T) {
	t.Parallel()
	if _, err := NewDurableChatService(context.Background(), t.TempDir(), WithHarnessDomainCommitReconciler(nil)); err == nil {
		t.Fatal("nil domain commit reconciler was accepted")
	}
}
