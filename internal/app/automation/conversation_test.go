package automationapp

import (
	"testing"
	"time"

	"denova/config"
	"denova/internal/agents/session"
	"denova/internal/automation"
)

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
				{ID: "fast", Name: "Fast", Model: "fast-model"},
				{ID: "quality", Name: "Quality", Model: "quality-model"},
			},
		},
	}
	service := &Service{}
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
	if resolved.ProfileID != "quality" || resolved.Model != "quality-model" || resolved.ThinkingLevel != "high" {
		t.Fatalf("inherited automation model = %#v, want quality profile", resolved)
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
