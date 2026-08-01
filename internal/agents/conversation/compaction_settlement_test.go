package conversation

import (
	"context"
	"testing"

	agent "github.com/alfredxw/denova/agent"

	"denova/config"
	agentcompaction "denova/internal/agents/context/compaction"
	agentstructural "denova/internal/agents/context/structural"
	agentrun "denova/internal/agents/run"
	"denova/internal/agents/session"
)

func TestSessionPostSettlementCompactionPublishesAfterAssistantCursor(t *testing.T) {
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.Create("post settlement compaction")
	if err != nil {
		t.Fatal(err)
	}
	if err := sess.Append(agent.UserMessage("old user")); err != nil {
		t.Fatal(err)
	}
	conversation := NewSessionConversationForAgent(sess, &config.Config{}, agentrun.AgentKindIDE)
	conversation.stagePreparedSessionCompaction(preparedSessionContextCompaction{
		Result: agentcompaction.Result{
			Triggered: true, Phase: agentcompaction.PhaseMidRun, Epoch: 1, Summary: "bounded old history",
			SourceMessageCount: 1, RetainedTurns: 2,
		},
		SourceStartIndex: 0, SourceEndIndex: 1,
	})
	if err := sess.Append(agent.AssistantMessage("settled assistant", nil)); err != nil {
		t.Fatal(err)
	}
	spec, err := conversation.PostSettlementContextStructuralSpec(context.Background(), "settled-operation", agentrun.Options{
		AgentKind: agentrun.AgentKindIDE, Workspace: "/book", SessionID: sess.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if spec == nil {
		t.Fatal("expected staged post-settlement structural spec")
	}
	if spec.RestorePlan == nil || spec.RestorePlan.Domain != agentstructural.DomainSession ||
		spec.RestorePlan.RecordID == "" || spec.RestorePlan.IntentHash == "" || len(spec.RestorePlan.Mutation) == 0 {
		t.Fatalf("post-settlement Session compaction has no exact restore plan: %#v", spec.RestorePlan)
	}
	identity := agentstructural.Identity{CommandID: agentrun.CommandID(spec.CommandID), OperationID: "publish-operation", Cycle: 1}
	intent, err := spec.Operation.Prepare(context.Background(), identity, nil)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := spec.Operation.Commit(context.Background(), identity, intent)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Revision == "" {
		t.Fatal("post-settlement compaction has no durable revision")
	}
	record, ok := sess.LatestContextCompaction(agentrun.AgentKindIDE)
	if !ok || record.SourceEndIndex != 1 || record.ContextRevision != sess.ContextCursor().Revision {
		t.Fatalf("post-settlement checkpoint = %#v ok=%t", record, ok)
	}
	if got := sess.GetEffectiveMessages(); len(got) != 2 {
		t.Fatalf("structural checkpoint modified raw display history: %#v", got)
	}
}
