package config

import (
	"encoding/json"
	"testing"
)

func TestMergeCustomAgentsReplacesCompleteDefinitionByIdentity(t *testing.T) {
	disabled := false
	parent := []CustomAgentConfig{{
		ID: "focused-editor", Name: "Focused editor", Description: "Edit prose.", Contract: AgentContractWritingPrimary,
		Model: AgentModelOverride{ProfileID: "writer", ThinkingLevel: "medium"},
		Tools: AgentToolOverride{AgentToolFilesystemRead: true, AgentToolWebSearch: true},
	}}
	child := []CustomAgentConfig{{
		ID: "focused-editor", Name: "User editor", Contract: AgentContractWritingPrimary, Enabled: &disabled,
		Tools: AgentToolOverride{AgentToolWebSearch: false},
	}}

	got := MergeCustomAgents(parent, child)
	if len(got) != 1 {
		t.Fatalf("merged custom Agents = %#v", got)
	}
	agent := got[0]
	if agent.ID != "focused-editor" || agent.Name != "User editor" || agent.Contract != AgentContractWritingPrimary {
		t.Fatalf("merged identity = %#v", agent)
	}
	if CustomAgentEnabled(agent) {
		t.Fatal("workspace archive override was not applied")
	}
	if agent.Model.ProfileID != "" || agent.Model.ThinkingLevel != "" {
		t.Fatalf("parent model leaked into complete replacement = %#v", agent.Model)
	}
	if agent.Tools[AgentToolFilesystemRead] || agent.Tools[AgentToolWebSearch] {
		t.Fatalf("parent tools leaked into complete replacement = %#v", agent.Tools)
	}
}

func TestApplyCustomAgentProjectsIndependentDefinitionOntoContract(t *testing.T) {
	cfg := &Config{
		AgentModels: AgentModelSettings{IDE: AgentModelOverride{ProfileID: "base", ThinkingLevel: "low"}},
		AgentTools: AgentToolSettings{IDE: AgentToolOverride{
			AgentToolFilesystemRead: true,
			AgentToolWebSearch:      true,
		}},
		CustomAgents: []CustomAgentConfig{{
			ID: "focused-editor", Name: "Focused editor", Contract: AgentContractWritingPrimary,
			Instructions: "Preserve the author's voice.",
			Model:        AgentModelOverride{ProfileID: "custom", ThinkingLevel: "high"},
			Tools:        AgentToolOverride{AgentToolFilesystemRead: false, AgentToolWebSearch: false},
		}},
	}

	if err := ApplyCustomAgent(cfg, AgentKindIDE, "focused-editor"); err != nil {
		t.Fatal(err)
	}
	if cfg.ActiveCustomAgentID != "focused-editor" || cfg.ActiveCustomAgentName != "Focused editor" {
		t.Fatalf("active custom Agent = %q %q", cfg.ActiveCustomAgentID, cfg.ActiveCustomAgentName)
	}
	if cfg.AgentModels.IDE.ProfileID != "custom" || cfg.AgentModels.IDE.ThinkingLevel != "high" {
		t.Fatalf("projected model = %#v", cfg.AgentModels.IDE)
	}
	if cfg.AgentTools.IDE[AgentToolFilesystemRead] || cfg.AgentTools.IDE[AgentToolWebSearch] {
		t.Fatalf("projected tools = %#v", cfg.AgentTools.IDE)
	}
	if cfg.AgentPrompts.IDE.FlowPrompt != "Preserve the author's voice." {
		t.Fatalf("projected prompt = %#v", cfg.AgentPrompts.IDE)
	}
	if err := ApplyCustomAgent(cfg, AgentKindGeneral, "focused-editor"); err == nil {
		t.Fatal("custom Agent was accepted by a different runtime contract")
	}
}

func TestDefaultSettingsShipWithoutCustomAgents(t *testing.T) {
	settings := DefaultSettings()
	if len(settings.CustomAgents) != 0 {
		t.Fatalf("default custom Agents = %#v, want none", settings.CustomAgents)
	}
	if settings.DefaultImageAgentID == nil || *settings.DefaultImageAgentID != "" {
		t.Fatalf("default Image Agent ID = %#v, want explicit built-in selection", settings.DefaultImageAgentID)
	}
}

func TestArchivedCustomAgentRemainsAvailableOnlyToPersistedHistory(t *testing.T) {
	disabled := false
	cfg := &Config{CustomAgents: []CustomAgentConfig{{
		ID: "retired-editor", Name: "Retired editor", Contract: AgentContractWritingPrimary, Enabled: &disabled,
		Instructions: "Preserve prior behavior.",
	}}}
	if err := ApplyCustomAgent(cfg, AgentKindIDE, "retired-editor"); err == nil {
		t.Fatal("archived custom Agent was available to a new selection")
	}
	if err := ApplyPersistedCustomAgent(cfg, AgentKindIDE, "retired-editor"); err != nil {
		t.Fatalf("persisted history could not restore archived custom Agent: %v", err)
	}
	if cfg.AgentPrompts.IDE.FlowPrompt != "Preserve prior behavior." {
		t.Fatalf("archived custom Agent prompt = %#v", cfg.AgentPrompts.IDE)
	}
}

func TestWorkspaceCustomAgentPatchIsRejectedAsUserScoped(t *testing.T) {
	patch := json.RawMessage("{\"custom_agents\":[{\"id\":\"editor\",\"contract\":\"writing.primary.v1\"}]}")
	if err := ValidateWorkspaceSettingsPatch(patch); err == nil {
		t.Fatalf("workspace accepted user-scoped custom Agent library: %s", patch)
	}
}

func TestSanitizeCustomAgentKeepsContextDraftAndSparseSkillExceptions(t *testing.T) {
	agents := SanitizeCustomAgents([]CustomAgentConfig{{
		ID: " Writer ", Name: " Writer ", Contract: AgentContractWritingPrimary,
		SkillPolicy: AgentSkillPolicy{
			Mode: AgentSkillPolicyExplicit, Pinned: []string{"outline", "outline", "blocked"}, Blocked: []string{"blocked"},
		},
		ContextBindings: []AgentContextBinding{{ID: " house style ", Content: ""}},
	}})
	if len(agents) != 1 || len(agents[0].ContextBindings) != 1 {
		t.Fatalf("sanitized custom Agents = %#v", agents)
	}
	if agents[0].ContextBindings[0].ID != "house-style" || agents[0].ContextBindings[0].Content != "" {
		t.Fatalf("context draft = %#v", agents[0].ContextBindings[0])
	}
	if len(agents[0].SkillPolicy.Pinned) != 1 || agents[0].SkillPolicy.Pinned[0] != "outline" || len(agents[0].SkillPolicy.Blocked) != 1 {
		t.Fatalf("Skill policy = %#v", agents[0].SkillPolicy)
	}
}
