package app

import (
	"context"
	"testing"

	"denova/config"
	agents "denova/internal/agents"
	"denova/internal/agents/session"
	"denova/internal/interactive"
)

func TestWritingCompactionRemovalUsesDurableStructuralCommand(t *testing.T) {
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.Create("durable compaction removal")
	if err != nil {
		t.Fatal(err)
	}
	if err := sess.Append(agents.UserMessage("raw history remains canonical")); err != nil {
		t.Fatal(err)
	}
	compaction, err := sess.AppendContextCompaction(session.ContextCompaction{
		AgentKind: config.AgentKindIDE, Summary: "checkpoint", SourceEndIndex: 1, SourceMessageCount: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	chat := agents.NewEphemeralChatService()
	t.Cleanup(func() { _ = chat.Close(context.Background()) })
	application := &App{
		workspace: "/book", workspaceGeneration: 1, sessionStore: store,
		session: sess, chatService: chat,
	}
	service := &ChatAppService{app: application}
	removed, err := service.executeWritingContextCompactionRemoval(context.Background(), "client-remove-writing-1")
	if err != nil || !removed {
		t.Fatalf("durable removal: removed=%t err=%v", removed, err)
	}
	replayed, err := service.executeWritingContextCompactionRemoval(context.Background(), "client-remove-writing-1")
	if err != nil || !replayed {
		t.Fatalf("idempotent removal replay: removed=%t err=%v", replayed, err)
	}
	if _, ok := sess.LatestContextCompaction(config.AgentKindIDE); ok {
		t.Fatal("removed compaction remains active")
	}
	marker, ok := sess.LatestContextCompactionRemoval(config.AgentKindIDE)
	if !ok || marker.CompactionID != compaction.ID {
		t.Fatalf("missing canonical removal marker: %#v", marker)
	}
	status, err := chat.RuntimeStatusProjection(context.Background(), agents.RunOptions{
		AgentKind: agents.AgentKindIDE, Workspace: "/book", SessionID: sess.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if status.Phase != agents.RunPhaseIdle || status.LastOperation == nil || status.LastOperation.Status != agents.OperationSucceeded ||
		status.LastDomainCommit == nil || status.LastDomainCommit.Revision == "" {
		t.Fatalf("durable structural settlement missing: %#v", status)
	}
}

func TestInteractiveCompactionRemovalUsesDurableStructuralCommand(t *testing.T) {
	workspace := t.TempDir()
	store := interactive.NewStore(workspace)
	story, err := store.CreateStory(interactive.CreateStoryRequest{Title: "durable game compaction", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	turn, err := store.AppendTurn(story.ID, interactive.AppendTurnRequest{BranchID: "main", User: "观察", Narrative: "钟楼响起。"})
	if err != nil {
		t.Fatal(err)
	}
	expected := turn.ID
	compaction, err := store.AppendContextCompaction(story.ID, "main", interactive.ContextCompactionEvent{
		AgentKind: config.AgentKindInteractiveStory, Summary: "钟楼响起", SourceTurnCount: 1, ExpectedParentID: &expected,
	})
	if err != nil {
		t.Fatal(err)
	}
	chat := agents.NewEphemeralChatService()
	t.Cleanup(func() { _ = chat.Close(context.Background()) })
	application := &App{workspace: workspace, workspaceGeneration: 1, interactive: store, chatService: chat}
	service := &InteractiveAppService{app: application}
	removed, err := service.executeInteractiveContextCompactionRemoval(context.Background(), story.ID, "main", "client-remove-game-1")
	if err != nil || !removed {
		t.Fatalf("durable interactive removal: removed=%t err=%v", removed, err)
	}
	replayed, err := service.executeInteractiveContextCompactionRemoval(context.Background(), story.ID, "main", "client-remove-game-1")
	if err != nil || !replayed {
		t.Fatalf("idempotent interactive removal replay: removed=%t err=%v", replayed, err)
	}
	snapshot, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.ContextCompaction != nil || snapshot.ContextCompactionRemoval == nil || snapshot.ContextCompactionRemoval.CompactionID != compaction.ID {
		t.Fatalf("canonical interactive removal missing: %#v", snapshot)
	}
	status, err := chat.RuntimeStatusProjection(context.Background(), agents.RunOptions{
		AgentKind: agents.AgentKindInteractiveStory, Workspace: workspace, StoryID: story.ID, BranchID: "main",
	})
	if err != nil {
		t.Fatal(err)
	}
	if status.Phase != agents.RunPhaseIdle || status.LastOperation == nil || status.LastOperation.Status != agents.OperationSucceeded ||
		status.LastDomainCommit == nil || status.LastDomainCommit.Revision == "" {
		t.Fatalf("durable game structural settlement missing: %#v", status)
	}
}

func TestInteractivePostSettlementCompactionPublishesAtSettledTurnHead(t *testing.T) {
	workspace := t.TempDir()
	store := interactive.NewStore(workspace)
	story, err := store.CreateStory(interactive.CreateStoryRequest{Title: "post settlement game compaction", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.AppendTurn(story.ID, interactive.AppendTurnRequest{
		BranchID: "main", User: "观察大厅", Narrative: "大厅中央悬着一盏旧灯。",
	}); err != nil {
		t.Fatal(err)
	}
	conversation := newInteractiveConversation(store, "", workspace, story.ID, "main", "", 0, &config.Config{})
	conversation.stagePreparedInteractiveCompaction(preparedInteractiveContextCompaction{
		Result: agents.ContextCompactionResult{
			Triggered: true, Phase: "mid_run", Epoch: 1, Summary: "大厅中央有一盏旧灯。",
			SourceMessageCount: 2, RetainedTurns: 2,
		},
		SourceTurnCount: 1,
	})
	settledTurn, err := store.AppendTurn(story.ID, interactive.AppendTurnRequest{
		BranchID: "main", User: "触碰旧灯", Narrative: "灯芯亮起微弱的蓝光。",
	})
	if err != nil {
		t.Fatal(err)
	}
	spec, err := conversation.PostSettlementContextStructuralSpec(context.Background(), "settled-game-operation", agents.RunOptions{
		AgentKind: agents.AgentKindInteractiveStory, Workspace: workspace, StoryID: story.ID, BranchID: "main",
	})
	if err != nil {
		t.Fatal(err)
	}
	if spec == nil {
		t.Fatal("expected staged post-settlement structural spec")
	}
	if spec.RestorePlan == nil || spec.RestorePlan.Domain != agents.ContextStructuralDomainStory ||
		spec.RestorePlan.RecordID == "" || spec.RestorePlan.IntentHash == "" || len(spec.RestorePlan.Mutation) == 0 {
		t.Fatalf("post-settlement Story compaction has no exact restore plan: %#v", spec.RestorePlan)
	}
	chat := agents.NewEphemeralChatService()
	t.Cleanup(func() { _ = chat.Close(context.Background()) })
	result, err := chat.ExecuteContextStructuralOperation(context.Background(), *spec)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Compaction.Triggered {
		t.Fatalf("unexpected compaction result: %#v", result)
	}
	snapshot, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.ContextCompaction == nil {
		t.Fatal("post-settlement checkpoint was not persisted")
	}
	if snapshot.ContextCompaction.ParentID != settledTurn.ID || snapshot.ContextCompaction.SourceTurnCount != 1 {
		t.Fatalf("post-settlement checkpoint = %#v, settled turn = %#v", snapshot.ContextCompaction, settledTurn)
	}
	if len(snapshot.Turns) != 2 {
		t.Fatalf("structural checkpoint changed raw story history: %#v", snapshot.Turns)
	}
}
