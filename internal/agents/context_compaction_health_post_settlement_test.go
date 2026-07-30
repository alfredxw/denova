package agents

import (
	"context"
	"errors"
	"testing"

	agent "github.com/alfredxw/denova/agent"

	"denova/config"
	"denova/internal/agents/session"
)

func TestSessionCompactionHealthCommitsAfterOwningTurnAndRequiresCheckpointPublication(t *testing.T) {
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	sess, err := store.Create("post-settlement health")
	if err != nil {
		t.Fatal(err)
	}
	conversation := NewSessionConversationForAgent(sess, &config.Config{}, config.AgentKindIDE)
	result := ContextCompactionResult{Phase: contextCompactionPhaseModelStep}
	conversation.stageSessionCompactionHealth(sess.ContextCursor(), "stable-structure", ContextCompactionHealthSuccess, &result)
	if err := sess.Append(agent.AssistantMessage("owning turn settled", nil)); err != nil {
		t.Fatal(err)
	}
	settledRevision := sess.ContextCursor().Revision
	if err := conversation.CommitPostSettlementContextCompactionHealth(
		context.Background(), OperationID("operation-publish-failed"),
		ContextCompactionPublication{Attempted: true, Err: errors.New("journal unavailable")},
	); err != nil {
		t.Fatal(err)
	}
	health, ok := sess.LatestContextCompactionHealth(config.AgentKindIDE)
	if !ok || health.Outcome != string(ContextCompactionHealthFailure) ||
		health.FailureCode != "checkpoint_publish_failed" || health.ConsecutiveFailures != 1 ||
		health.BasisRevision != settledRevision {
		t.Fatalf("publish failure health = %#v ok=%t settled_revision=%d", health, ok, settledRevision)
	}

	conversation.stageSessionCompactionHealth(sess.ContextCursor(), "stable-structure", ContextCompactionHealthSuccess, &result)
	if err := conversation.CommitPostSettlementContextCompactionHealth(
		context.Background(), OperationID("operation-publish-succeeded"), ContextCompactionPublication{Attempted: true},
	); err != nil {
		t.Fatal(err)
	}
	health, ok = sess.LatestContextCompactionHealth(config.AgentKindIDE)
	if !ok || health.Outcome != string(ContextCompactionHealthSuccess) || health.ConsecutiveFailures != 0 {
		t.Fatalf("published checkpoint did not reset health: %#v ok=%t", health, ok)
	}
}
