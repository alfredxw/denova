package app

import (
	"testing"
	"time"

	"denova/config"
	"denova/internal/agents/session"
	"denova/internal/automation"
)

func TestNewConversationInheritsLatestSameKindWithoutChangingOlderSessions(t *testing.T) {
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	runtimeCfg := config.Config{
		AgentApprovalMode: config.AgentApprovalWrite,
		AgentModels: config.AgentModelSettings{
			IDE:     config.AgentModelOverride{ThinkingLevel: "medium"},
			General: config.AgentModelOverride{ThinkingLevel: "low"},
		},
	}

	first, firstSnapshot, err := getOrCreateConversationSession(store, "writing-first", &runtimeCfg, config.AgentKindIDE)
	if err != nil {
		t.Fatal(err)
	}
	firstSelection := firstSnapshot.Config
	firstSelection.ThinkingLevel = "high"
	firstSelection.ApprovalMode = config.AgentApprovalFullAccess
	configuredFirst, err := first.SetRuntimeConfig(firstSelection, firstSnapshot.Revision)
	if err != nil {
		t.Fatal(err)
	}

	_, inherited, err := getOrCreateConversationSession(store, "writing-second", &runtimeCfg, config.AgentKindIDE)
	if err != nil {
		t.Fatal(err)
	}
	if inherited.Revision != 1 || inherited.Config != configuredFirst.Config {
		t.Fatalf("new writing conversation did not inherit latest selection: %#v", inherited)
	}

	firstAfter, ok := first.RuntimeConfig()
	if !ok || firstAfter != configuredFirst {
		t.Fatalf("creating a new conversation changed the older one: %#v", firstAfter)
	}

	_, general, err := getOrCreateConversationSession(store, "general-first", &runtimeCfg, config.AgentKindGeneral)
	if err != nil {
		t.Fatal(err)
	}
	if general.AgentKind != config.AgentKindGeneral || general.ThinkingLevel != "low" || general.ApprovalMode != config.AgentApprovalWrite {
		t.Fatalf("different Agent kind should use its own default/history: %#v", general)
	}
}

func TestLegacyConversationInitializesOnceAndThenStaysIndependent(t *testing.T) {
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	legacy, err := store.GetOrCreate("legacy-writing")
	if err != nil {
		t.Fatal(err)
	}
	runtimeCfg := config.Config{
		AgentApprovalMode: config.AgentApprovalWrite,
		AgentModels: config.AgentModelSettings{
			IDE: config.AgentModelOverride{ThinkingLevel: "medium"},
		},
	}
	initialized, err := ensureExistingSessionConfig(legacy, &runtimeCfg, config.AgentKindIDE)
	if err != nil {
		t.Fatal(err)
	}

	runtimeCfg.AgentApprovalMode = config.AgentApprovalAsk
	runtimeCfg.AgentModels.IDE.ThinkingLevel = "max"
	again, err := ensureExistingSessionConfig(legacy, &runtimeCfg, config.AgentKindIDE)
	if err != nil {
		t.Fatal(err)
	}
	if again != initialized {
		t.Fatalf("initialized old conversation followed later Settings: before=%#v after=%#v", initialized, again)
	}
}

func TestConversationConfigPreviewDoesNotPersistDraftSession(t *testing.T) {
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	runtimeCfg := config.Config{
		AgentApprovalMode: config.AgentApprovalWrite,
		AgentModels: config.AgentModelSettings{
			General: config.AgentModelOverride{ThinkingLevel: "medium"},
		},
	}
	snapshot, err := previewConversationSessionConfig(store, "local-draft", &runtimeCfg, config.AgentKindGeneral)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Revision != 0 || snapshot.AgentKind != config.AgentKindGeneral || snapshot.ThinkingLevel != "medium" {
		t.Fatalf("draft preview = %#v", snapshot)
	}
	if store.Exists("local-draft") {
		t.Fatal("reading draft configuration must not create a durable session")
	}
	_, persisted, err := getOrCreateConversationSession(store, "local-draft", &runtimeCfg, config.AgentKindGeneral)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.Revision != 1 || !store.Exists("local-draft") {
		t.Fatalf("persisted draft configuration = %#v", persisted)
	}
}

func TestAutomationConversationInheritsRecentSelectionAndAppliesItToRuntime(t *testing.T) {
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	snap := &automationWorkspaceSnapshot{
		sessionStore: store,
		cfg: config.Config{
			OpenAIModel:       "base-model",
			AgentApprovalMode: config.AgentApprovalWrite,
			AgentModels: config.AgentModelSettings{
				Automation: config.AgentModelOverride{ProfileID: "fast", ThinkingLevel: "low"},
			},
			ModelProfiles: []config.ModelProfileSettings{
				{ID: "fast", Name: "Fast", OpenAIModel: "fast-model"},
				{ID: "quality", Name: "Quality", OpenAIModel: "quality-model"},
			},
		},
	}
	service := &AutomationAppService{}
	task := automation.Task{Name: "Review", ModelProfileID: "fast"}
	firstRun := automation.RunRecord{ID: "first", SessionID: automationRunSessionID("first"), Trigger: automation.TriggerManual, StartedAt: time.Now()}
	firstConversation, err := service.newRunConversation(snap, firstRun, task)
	if err != nil {
		t.Fatal(err)
	}
	firstSession, err := store.Get(firstRun.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	firstSnapshot, ok := firstSession.RuntimeConfig()
	if !ok {
		t.Fatal("first automation conversation has no runtime config")
	}
	configured := firstSnapshot.Config
	configured.ProfileID = "quality"
	configured.ThinkingLevel = "high"
	configured.ApprovalMode = config.AgentApprovalFullAccess
	if _, err := firstSession.SetRuntimeConfig(configured, firstSnapshot.Revision); err != nil {
		t.Fatal(err)
	}

	secondRun := automation.RunRecord{ID: "second", SessionID: automationRunSessionID("second"), Trigger: automation.TriggerSchedule, StartedAt: time.Now()}
	secondConversation, err := service.newRunConversation(snap, secondRun, task)
	if err != nil {
		t.Fatal(err)
	}
	secondRuntime := secondConversation.RuntimeConfig()
	resolved := config.ResolveAgentModel(&secondRuntime, config.AgentKindAutomation)
	if resolved.ProfileID != "quality" || resolved.OpenAIModel != "quality-model" || resolved.ThinkingLevel != "high" {
		t.Fatalf("inherited automation model = %#v", resolved)
	}
	if secondConversation.RuntimeConfig().AgentApprovalMode != config.AgentApprovalFullAccess {
		t.Fatalf("inherited automation approval = %q", secondConversation.RuntimeConfig().AgentApprovalMode)
	}
	firstRuntime := firstConversation.RuntimeConfig()
	firstResolved := config.ResolveAgentModel(&firstRuntime, config.AgentKindAutomation)
	if firstResolved.ProfileID != "fast" {
		t.Fatalf("older automation runtime changed after configuring a newer run: %#v", firstResolved)
	}
}
