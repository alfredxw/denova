package config

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestCLIRuntimeSettingsReplaceModelsAndSurviveAgentProfiles(t *testing.T) {
	for _, engine := range []RuntimeID{RuntimeGrok, RuntimeAntigravity} {
		t.Run(string(engine), func(t *testing.T) {
			original := Settings{AgentRuntimes: AgentRuntimeSettings{General: &RuntimePreferences{
				Selected: engine, Grok: &GrokRuntimeSettings{Model: "first", Effort: "high"},
				Antigravity: &AntigravityRuntimeSettings{Model: "second", Effort: "low"},
			}}}
			patch := json.RawMessage(`{"agent_runtimes":{"general":{"` + string(engine) + `":{"model":"new"}}}}`)
			next, err := ApplySettingsMergePatch(original, patch)
			if err != nil {
				t.Fatal(err)
			}
			selection, err := next.AgentRuntimes.General.Selection(AgentKindGeneral)
			if err != nil {
				t.Fatal(err)
			}
			want := RuntimeSelection{Kind: engine}
			if engine == RuntimeGrok {
				want.Grok = &GrokRuntimeSettings{Model: "new"}
			} else {
				want.Antigravity = &AntigravityRuntimeSettings{Model: "new"}
			}
			if !reflect.DeepEqual(selection, want) {
				t.Fatalf("model replacement retained stale settings: %+v", selection)
			}
			content, err := encodeMainAgentProfile(next, fixedAgentProfile{Kind: AgentKindGeneral})
			if err != nil {
				t.Fatal(err)
			}
			document, err := decodeMainAgentProfile("general.toml", content, AgentKindGeneral)
			if err != nil {
				t.Fatal(err)
			}
			var restored Settings
			if err := applyMainAgentProfile(&restored, document, AgentKindGeneral); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(restored.AgentRuntimes, next.AgentRuntimes) {
				t.Fatal("profile round trip lost runtime branches")
			}
			copy := MergeAgentRuntimeSettings(next.AgentRuntimes, AgentRuntimeSettings{}).General
			copy.Grok.Model, copy.Antigravity.Model = "mutated", "mutated"
			if next.AgentRuntimes.General.Grok.Model == "mutated" || next.AgentRuntimes.General.Antigravity.Model == "mutated" {
				t.Fatal("merge aliases parent branches")
			}
			for _, kind := range []string{AgentKindIDE, AgentKindGeneral, AgentKindInteractiveStory} {
				if err := selection.Validate(kind); err != nil {
					t.Fatal(err)
				}
			}
			if err := selection.Validate(AgentKindImage); err == nil {
				t.Fatal("image Agent accepted an external runtime")
			}
			selection.Claude = &ClaudeRuntimeSettings{Model: "mixed"}
			if err := selection.Validate(AgentKindGeneral); err == nil {
				t.Fatal("mixed execution settings accepted")
			}
		})
	}
}
