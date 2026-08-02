package app

import (
	"context"
	agentharness "denova/internal/agents/harness"
	agentrun "denova/internal/agents/run"
	"testing"

	"denova/config"
	agents "denova/internal/agents"
	agentcompaction "denova/internal/agents/context/compaction"
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
		CompactionCheckpoint: agentcompaction.NewCheckpoint(config.AgentKindIDE, agentcompaction.Result{Summary: "checkpoint"}),
		SourceEndIndex:       1,
		SourceMessageCount:   1,
	})
	if err != nil {
		t.Fatal(err)
	}
	chat := agentharness.NewEphemeralService()
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
	status, err := chat.RuntimeStatusProjection(context.Background(), agentrun.Options{
		AgentKind: agentrun.AgentKindIDE, Workspace: "/book", SessionID: sess.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if status.Phase != agentrun.RunPhaseIdle || status.LastOperation == nil || status.LastOperation.Status != agentrun.OperationSucceeded ||
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
		CompactionCheckpoint: agentcompaction.NewCheckpoint(config.AgentKindInteractiveStory, agentcompaction.Result{Summary: "钟楼响起"}),
		SourceTurnCount:      1,
		ExpectedParentID:     &expected,
	})
	if err != nil {
		t.Fatal(err)
	}
	chat := agentharness.NewEphemeralService()
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
	status, err := chat.RuntimeStatusProjection(context.Background(), agentrun.Options{
		AgentKind: agentrun.AgentKindInteractiveStory, Workspace: workspace, StoryID: story.ID, BranchID: "main",
	})
	if err != nil {
		t.Fatal(err)
	}
	if status.Phase != agentrun.RunPhaseIdle || status.LastOperation == nil || status.LastOperation.Status != agentrun.OperationSucceeded ||
		status.LastDomainCommit == nil || status.LastDomainCommit.Revision == "" {
		t.Fatalf("durable game structural settlement missing: %#v", status)
	}
}
