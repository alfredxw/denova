package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/cloudwego/eino/schema"

	"denova/config"
	"denova/internal/agentruntime"
	"denova/internal/session"
)

type recordingContextStructuralOperation struct {
	mu        sync.Mutex
	prepared  int
	committed int
	result    ContextStructuralResult
	receipt   ContextStructuralReceipt
	hash      string
}

func TestSessionPostSettlementCompactionPublishesAfterAssistantCursor(t *testing.T) {
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.Create("post settlement compaction")
	if err != nil {
		t.Fatal(err)
	}
	if err := sess.Append(schema.UserMessage("old user")); err != nil {
		t.Fatal(err)
	}
	conversation := NewSessionConversationForAgent(sess, &config.Config{}, AgentKindIDE)
	conversation.stagePreparedSessionCompaction(preparedSessionContextCompaction{
		Result: ContextCompactionResult{
			Triggered: true, Phase: contextCompactionPhaseMidRun, Epoch: 1, Summary: "bounded old history",
			SourceMessageCount: 1, RetainedTurns: 2,
		},
		SourceStartIndex: 0, SourceEndIndex: 1,
	})
	if err := sess.Append(schema.AssistantMessage("settled assistant", nil)); err != nil {
		t.Fatal(err)
	}
	spec, err := conversation.PostSettlementContextStructuralSpec(context.Background(), "settled-operation", RunOptions{
		AgentKind: AgentKindIDE, Workspace: "/book", SessionID: sess.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if spec == nil {
		t.Fatal("expected staged post-settlement structural spec")
	}
	if spec.RestorePlan == nil || spec.RestorePlan.Domain != ContextStructuralDomainSession ||
		spec.RestorePlan.RecordID == "" || spec.RestorePlan.IntentHash == "" || len(spec.RestorePlan.Mutation) == 0 {
		t.Fatalf("post-settlement Session compaction has no exact restore plan: %#v", spec.RestorePlan)
	}
	service := NewEphemeralChatService()
	defer service.Close(context.Background())
	result, err := service.ExecuteContextStructuralOperation(context.Background(), *spec)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Compaction.Triggered {
		t.Fatalf("unexpected result: %#v", result)
	}
	record, ok := sess.LatestContextCompaction(AgentKindIDE)
	if !ok || record.SourceEndIndex != 1 || record.ContextRevision != sess.ContextCursor().Revision {
		t.Fatalf("post-settlement checkpoint = %#v ok=%t", record, ok)
	}
	if got := sess.GetEffectiveMessages(); len(got) != 2 {
		t.Fatalf("structural checkpoint modified raw display history: %#v", got)
	}
}

func (o *recordingContextStructuralOperation) Prepare(_ context.Context, identity ContextStructuralIdentity, _ func(Event)) (ContextStructuralIntent, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.prepared++
	if identity.CommandID == "" || identity.OperationID == "" || identity.Cycle != 1 {
		return ContextStructuralIntent{}, fmt.Errorf("missing structural identity: %#v", identity)
	}
	hash := o.hash
	if hash == "" {
		hash = "sha256:prepared"
	}
	return ContextStructuralIntent{Hash: hash, Commit: true, Result: o.result}, nil
}

func (o *recordingContextStructuralOperation) Commit(_ context.Context, _ ContextStructuralIdentity, intent ContextStructuralIntent) (ContextStructuralReceipt, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.committed++
	wantHash := o.hash
	if wantHash == "" {
		wantHash = "sha256:prepared"
	}
	if intent.Hash != wantHash {
		return ContextStructuralReceipt{}, fmt.Errorf("unexpected intent hash %q", intent.Hash)
	}
	return o.receipt, nil
}

func (o *recordingContextStructuralOperation) Reconcile(context.Context) (ContextStructuralResult, ContextStructuralReceipt, bool, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.result, o.receipt, o.committed > 0, nil
}

func TestExecuteContextStructuralOperationUsesDurableBindingAndReceipt(t *testing.T) {
	service, err := newHarnessChatService(context.Background(), DefaultLoopPolicy(), agentruntime.NewMemoryJournalStore())
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close(context.Background())
	op := &recordingContextStructuralOperation{
		result:  ContextStructuralResult{Compaction: ContextCompactionResult{Triggered: true, Epoch: 2, Summary: "bounded checkpoint"}},
		receipt: ContextStructuralReceipt{Revision: "context:8"},
	}
	options := RunOptions{AgentKind: AgentKindIDE, Workspace: "/book", SessionID: "session-1"}
	binding, err := harnessBindingForOptions(options)
	if err != nil {
		t.Fatal(err)
	}
	bindingRef, err := agentruntime.BindingReference(binding)
	if err != nil {
		t.Fatal(err)
	}
	mutation := json.RawMessage(`{"id":"cc-manual-compaction-context-7"}`)
	op.hash, err = ContextStructuralIntentHash(
		ContextStructuralCompact,
		bindingRef,
		"context:7",
		"cc-manual-compaction-context-7",
		mutation,
	)
	if err != nil {
		t.Fatal(err)
	}
	plan := ContextStructuralRestorePlan{
		Version:    ContextStructuralRestorePlanVersion,
		Domain:     ContextStructuralDomainSession,
		Action:     ContextStructuralCompact,
		Commit:     true,
		IntentHash: op.hash,
		RecordID:   "cc-manual-compaction-context-7",
		Result:     op.result,
		Mutation:   mutation,
	}
	result, err := service.ExecuteContextStructuralOperation(context.Background(), ContextStructuralSpec{
		CommandID: "manual-compaction-context-7",
		Action:    ContextStructuralCompact,
		Ref: agentruntime.ContextCompactionRef{
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
	service := NewEphemeralChatService()
	defer service.Close(context.Background())

	_, err := service.ExecuteContextStructuralOperation(context.Background(), ContextStructuralSpec{
		CommandID: "missing-restore-plan",
		Action:    ContextStructuralCompact,
		Ref: agentruntime.ContextCompactionRef{
			Source: "session.effective_messages", Purpose: "test invalid admission",
			Resource: "session-1", ExpectedRevision: "context:7",
		},
		Options:   RunOptions{AgentKind: AgentKindIDE, Workspace: "/book", SessionID: "session-1"},
		Operation: &recordingContextStructuralOperation{},
	})
	if !errors.Is(err, agentruntime.ErrInvalidCommand) {
		t.Fatalf("error = %v, want ErrInvalidCommand", err)
	}
}

func TestStructuralRecoveryPinsExactReplayRegistrationUntilEngineTake(t *testing.T) {
	t.Parallel()

	engine := newHarnessEngine(newTurnExecutor(DefaultLoopPolicy()))
	binding := agentruntime.BindingRef{
		Kind: agentruntime.BindingWriting, Profile: agentruntime.ProfileWriting,
		Workspace: "/book", SessionID: "recovered-structural",
	}
	guard := &bindingHarnessEngine{owner: engine, binding: binding}
	ref := "recovered-structural-spec"
	command := agentruntime.CompactIfNeeded{ID: "recovered-structural", Ref: agentruntime.ContextCompactionRef{
		SpecRef: ref, Source: "session.messages", Purpose: "checkpoint",
		Resource: "recovered-structural", ExpectedRevision: "context:7",
	}}
	lease, err := engine.register(ref, command, HarnessTurnSpec{
		CommandID: command.ID, CommandKind: AgentCommandKind(ContextStructuralCompact),
		Conversation: &contextStructuralConversation{
			action: ContextStructuralCompact, operation: &recordingContextStructuralOperation{},
		},
		Options: RunOptions{AgentKind: AgentKindIDE, Workspace: "/book", SessionID: "recovered-structural"}.normalized(""),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := guard.RestoreStructuralOperation(context.Background(), agentruntime.StructuralOperationSnapshot{
		Binding: binding, CommandID: command.ID, OperationID: "operation", Cycle: 1,
		Kind: agentruntime.StructuralCompactContext, Ref: command.Ref,
	}); err != nil {
		t.Fatal(err)
	}
	lease.release()
	if _, err := engine.take(ref); err != nil {
		t.Fatalf("exact replay registration was released before recovered Engine.Run: %v", err)
	}
}
