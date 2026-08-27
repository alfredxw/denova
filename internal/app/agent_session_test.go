package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"denova/config"
	agents "denova/internal/agents"
	agentrun "denova/internal/agents/run"
	"denova/internal/agents/session"
	configmanagerapp "denova/internal/app/configmanager"
	projectdomain "denova/internal/project"
)

func TestBoundedGlobalRunCatalogKeepsExplicitTarget(t *testing.T) {
	runs := []GlobalAgentRunTraceSummary{
		{RunTraceSummary: agentrun.RunTraceSummary{ID: "newest"}, ProjectID: "project-new"},
		{RunTraceSummary: agentrun.RunTraceSummary{ID: "target"}, ProjectID: "project-old"},
	}

	bounded := boundedGlobalRunCatalog(runs, 1, GlobalAgentRunTraceTarget{ProjectID: "project-old", RunID: "target"})

	if len(bounded) != 1 || bounded[0].ProjectID != "project-old" || bounded[0].ID != "target" {
		t.Fatalf("bounded global Run catalog = %#v, want explicit target", bounded)
	}
}

func TestGlobalAgentRunTracesReadsTargetPastPerProjectLimit(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "project")
	if err := os.MkdirAll(workspace, 0o700); err != nil {
		t.Fatal(err)
	}
	registry := projectdomain.NewRegistry(filepath.Join(root, "denova"))
	record, err := registry.Add(workspace, projectdomain.TypeGeneral, "Project")
	if err != nil {
		t.Fatal(err)
	}
	layout, err := registry.EnsureState(record)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(layout.RunsDir(), 0o700); err != nil {
		t.Fatal(err)
	}

	writeRun := func(runID, createdAt string, modifiedAt time.Time) {
		t.Helper()
		payload := strings.Join([]string{
			`{"type":"run_created","run_id":"` + runID + `","created_at":"` + createdAt + `","data":{"agent_kind":"ide"}}`,
			`{"type":"run_finished","run_id":"` + runID + `","created_at":"` + createdAt + `","data":{"status":"success"}}`,
		}, "\n") + "\n"
		path := filepath.Join(layout.RunsDir(), runID+".jsonl")
		if err := os.WriteFile(path, []byte(payload), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(path, modifiedAt, modifiedAt); err != nil {
			t.Fatal(err)
		}
	}
	baseTime := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	writeRun("target", "2026-08-26T12:00:00Z", baseTime)
	writeRun("newest", "2026-08-27T12:00:00Z", baseTime.Add(time.Hour))

	application := &App{
		cfg:             &config.Config{Labs: config.ResolvedLabs{DeveloperMode: true}},
		projectRegistry: registry,
	}
	catalog, err := application.GlobalAgentRunTraces(context.Background(), 1, GlobalAgentRunTraceTarget{
		ProjectID: record.ID,
		RunID:     "target",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog.Runs) != 1 || catalog.Runs[0].ProjectID != record.ID || catalog.Runs[0].ID != "target" {
		t.Fatalf("targeted global Run catalog = %#v, want explicit target", catalog.Runs)
	}
}

func TestAgentRunTracesUseProjectStateRoot(t *testing.T) {
	workspace := t.TempDir()
	stateRoot := t.TempDir()
	application := &App{
		workspace: workspace,
		cfg:       &config.Config{ProjectStateDir: stateRoot},
	}
	runID := "run-project-state"
	payload := strings.Join([]string{
		`{"type":"run_created","run_id":"run-project-state","created_at":"2026-08-02T12:25:04Z","data":{"agent_kind":"ide"}}`,
		`{"type":"run_finished","run_id":"run-project-state","created_at":"2026-08-02T12:35:38Z","data":{"status":"success"}}`,
	}, "\n") + "\n"
	runsDir := filepath.Join(stateRoot, "runs")
	if err := os.MkdirAll(runsDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runsDir, runID+".jsonl"), []byte(payload), 0o600); err != nil {
		t.Fatal(err)
	}

	summaries, err := application.AgentRunTraces(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(summaries) != 1 || summaries[0].ID != runID || summaries[0].Status != "success" {
		t.Fatalf("project-state trace summaries = %#v", summaries)
	}
	trace, err := application.AgentRunTrace(runID)
	if err != nil {
		t.Fatal(err)
	}
	if trace.Summary.ID != runID || len(trace.Records) != 2 {
		t.Fatalf("project-state trace detail = %#v", trace)
	}
	export, err := application.ExportAgentRunTrace(runID)
	if err != nil {
		t.Fatal(err)
	}
	if string(export.Data) != payload {
		t.Fatalf("project-state trace export = %q, want %q", string(export.Data), payload)
	}
}

func TestAgentSessionIDCoversBuiltInModelAgents(t *testing.T) {
	for _, agentKind := range persistentAgentKinds() {
		id, ok := agentSessionID(agentKind)
		if !ok || id == "" {
			t.Fatalf("agent %s should have a persistent session id", agentKind)
		}
	}
}

func TestPersistAgentCallInStoreWritesFullMessages(t *testing.T) {
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	longInput := strings.Repeat("输入", 7000)
	longOutput := strings.Repeat("输出", 5000)

	if err := persistAgentCallInStore(store, config.AgentKindVersionSummary, longInput, longOutput); err != nil {
		t.Fatal(err)
	}

	sess, err := agentSessionFromStore(store, config.AgentKindVersionSummary)
	if err != nil {
		t.Fatal(err)
	}
	history := sess.History()
	if len(history) != 2 {
		t.Fatalf("history length = %d, want 2", len(history))
	}
	if history[0].Role != "user" || history[1].Role != "assistant" {
		t.Fatalf("unexpected roles: %#v", history)
	}
	if history[0].Content != longInput || history[1].Content != longOutput {
		t.Fatalf("expected full persisted messages")
	}
	if sess.MessageCount() != 2 {
		t.Fatalf("message count = %d, want 2", sess.MessageCount())
	}
}

func TestClearAgentSessionInStoreMarksEffectiveContextForEveryBuiltInAgent(t *testing.T) {
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, agentKind := range persistentAgentKinds() {
		if err := persistAgentCallInStore(store, agentKind, "清理前", "旧输出"); err != nil {
			t.Fatalf("persist before clear %s: %v", agentKind, err)
		}
		if err := clearAgentSessionInStore(store, agentKind); err != nil {
			t.Fatalf("clear %s: %v", agentKind, err)
		}
		if err := persistAgentCallInStore(store, agentKind, "清理后", "新输出"); err != nil {
			t.Fatalf("persist after clear %s: %v", agentKind, err)
		}
		sess, err := agentSessionFromStore(store, agentKind)
		if err != nil {
			t.Fatal(err)
		}
		effective := sess.GetEffectiveMessages()
		if len(effective) != 2 || effective[0].Content != "清理后" || effective[1].Content != "新输出" {
			t.Fatalf("agent %s effective messages should only include messages after clear: %#v", agentKind, effective)
		}
		history := sess.History()
		hasClear := false
		for _, entry := range history {
			if entry.Type == "clear" {
				hasClear = true
				break
			}
		}
		if !hasClear {
			t.Fatalf("agent %s history should keep clear marker: %#v", agentKind, history)
		}
	}
}

func TestConfigManagerScopedSessionsAreIsolated(t *testing.T) {
	root := t.TempDir()
	application, err := New(context.Background(), &config.Config{
		OpenAIModel: "test-model", NovaDir: root, Workspace: root, ResumeLastWorkspace: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(application.Close)
	projectID := application.ProjectID()
	automationReq := configmanagerapp.Request{ProjectID: projectID, Origin: "automation", ResourceID: "daily-review"}
	loreReq := configmanagerapp.Request{ProjectID: projectID, Origin: "lore", ResourceID: "__config_manager_lore__"}

	automationID, err := configmanagerapp.SessionID(automationReq)
	if err != nil {
		t.Fatal(err)
	}
	loreID, err := configmanagerapp.SessionID(loreReq)
	if err != nil {
		t.Fatal(err)
	}
	if automationID == loreID {
		t.Fatalf("scoped config manager sessions should differ: %s", automationID)
	}
	runtime, err := application.AgentChat().ProjectRuntime(context.Background(), projectID)
	if err != nil {
		t.Fatal(err)
	}
	automationSession, err := runtime.SessionStore.GetOrCreate(automationID)
	if err != nil {
		t.Fatal(err)
	}
	if err := automationSession.Append(agents.UserMessage("自动化配置")); err != nil {
		t.Fatal(err)
	}
	loreSession, err := runtime.SessionStore.GetOrCreate(loreID)
	if err != nil {
		t.Fatal(err)
	}
	if err := loreSession.Append(agents.UserMessage("资料库配置")); err != nil {
		t.Fatal(err)
	}

	automationHistory, err := application.ConfigManager().Messages(automationReq)
	if err != nil {
		t.Fatal(err)
	}
	if len(automationHistory) != 1 || automationHistory[0].Content != "自动化配置" {
		t.Fatalf("automation history should stay scoped: %#v", automationHistory)
	}
	loreHistory, err := application.ConfigManager().Messages(loreReq)
	if err != nil {
		t.Fatal(err)
	}
	if len(loreHistory) != 1 || loreHistory[0].Content != "资料库配置" {
		t.Fatalf("lore history should stay scoped: %#v", loreHistory)
	}
	if err := application.ConfigManager().Clear(automationReq); err != nil {
		t.Fatal(err)
	}
	loreHistory, err = application.ConfigManager().Messages(loreReq)
	if err != nil {
		t.Fatal(err)
	}
	if len(loreHistory) != 1 || loreHistory[0].Content != "资料库配置" {
		t.Fatalf("clearing automation should not clear lore history: %#v", loreHistory)
	}
}

func persistentAgentKinds() []string {
	var kinds []string
	for _, definition := range config.AgentKindDefinitions() {
		if definition.SessionID != "" {
			kinds = append(kinds, definition.Kind)
		}
	}
	return kinds
}

func TestAppClearAgentSessionSupportsBackgroundAgents(t *testing.T) {
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	app := &App{sessionStore: store}

	if err := app.ClearAgentSession(config.AgentKindVersionSummary); err != nil {
		t.Fatal(err)
	}
	history, err := app.AgentSessionMessages(config.AgentKindVersionSummary)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 1 || history[0].Type != "clear" {
		t.Fatalf("version summary agent should expose clear marker history: %#v", history)
	}
}
