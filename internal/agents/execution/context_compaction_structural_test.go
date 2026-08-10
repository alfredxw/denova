package execution

import (
	"context"
	agentstructural "denova/internal/agents/context/structural"
	agentrun "denova/internal/agents/run"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"

	agentcompaction "denova/internal/agents/context/compaction"

	runstate "github.com/alfredxw/denova/agent/runtime"
)

type recordingContextStructuralOperation struct {
	mu        sync.Mutex
	prepared  int
	committed int
	result    agentstructural.Result
	receipt   agentstructural.Receipt
	hash      string
}

func (o *recordingContextStructuralOperation) Prepare(_ context.Context, identity agentstructural.Identity, _ func(agentrun.Event)) (agentstructural.Intent, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.prepared++
	if identity.CommandID == "" || identity.OperationID == "" || identity.Cycle != 1 {
		return agentstructural.Intent{}, fmt.Errorf("missing structural identity: %#v", identity)
	}
	hash := o.hash
	if hash == "" {
		hash = "sha256:prepared"
	}
	return agentstructural.Intent{Hash: hash, Commit: true, Result: o.result}, nil
}

func (o *recordingContextStructuralOperation) Commit(_ context.Context, _ agentstructural.Identity, intent agentstructural.Intent) (agentstructural.Receipt, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.committed++
	wantHash := o.hash
	if wantHash == "" {
		wantHash = "sha256:prepared"
	}
	if intent.Hash != wantHash {
		return agentstructural.Receipt{}, fmt.Errorf("unexpected intent hash %q", intent.Hash)
	}
	return o.receipt, nil
}

func (o *recordingContextStructuralOperation) Reconcile(context.Context) (agentstructural.Result, agentstructural.Receipt, bool, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.result, o.receipt, o.committed > 0, nil
}

func TestExecuteContextStructuralOperationUsesDurableBindingAndReceipt(t *testing.T) {
	service, err := newRuntime(context.Background(), agentrun.DefaultLoopPolicy(), runstate.NewMemoryJournalStore())
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close(context.Background())
	op := &recordingContextStructuralOperation{
		result:  agentstructural.Result{Compaction: agentcompaction.Result{Triggered: true, Epoch: 2, Summary: "bounded checkpoint"}},
		receipt: agentstructural.Receipt{Revision: "context:8"},
	}
	options := agentrun.Options{AgentKind: agentrun.AgentKindIDE, Workspace: "/book", SessionID: "session-1"}
	binding, err := agentrun.BindingForOptions(options)
	if err != nil {
		t.Fatal(err)
	}
	bindingRef, err := runstate.BindingReference(binding)
	if err != nil {
		t.Fatal(err)
	}
	mutation := json.RawMessage(`{"id":"cc-manual-compaction-context-7"}`)
	productBinding, err := agentrun.ParseRuntimeBinding(bindingRef)
	if err != nil {
		t.Fatal(err)
	}
	op.hash, err = agentstructural.IntentHash(
		agentstructural.Compact,
		productBinding,
		"context:7",
		"cc-manual-compaction-context-7",
		mutation,
	)
	if err != nil {
		t.Fatal(err)
	}
	plan := agentstructural.RestorePlan{
		Version:    agentstructural.RestorePlanVersion,
		Domain:     agentstructural.DomainSession,
		Action:     agentstructural.Compact,
		Commit:     true,
		IntentHash: op.hash,
		RecordID:   "cc-manual-compaction-context-7",
		Result:     op.result,
		Mutation:   mutation,
	}
	result, err := service.ExecuteStructuralOperation(context.Background(), agentstructural.Spec{
		CommandID: "manual-compaction-context-7",
		Action:    agentstructural.Compact,
		Ref: agentrun.ContextCompactionRef{
			Source: "session.effective_messages", Purpose: "bounded model history checkpoint",
			Resource: "session-1", ExpectedRevision: "context:7", Force: true,
		},
		Options: options, Operation: op, RestorePlan: &plan,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Compaction.Triggered || result.Compaction.Epoch != 2 {
		t.Fatalf("unexpected structural result: %#v", result)
	}
	op.mu.Lock()
	prepared, committed := op.prepared, op.committed
	op.mu.Unlock()
	if prepared != 1 || committed != 1 {
		t.Fatalf("prepare/commit calls = %d/%d, want 1/1", prepared, committed)
	}
}

func TestExecuteContextStructuralOperationRejectsUnrecoverableSpec(t *testing.T) {
	service := NewEphemeralRuntime()
	defer service.Close(context.Background())

	_, err := service.ExecuteStructuralOperation(context.Background(), agentstructural.Spec{
		CommandID: "missing-restore-plan",
		Action:    agentstructural.Compact,
		Ref: agentrun.ContextCompactionRef{
			Source: "session.effective_messages", Purpose: "test invalid admission",
			Resource: "session-1", ExpectedRevision: "context:7",
		},
		Options:   agentrun.Options{AgentKind: agentrun.AgentKindIDE, Workspace: "/book", SessionID: "session-1"},
		Operation: &recordingContextStructuralOperation{},
	})
	if !errors.Is(err, runstate.ErrInvalidCommand) {
		t.Fatalf("error = %v, want agentrun.ErrInvalidCommand", err)
	}
}

func TestStructuralRecoveryPinsExactReplayRegistrationUntilEngineTake(t *testing.T) {
	t.Parallel()

	engine := newDurableEngine(newTestExecutor(agentrun.DefaultLoopPolicy()))
	binding := mustRuntimeBinding(agentrun.RuntimeBinding{AgentKind: agentrun.AgentKindIDE, Workspace: "/book", SessionID: "recovered-structural"})
	guard := &bindingEngine{owner: engine, binding: binding}
	ref := "recovered-structural-spec"
	command := runstate.CompactIfNeeded{ID: "recovered-structural", Ref: runstate.ContextCompactionRef{
		SpecRef: ref, Source: "session.messages", Purpose: "checkpoint",
		Resource: "recovered-structural", ExpectedRevision: "context:7",
	}}
	lease, err := engine.register(ref, command, cycleSpec{
		CommandID: agentrun.CommandID(command.ID), CommandKind: CommandKind(agentstructural.Compact),
		Conversation: &contextStructuralConversation{
			action: agentstructural.Compact, operation: &recordingContextStructuralOperation{},
		},
		Options: agentrun.Options{AgentKind: agentrun.AgentKindIDE, Workspace: "/book", SessionID: "recovered-structural"}.Normalize(""),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := guard.RestoreStructuralOperation(context.Background(), runstate.StructuralOperationSnapshot{
		Binding: binding, CommandID: command.ID, OperationID: "operation", Cycle: 1,
		Kind: runstate.StructuralCompactContext, Ref: command.Ref,
	}); err != nil {
		t.Fatal(err)
	}
	lease.release()
	if _, err := engine.take(ref); err != nil {
		t.Fatalf("exact replay registration was released before recovered Engine.Run: %v", err)
	}
}
