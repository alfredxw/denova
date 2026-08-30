package conversationconfig

import (
	"testing"

	"denova/config"
)

func TestDefaultWithCustomAgentPersistsIdentityAndDefaults(t *testing.T) {
	runtime := &config.Config{
		AgentModels: config.AgentModelSettings{IDE: config.AgentModelOverride{ProfileID: "default", ThinkingLevel: "low"}},
		CustomAgents: []config.CustomAgentConfig{{
			ID: "focused-editor", Name: "Focused editor", BaseKind: config.AgentKindIDE,
			Model: config.AgentModelOverride{ThinkingLevel: "high"},
		}},
	}

	selection, err := DefaultWithCustomAgent(runtime, config.AgentKindIDE, "focused-editor")
	if err != nil {
		t.Fatal(err)
	}
	if selection.AgentKind != config.AgentKindIDE || selection.CustomAgentID != "focused-editor" {
		t.Fatalf("conversation selection = %#v", selection)
	}
	if selection.ProfileID != "default" || selection.ThinkingLevel != "high" {
		t.Fatalf("custom Agent model defaults = %#v", selection)
	}
	if _, err := DefaultWithCustomAgent(runtime, config.AgentKindGeneral, "focused-editor"); err == nil {
		t.Fatal("custom Agent was accepted for the wrong conversation kind")
	}
}

func TestPersistedSelectionCanRestoreArchivedCustomAgent(t *testing.T) {
	disabled := false
	runtime := &config.Config{CustomAgents: []config.CustomAgentConfig{{
		ID: "retired-editor", Name: "Retired editor", BaseKind: config.AgentKindIDE, Enabled: &disabled,
	}}}
	selection := Config{
		AgentKind: config.AgentKindIDE, CustomAgentID: "retired-editor",
		ProfileID: "default", ThinkingLevel: "medium", ApprovalMode: config.AgentApprovalAsk,
	}
	if err := Validate(runtime, selection, config.AgentKindIDE); err == nil {
		t.Fatal("archived custom Agent was accepted as a new selection")
	}
	if err := ValidatePersisted(runtime, selection, config.AgentKindIDE); err != nil {
		t.Fatalf("persisted archived custom Agent was rejected: %v", err)
	}
	if err := Apply(runtime, selection); err != nil {
		t.Fatalf("persisted archived custom Agent could not be applied: %v", err)
	}
}
