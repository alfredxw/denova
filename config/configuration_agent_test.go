package config

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestConfigurationCapabilitiesBelongToProjectAgents(t *testing.T) {
	if _, ok := LookupAgentKind(AgentKindConfigManager); ok {
		t.Fatal("retired Config Manager must not be an active Agent kind")
	}
	for _, kind := range []string{AgentKindGeneral, AgentKindIDE} {
		resolved := ResolveAgentTools(nil, kind)
		if !resolved.Allows(AgentToolConfigRead) || !resolved.Allows(AgentToolConfigApply) {
			t.Fatalf("%s must expose config_read and config_apply: %#v", kind, resolved)
		}
	}
	if resolved := ResolveAgentTools(nil, AgentKindInteractiveStory); resolved.Allows(AgentToolConfigRead) || resolved.Allows(AgentToolConfigApply) {
		t.Fatalf("Game Agent must not expose configuration mutation tools: %#v", resolved)
	}
}

func TestRetiredConfigManagerSettingsSurviveRoundTrip(t *testing.T) {
	legacyParent := AgentKindConfigManager
	want := Settings{
		AgentModels:      AgentModelSettings{ConfigManager: AgentModelOverride{ThinkingLevel: "low"}},
		AgentTools:       AgentToolSettings{ConfigManager: AgentToolOverride{AgentToolConfigRead: true}},
		AgentPrompts:     AgentPromptSettings{ConfigManager: AgentPromptOverride{FlowPrompt: "legacy flow"}},
		AgentSkills:      AgentSkillSettings{ConfigManager: AgentSkillOverride{"legacy-skill": false}},
		AgentContexts:    AgentContextSettings{ConfigManager: AgentContextOverride{CompactionEnabled: boolPtr(false)}},
		GeneralSubAgents: AgentGeneralSubAgentSettings{ConfigManager: boolPtr(true)},
		SubAgents: []SubAgentConfig{{
			ID: "legacy-helper", Description: "Legacy helper", SystemPrompt: "Keep legacy data.",
			Parents: []string{legacyParent},
		}},
	}
	path := filepath.Join(t.TempDir(), "settings.toml")
	if err := WriteSettingsFile(path, want); err != nil {
		t.Fatal(err)
	}
	got, err := ReadSettingsFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.AgentModels.ConfigManager, want.AgentModels.ConfigManager) ||
		!reflect.DeepEqual(got.AgentTools.ConfigManager, want.AgentTools.ConfigManager) ||
		!reflect.DeepEqual(got.AgentPrompts.ConfigManager, want.AgentPrompts.ConfigManager) ||
		!reflect.DeepEqual(got.AgentSkills.ConfigManager, want.AgentSkills.ConfigManager) ||
		!reflect.DeepEqual(got.AgentContexts.ConfigManager, want.AgentContexts.ConfigManager) ||
		!reflect.DeepEqual(got.GeneralSubAgents.ConfigManager, want.GeneralSubAgents.ConfigManager) {
		t.Fatalf("retired Config Manager settings changed during round trip:\n got=%#v\nwant=%#v", got, want)
	}
	if len(got.SubAgents) != 1 || !reflect.DeepEqual(got.SubAgents[0].Parents, []string{legacyParent}) {
		t.Fatalf("legacy Config Manager SubAgent parent was not preserved: %#v", got.SubAgents)
	}
}
