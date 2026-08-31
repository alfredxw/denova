package agentchat

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"denova/config"
	"denova/internal/agents/conversationconfig"
	"denova/internal/agents/session"
	projectdomain "denova/internal/project"
)

func TestProjectsAndHistoryShareEveryProjectSession(t *testing.T) {
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
	ordinary, err := store.Create("Ordinary conversation")
	if err != nil {
		t.Fatal(err)
	}
	seed := conversationconfig.Config{
		AgentKind:     config.AgentKindGeneral,
		ProfileID:     "default",
		ThinkingLevel: "medium",
		ApprovalMode:  config.AgentApprovalWrite,
	}
	configured, err := store.CreateWithRuntimeConfig("Configured conversation", seed)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	service := NewService(activeViewTestHost{}, registry)
	t.Cleanup(func() { service.Close(context.Background()) })

	projects := service.Projects()
	assertProjectSessionIDs(t, projects, ordinary.ID, configured.ID)

	history := service.History(HistoryQuery{Limit: 10})
	if history.Total != 2 || len(history.Items) != 2 {
		t.Fatalf("history = %#v", history)
	}
	historyIDs := map[string]bool{}
	for _, item := range history.Items {
		historyIDs[item.Session.ID] = true
	}
	if !historyIDs[ordinary.ID] || !historyIDs[configured.ID] {
		t.Fatalf("history session IDs = %#v", historyIDs)
	}
}

func assertProjectSessionIDs(t *testing.T, projects []Project, wantIDs ...string) {
	t.Helper()
	if len(projects) != 1 || projects[0].Total != len(wantIDs) || len(projects[0].Sessions) != len(wantIDs) {
		t.Fatalf("projects = %#v", projects)
	}
	gotIDs := map[string]bool{}
	for _, conversation := range projects[0].Sessions {
		gotIDs[conversation.ID] = true
	}
	for _, wantID := range wantIDs {
		if !gotIDs[wantID] {
			t.Fatalf("project session IDs = %#v, missing %q", gotIDs, wantID)
		}
	}
}
