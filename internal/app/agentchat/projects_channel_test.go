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

func TestProjectsAndHistoryKeepSessionChannelsIndependent(t *testing.T) {
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
	configuration, err := store.CreateWithRuntimeConfig("Configuration conversation", seed, session.ChannelConfiguration)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	service := NewService(activeViewTestHost{}, registry)
	t.Cleanup(func() { service.Close(context.Background()) })

	agentProjects := service.Projects(session.ChannelAgent)
	assertSingleChannelSession(t, agentProjects, ordinary.ID, session.ChannelAgent)
	configurationProjects := service.Projects(session.ChannelConfiguration)
	assertSingleChannelSession(t, configurationProjects, configuration.ID, session.ChannelConfiguration)

	agentHistory := service.History(HistoryQuery{Channel: session.ChannelAgent, Limit: 10})
	if agentHistory.Total != 1 || agentHistory.Items[0].Session.ID != ordinary.ID {
		t.Fatalf("ordinary history = %#v", agentHistory)
	}
	configurationHistory := service.History(HistoryQuery{Channel: session.ChannelConfiguration, Limit: 10})
	if configurationHistory.Total != 1 || configurationHistory.Items[0].Session.ID != configuration.ID {
		t.Fatalf("configuration history = %#v", configurationHistory)
	}
}

func assertSingleChannelSession(t *testing.T, projects []Project, wantID string, wantChannel session.Channel) {
	t.Helper()
	if len(projects) != 1 || projects[0].Total != 1 || len(projects[0].Sessions) != 1 {
		t.Fatalf("projects = %#v", projects)
	}
	got := projects[0].Sessions[0]
	if got.ID != wantID || got.Channel != wantChannel {
		t.Fatalf("session = %#v, want id=%q channel=%q", got, wantID, wantChannel)
	}
}
