package config

import (
	"encoding/json"
	"testing"
)

func TestMergeCustomAgentsPreservesIdentityAndMergesSparseOverrides(t *testing.T) {
	disabled := false
	parent := []CustomAgentConfig{{
		ID: "focused-editor", Name: "Focused editor", Description: "Edit prose.", BaseKind: AgentKindIDE,
		Model: AgentModelOverride{ProfileID: "writer", ThinkingLevel: "medium"},
		Tools: AgentToolOverride{AgentToolFilesystemRead: true, AgentToolWebSearch: true},
	}}
	child := []CustomAgentConfig{{
		ID: "focused-editor", Name: "Workspace editor", BaseKind: AgentKindGeneral, Enabled: &disabled,
		Tools: AgentToolOverride{AgentToolWebSearch: false},
	}}

	got := MergeCustomAgents(parent, child)
	if len(got) != 1 {
		t.Fatalf("merged custom Agents = %#v", got)
	}
	agent := got[0]
	if agent.ID != "focused-editor" || agent.Name != "Workspace editor" || agent.BaseKind != AgentKindIDE {
		t.Fatalf("merged identity = %#v", agent)
	}
	if CustomAgentEnabled(agent) {
		t.Fatal("workspace archive override was not applied")
	}
	if agent.Model.ProfileID != "writer" || agent.Model.ThinkingLevel != "medium" {
		t.Fatalf("inherited model override = %#v", agent.Model)
	}
	if !agent.Tools[AgentToolFilesystemRead] || agent.Tools[AgentToolWebSearch] {
		t.Fatalf("merged tool overrides = %#v", agent.Tools)
	}
}

func TestApplyCustomAgentProjectsOntoFixedBase(t *testing.T) {
	cfg := &Config{
		AgentModels: AgentModelSettings{IDE: AgentModelOverride{ProfileID: "base", ThinkingLevel: "low"}},
		AgentTools: AgentToolSettings{IDE: AgentToolOverride{
			AgentToolFilesystemRead: true,
			AgentToolWebSearch:      true,
		}},
		CustomAgents: []CustomAgentConfig{{
			ID: "focused-editor", Name: "Focused editor", BaseKind: AgentKindIDE,
			Model:  AgentModelOverride{ProfileID: "custom", ThinkingLevel: "high"},
			Tools:  AgentToolOverride{AgentToolWebSearch: false},
			Prompt: AgentPromptOverride{SystemPrompt: "Preserve the author's voice."},
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
	if !cfg.AgentTools.IDE[AgentToolFilesystemRead] || cfg.AgentTools.IDE[AgentToolWebSearch] {
		t.Fatalf("projected tools = %#v", cfg.AgentTools.IDE)
	}
	if cfg.AgentPrompts.IDE.SystemPrompt != "Preserve the author's voice." {
		t.Fatalf("projected prompt = %#v", cfg.AgentPrompts.IDE)
	}
	if err := ApplyCustomAgent(cfg, AgentKindGeneral, "focused-editor"); err == nil {
		t.Fatal("custom Agent was accepted by a different fixed base kind")
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
		ID: "retired-editor", Name: "Retired editor", BaseKind: AgentKindIDE, Enabled: &disabled,
		Prompt: AgentPromptOverride{SystemPrompt: "Preserve prior behavior."},
	}}}
	if err := ApplyCustomAgent(cfg, AgentKindIDE, "retired-editor"); err == nil {
		t.Fatal("archived custom Agent was available to a new selection")
	}
	if err := ApplyPersistedCustomAgent(cfg, AgentKindIDE, "retired-editor"); err != nil {
		t.Fatalf("persisted history could not restore archived custom Agent: %v", err)
	}
	if cfg.AgentPrompts.IDE.SystemPrompt != "Preserve prior behavior." {
		t.Fatalf("archived custom Agent prompt = %#v", cfg.AgentPrompts.IDE)
	}
}

func TestWorkspaceCustomAgentPatchRejectsUserScopedSelections(t *testing.T) {
	valid := json.RawMessage("{\"custom_agents\":[{\"id\":\"editor\",\"base_kind\":\"ide\",\"prompt\":{\"system_prompt\":\"Keep edits small.\"}}]}")
	if err := ValidateWorkspaceSettingsPatch(valid); err != nil {
		t.Fatalf("workspace behavior override was rejected: %v", err)
	}
	for _, patch := range []json.RawMessage{
		json.RawMessage("{\"custom_agents\":[{\"id\":\"editor\",\"base_kind\":\"ide\",\"model\":{\"profile_id\":\"private-model\"}}]}"),
		json.RawMessage("{\"custom_agents\":[{\"id\":\"artist\",\"base_kind\":\"image\",\"image_api_profile_id\":\"private-image\"}]}"),
	} {
		if err := ValidateWorkspaceSettingsPatch(patch); err == nil {
			t.Fatalf("workspace accepted user-scoped custom Agent selection: %s", patch)
		}
	}
}
