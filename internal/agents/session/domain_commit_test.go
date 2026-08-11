package session

import (
	"errors"
	"testing"

	agent "github.com/alfredxw/denova/agent"

	agentcontext "denova/internal/agents/context"
)

func TestCommitDomainMessageIsIdempotentAndPersistsCoordinatorIdentity(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.GetOrCreate("domain-commit")
	if err != nil {
		t.Fatal(err)
	}
	intent, err := NewDomainCommitIntent(
		DomainCommitIdentity{CommandID: "command-1", OperationID: "operation-1", Cycle: 1},
		agent.AssistantMessage("canonical answer", nil),
		MessageMetadata{RunID: "run-1", AgentKind: "ide"},
	)
	if err != nil {
		t.Fatal(err)
	}

	first, err := sess.CommitDomainMessage(intent)
	if err != nil {
		t.Fatal(err)
	}
	second, err := sess.CommitDomainMessage(intent)
	if err != nil {
		t.Fatal(err)
	}
	if second != first {
		t.Fatalf("retry receipt = %+v, want %+v", second, first)
	}
	if got := sess.MessageCountTotal(); got != 1 {
		t.Fatalf("message count = %d, want one canonical message", got)
	}

	reloaded, err := loadSession(sess.filePath)
	if err != nil {
		t.Fatal(err)
	}
	third, err := reloaded.CommitDomainMessage(intent)
	if err != nil {
		t.Fatal(err)
	}
	if third != first || reloaded.MessageCountTotal() != 1 {
		t.Fatalf("reloaded retry = %+v count=%d, want receipt %+v count=1", third, reloaded.MessageCountTotal(), first)
	}
	history := reloaded.History()
	if len(history) != 1 || history[0].ID != first.MessageID || history[0].AgentCommandID != "command-1" || history[0].AgentOperationID != "operation-1" || history[0].AgentCycle != 1 {
		t.Fatalf("history identity was not restored: %+v", history)
	}
}

func TestCommitDomainMessageRejectsIdentityReuseWithDifferentPayload(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.GetOrCreate("domain-conflict")
	if err != nil {
		t.Fatal(err)
	}
	identity := DomainCommitIdentity{CommandID: "command-1", OperationID: "operation-1", Cycle: 1}
	first, _ := NewDomainCommitIntent(identity, agent.AssistantMessage("first", nil), MessageMetadata{})
	second, _ := NewDomainCommitIntent(identity, agent.AssistantMessage("different", nil), MessageMetadata{})
	if _, err := sess.CommitDomainMessage(first); err != nil {
		t.Fatal(err)
	}
	if _, err := sess.CommitDomainMessage(second); !errors.Is(err, ErrDomainCommitIdentityConflict) {
		t.Fatalf("conflicting retry error = %v, want %v", err, ErrDomainCommitIdentityConflict)
	}
	if sess.MessageCountTotal() != 1 {
		t.Fatalf("conflicting retry appended a second message")
	}
}

func TestCommitDomainMessageRejectsSameContentWithDifferentSemanticMetadata(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.GetOrCreate("domain-metadata-conflict")
	if err != nil {
		t.Fatal(err)
	}
	identity := DomainCommitIdentity{CommandID: "command-1", OperationID: "operation-1", Cycle: 1}
	first, err := NewDomainCommitIntent(identity, agent.UserMessage("same content"), MessageMetadata{UserReferences: []agentcontext.UserReference{{Kind: "file", Label: "a.md"}}})
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewDomainCommitIntent(identity, agent.UserMessage("same content"), MessageMetadata{UserReferences: []agentcontext.UserReference{{Kind: "file", Label: "b.md"}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sess.CommitDomainMessage(first); err != nil {
		t.Fatal(err)
	}
	if _, err := sess.CommitDomainMessage(second); !errors.Is(err, ErrDomainCommitIdentityConflict) {
		t.Fatalf("metadata conflict error = %v, want %v", err, ErrDomainCommitIdentityConflict)
	}
}

func TestAgentCanonicalHashPersistsAtomicallyAndReconcilesExactly(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.GetOrCreate("agent-canonical-hash")
	if err != nil {
		t.Fatal(err)
	}
	identity := DomainCommitIdentity{CommandID: "command-1", OperationID: "operation-1", Cycle: 1}
	intent, err := NewDomainCommitIntent(identity, agent.AssistantMessage("answer", nil), MessageMetadata{})
	if err != nil {
		t.Fatal(err)
	}
	intent, err = intent.WithAgentCanonicalHash("agent-output-hash")
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := sess.CommitDomainMessage(intent)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.AgentCanonicalHash != "agent-output-hash" {
		t.Fatalf("receipt canonical hash=%q", receipt.AgentCanonicalHash)
	}
	reopened, err := loadSession(sess.filePath)
	if err != nil {
		t.Fatal(err)
	}
	found, ok, err := reopened.FindAgentCanonicalCommit(identity, agent.Assistant, "agent-output-hash")
	if err != nil || !ok || found.MessageID != receipt.MessageID {
		t.Fatalf("canonical lookup receipt=%#v found=%t error=%v", found, ok, err)
	}
	if _, _, err := reopened.FindAgentCanonicalCommit(identity, agent.Assistant, "different-hash"); !errors.Is(err, ErrDomainCommitIdentityConflict) {
		t.Fatalf("mismatched canonical lookup error=%v", err)
	}
	conflict := intent
	conflict.Metadata.AgentCanonicalHash = "different-hash"
	if _, err := reopened.CommitDomainMessage(conflict); !errors.Is(err, ErrDomainCommitIdentityConflict) {
		t.Fatalf("mismatched canonical retry error=%v", err)
	}
}

func TestContextCursorRejectsStaleStructuralMutation(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.GetOrCreate("context-cursor")
	if err != nil {
		t.Fatal(err)
	}
	stale := sess.ContextCursor()
	if err := sess.Append(agent.UserMessage("new turn")); err != nil {
		t.Fatal(err)
	}
	if err := sess.AppendClearMarkerAt(stale); !errors.Is(err, ErrContextRevisionConflict) {
		t.Fatalf("stale clear error = %v, want %v", err, ErrContextRevisionConflict)
	}
	current := sess.ContextCursor()
	if err := sess.AppendClearMarkerAt(current); err != nil {
		t.Fatal(err)
	}
	if after := sess.ContextCursor(); after.Revision != current.Revision+1 {
		t.Fatalf("revision after clear = %d, want %d", after.Revision, current.Revision+1)
	}
}
