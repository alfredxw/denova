package execution

import (
	"context"
	"denova/internal/agents/run"
	"errors"
	"testing"

	runstate "github.com/alfredxw/denova/agent/runtime"
)

func TestBindingExecutionEngineDelegatesExactDomainCommitQuery(t *testing.T) {
	t.Parallel()

	productBinding := agentrun.RuntimeBinding{AgentKind: agentrun.AgentKindIDE, Workspace: "/book", SessionID: "session"}
	binding := mustRuntimeBinding(productBinding)
	want := runstate.DomainCommitReconcileRequest{
		Binding: binding,
		Commit: runstate.DomainCommitState{
			Identity: runstate.DomainCommitIdentity{
				CommandID: "command", OperationID: "operation", Cycle: 1, Stage: runstate.DomainCommitOutput,
			},
			Hash: "sha256:exact",
		},
	}
	profile := &testExecutionProfile{id: ProfileWriting}
	profile.reconcile = func(_ context.Context, got agentrun.DomainCommitReconcileRequest) (agentrun.DomainCommitReconcileResult, error) {
		if got.Binding != productBinding || got.Commit != (agentrun.DomainCommitState{
			Identity: agentrun.DomainCommitIdentity{CommandID: "command", OperationID: "operation", Cycle: 1, Stage: agentrun.DomainCommitOutput},
			Hash:     "sha256:exact",
		}) {
			return agentrun.DomainCommitReconcileResult{}, errors.New("query changed across adapter")
		}
		return agentrun.DomainCommitReconcileResult{Found: true, Revision: "canonical:1"}, nil
	}
	profiles, err := newProfileRegistry([]Profile{profile})
	if err != nil {
		t.Fatal(err)
	}
	owner := newDurableEngine(newTestExecutor(agentrun.DefaultLoopPolicy()), profiles)
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
	wrong.Binding = mustRuntimeBinding(agentrun.RuntimeBinding{AgentKind: agentrun.AgentKindIDE, Workspace: "/book", SessionID: "other"})
	if _, err := reconciler.ReconcileDomainCommit(context.Background(), wrong); !errors.Is(err, ErrBindingMismatch) {
		t.Fatalf("binding mismatch error = %v, want %v", err, ErrBindingMismatch)
	}
}

func TestWithProfilesRejectsNil(t *testing.T) {
	t.Parallel()
	if _, err := NewDurableRuntime(context.Background(), t.TempDir(), WithProfiles(nil)); err == nil {
		t.Fatal("nil execution profile was accepted")
	}
}
