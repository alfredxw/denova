package conversationconfig

import (
	"testing"

	"denova/config"
)

func TestDefaultWithCustomAgentPersistsIdentityAndDefaults(t *testing.T) {
	runtime := &config.Config{
		AgentModels: config.AgentModelSettings{IDE: config.AgentModelOverride{ProfileID: "default", ThinkingLevel: "low"}},
		CustomAgents: []config.CustomAgentConfig{{
			ID: "focused-editor", Name: "Focused editor", Contract: config.AgentContractWritingPrimary,
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
	if selection.CustomAgent == nil || selection.CustomAgent.Contract != config.AgentContractWritingPrimary {
		t.Fatalf("conversation did not capture the custom Agent definition: %#v", selection.CustomAgent)
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
		ID: "retired-editor", Name: "Retired editor", Contract: config.AgentContractWritingPrimary, Enabled: &disabled,
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

func TestConversationAppliesCapturedDefinitionAfterLibraryChanges(t *testing.T) {
	runtime := &config.Config{CustomAgents: []config.CustomAgentConfig{{
		ID: "writer", Name: "Writer", Contract: config.AgentContractWritingPrimary,
		Instructions: "ORIGINAL WORKFLOW",
		Tools:        config.AgentToolOverride{config.AgentToolWebSearch: false},
	}}}
	selection, err := DefaultWithCustomAgent(runtime, config.AgentKindIDE, "writer")
	if err != nil {
		t.Fatal(err)
	}
	runtime.CustomAgents[0].Instructions = "CHANGED WORKFLOW"
	runtime.CustomAgents[0].Tools[config.AgentToolWebSearch] = true

	if err := Apply(runtime, selection); err != nil {
		t.Fatal(err)
	}
	if runtime.AgentPrompts.IDE.FlowPrompt != "ORIGINAL WORKFLOW" || runtime.AgentTools.IDE[config.AgentToolWebSearch] {
		t.Fatalf("conversation followed changed library definition: prompt=%q tools=%#v", runtime.AgentPrompts.IDE.FlowPrompt, runtime.AgentTools.IDE)
	}
}
