package app

import (
	"context"
	agentchat "denova/internal/agents/chat"
	agentharness "denova/internal/agents/harness"
	"os"
	"path/filepath"
	"testing"

	"denova/config"
	agentrun "denova/internal/agents/run"
	"denova/internal/agents/session"
	"denova/internal/book"
	"denova/internal/interactive"
	projectdomain "denova/internal/project"
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
	request := agentharness.InputMaterializationRequest{
		Binding: agentrun.RuntimeBinding{AgentKind: agentrun.AgentKindIDE, Workspace: workspace, SessionID: "accepted-writing"},
		Identity: agentrun.CycleIdentity{
			CommandID: "writing-command", OperationID: "writing-operation", Cycle: 1,
		},
		AgentKind: config.AgentKindIDE,
		Message:   "write this chapter",
		Request: agentchat.ChatRequest{
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

func TestAppMaterializesGeneralProjectInputInUserState(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	workspace := filepath.Join(t.TempDir(), "general-project")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	registry := projectdomain.NewRegistry(dataDir)
	record, err := registry.Add(workspace, projectdomain.TypeGeneral, "General")
	if err != nil {
		t.Fatal(err)
	}
	layout, err := registry.EnsureState(record)
	if err != nil {
		t.Fatal(err)
	}
	store, err := session.NewStore(layout.SessionsDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetOrCreate("general-session"); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	request := agentharness.InputMaterializationRequest{
		Binding: agentrun.RuntimeBinding{
			AgentKind: agentrun.AgentKindGeneral, ProjectID: record.ID,
			Mode: "agent_chat", SessionID: "general-session",
		},
		Identity: agentrun.CycleIdentity{
			CommandID: "general-command", OperationID: "general-operation", Cycle: 1,
		},
		AgentKind: agentrun.AgentKindGeneral,
		Message:   "inspect the repository",
		Request:   agentchat.ChatRequest{Message: "inspect the repository"},
	}
	application := &App{
		cfg: &config.Config{NovaDir: dataDir}, projectRegistry: registry,
	}
	plan, err := application.PlanHarnessInputMaterialization(context.Background(), request)
	if err != nil || !plan.Required || plan.Hash == "" {
		t.Fatalf("General input plan = %#v err=%v", plan, err)
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
		t.Fatalf("idempotent General receipts first=%#v second=%#v", first, second)
	}

	reloaded, err := session.NewStore(layout.SessionsDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reloaded.Close() })
	sess, err := reloaded.Get("general-session")
	if err != nil {
		t.Fatal(err)
	}
	if history := sess.History(); len(history) != 1 || history[0].Content != request.Message {
		t.Fatalf("General Project history = %#v", history)
	}
	if _, err := os.Stat(book.NewState(workspace).SessionDir()); !os.IsNotExist(err) {
		t.Fatalf("workspace-private session directory should not be created, err=%v", err)
	}

	result, err := application.reconcileHarnessDomainCommit(
		context.Background(),
		domainRecoveryRequest(
			request.Binding, string(request.Identity.CommandID), string(request.Identity.OperationID),
			request.Identity.Cycle, agentrun.DomainCommitInput, plan.Hash,
		),
	)
	if err != nil || !result.Found || result.Revision != first.Revision {
		t.Fatalf("General Project input reconciliation = %#v err=%v", result, err)
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
	request := agentharness.InputMaterializationRequest{
		Binding: agentrun.RuntimeBinding{AgentKind: agentrun.AgentKindInteractiveStory, Workspace: workspace, StoryID: story.ID, BranchID: "main"},
		Identity: agentrun.CycleIdentity{
			CommandID: "game-command", OperationID: "game-operation", Cycle: 1,
		},
		AgentKind: config.AgentKindInteractiveStory,
		Message:   "open the sealed door",
		Request:   agentchat.ChatRequest{Message: "open the sealed door"},
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
