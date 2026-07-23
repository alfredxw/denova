package app

import (
	"context"
	"path/filepath"
	"testing"

	"denova/config"
	"denova/internal/agent"
	"denova/internal/agentruntime"
	"denova/internal/book"
	"denova/internal/interactive"
	"denova/internal/session"
)

func TestAppMaterializesAcceptedWritingInputExactlyOnce(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	workspace := filepath.Join(root, "book")
	state := book.NewState(workspace)
	if err := state.InitWorkspace(); err != nil {
		t.Fatal(err)
	}
	store, err := session.NewStore(state.SessionDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetOrCreate("accepted-writing"); err != nil {
		t.Fatal(err)
	}
	request := agent.HarnessInputMaterializationRequest{
		Binding: agentruntime.BindingRef{
			Kind: agentruntime.BindingWriting, Profile: agentruntime.ProfileWriting,
			Workspace: workspace, SessionID: "accepted-writing",
		},
		Identity: agent.HarnessCycleIdentity{
			CommandID: "writing-command", OperationID: "writing-operation", Cycle: 1,
		},
		AgentKind: config.AgentKindIDE,
		Message:   "write this chapter",
		Request: agent.ChatRequest{
			Message: "write this chapter", References: []string{"chapters/one.md"},
		},
	}
	application := &App{cfg: &config.Config{NovaDir: root}}
	plan, err := application.PlanHarnessInputMaterialization(context.Background(), request)
	if err != nil || !plan.Required || plan.Hash == "" {
		t.Fatalf("writing input plan = %#v err=%v", plan, err)
	}
	first, err := application.MaterializeHarnessInput(context.Background(), request, plan)
	if err != nil {
		t.Fatal(err)
	}
	second, err := application.MaterializeHarnessInput(context.Background(), request, plan)
	if err != nil {
		t.Fatal(err)
	}
	if first.Revision == "" || second != first {
		t.Fatalf("idempotent writing receipts first=%#v second=%#v", first, second)
	}
	reloadedStore, err := session.NewStore(state.SessionDir())
	if err != nil {
		t.Fatal(err)
	}
	reloaded, err := reloadedStore.Get("accepted-writing")
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.MessageCountTotal() != 1 {
		t.Fatalf("canonical writing inputs = %d, want exactly one", reloaded.MessageCountTotal())
	}
	history := reloaded.History()
	if len(history) != 1 || history[0].Role != "user" || history[0].Content != request.Message ||
		history[0].AgentCommandID != "writing-command" || len(history[0].UserReferences) != 1 {
		t.Fatalf("canonical writing input = %#v", history)
	}
}

func TestAppMaterializesAcceptedGameInputAsPendingWithoutNarrative(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	store := interactive.NewStore(workspace)
	story, err := store.CreateStory(interactive.CreateStoryRequest{Title: "accepted input", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	request := agent.HarnessInputMaterializationRequest{
		Binding: agentruntime.BindingRef{
			Kind: agentruntime.BindingGame, Profile: agentruntime.ProfileGame,
			Workspace: workspace, StoryID: story.ID, BranchID: "main",
		},
		Identity: agent.HarnessCycleIdentity{
			CommandID: "game-command", OperationID: "game-operation", Cycle: 1,
		},
		AgentKind: config.AgentKindInteractiveStory,
		Message:   "open the sealed door",
		Request:   agent.ChatRequest{Message: "open the sealed door"},
	}
	application := &App{}
	plan, err := application.PlanHarnessInputMaterialization(context.Background(), request)
	if err != nil || !plan.Required || plan.Hash == "" {
		t.Fatalf("game input plan = %#v err=%v", plan, err)
	}
	first, err := application.MaterializeHarnessInput(context.Background(), request, plan)
	if err != nil {
		t.Fatal(err)
	}
	second, err := application.MaterializeHarnessInput(context.Background(), request, plan)
	if err != nil {
		t.Fatal(err)
	}
	if first.Revision == "" || second != first {
		t.Fatalf("idempotent game receipts first=%#v second=%#v", first, second)
	}
	snapshot, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.PendingPlayerInputs) != 1 || len(snapshot.Turns) != 0 || snapshot.CurrentTurn != nil {
		t.Fatalf("accepted game input should be pending without invented narrative: %#v", snapshot)
	}
	if snapshot.PendingPlayerInputs[0].ID != first.Revision || snapshot.PendingPlayerInputs[0].AgentCommitHash != plan.Hash {
		t.Fatalf("pending game input = %#v, receipt=%#v", snapshot.PendingPlayerInputs[0], first)
	}
}
