package agentchat

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"denova/config"
	agents "denova/internal/agents"
	agentexecution "denova/internal/agents/execution"
	"denova/internal/agents/session"
	agenttool "denova/internal/agents/tool"
	"denova/internal/book"
	projectdomain "denova/internal/project"
)

type activeViewTestHost struct{}

func (activeViewTestHost) BaseRuntime() (config.Config, *agentexecution.Runtime) {
	return config.Config{}, nil
}

func (activeViewTestHost) ProjectVersionService(string) (*book.VersionService, error) {
	return nil, nil
}

func (activeViewTestHost) CurrentWorkspace() string { return "" }

func (activeViewTestHost) OnVerifiedMutations(context.Context, string, *book.VersionService, config.Config, []agenttool.Mutation, agenttool.Verification) {
}

func (activeViewTestHost) ProjectAgentHostCapabilities(context.Context, projectdomain.Type, *config.Config, string) (agents.AgentHostCapabilities, error) {
	return agents.AgentHostCapabilities{}, nil
}

func TestActiveViewProjectsPendingInterruptionForIdleAgentChat(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	registry := projectdomain.NewRegistry(filepath.Join(root, "denova"))
	record, err := registry.Add(workspace, projectdomain.TypeGeneral, "")
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
	t.Cleanup(func() { _ = store.Close() })
	sess, err := store.GetOrCreate("session-1")
	if err != nil {
		t.Fatal(err)
	}
	if err := sess.MarkInterrupted("Draft a scene", "Partial scene", "user_requested"); err != nil {
		t.Fatal(err)
	}
	pending := sess.PendingInterruption()
	if pending == nil {
		t.Fatal("expected pending interruption fixture")
	}

	service := NewService(activeViewTestHost{}, registry)
	service.projects[record.ID] = &projectRuntime{
		projectID: record.ID, projectType: projectdomain.TypeGeneral,
		agentKind: "general", stateRoot: layout.StateRoot, workspace: workspace,
		store: store,
	}

	view := service.ActiveView(context.Background(), Binding{ProjectID: record.ID, SessionID: sess.ID})
	if view.PendingInterruptionID != pending.ID {
		t.Fatalf("pending interruption id = %q, want %q", view.PendingInterruptionID, pending.ID)
	}
}
