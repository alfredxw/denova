package config

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"
)

func TestSettingsRuntimePatchPreservesSelectionAndInactiveBranches(t *testing.T) {
	original := Settings{AgentRuntimes: AgentRuntimeSettings{IDE: &RuntimePreferences{
		Selected: RuntimeCodex, Codex: &CodexRuntimeSettings{Model: "first", Effort: "high"},
	}}}
	for _, test := range []struct {
		name  string
		patch string
		want  *RuntimePreferences
	}{
		{"switch", `{"agent_runtimes":{"ide":{"selected":"native"}}}`, &RuntimePreferences{Selected: RuntimeNative, Codex: &CodexRuntimeSettings{Model: "first", Effort: "high"}}},
		{"replace model", `{"agent_runtimes":{"ide":{"codex":{"model":"second"}}}}`, &RuntimePreferences{Selected: RuntimeCodex, Codex: &CodexRuntimeSettings{Model: "second"}}},
		{"clear selector", `{"agent_runtimes":{"ide":{"selected":null}}}`, &RuntimePreferences{Codex: &CodexRuntimeSettings{Model: "first", Effort: "high"}}},
		{"clear branch", `{"agent_runtimes":{"ide":{"codex":null}}}`, &RuntimePreferences{Selected: RuntimeCodex}},
		{"clear role", `{"agent_runtimes":{"ide":null}}`, nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := ValidateWorkspaceSettingsPatch(json.RawMessage(test.patch)); err != nil {
				t.Fatal(err)
			}
			result, err := ApplySettingsMergePatch(original, json.RawMessage(test.patch))
			if err != nil || !reflect.DeepEqual(result.AgentRuntimes.IDE, test.want) {
				t.Fatalf("got %#v, %v; want %#v", result.AgentRuntimes.IDE, err, test.want)
			}
			if original.AgentRuntimes.IDE.Selected != RuntimeCodex || original.AgentRuntimes.IDE.Codex.Effort != "high" {
				t.Fatal("patch mutated the source settings")
			}
		})
	}
	for _, patch := range []string{
		`{"agent_runtimes":{"ide":{"codex":{"effort":"low"}}}}`,
		`{"agent_runtimes":{"ide":{"selected":"missing-runtime"}}}`,
		`{"agent_runtimes":{"image":{"selected":"codex"}}}`,
		`{"agent_runtimes":{"ide":{"codex":{"unknown":null}}}}`,
	} {
		if _, err := ApplySettingsMergePatch(original, json.RawMessage(patch)); !errors.Is(err, ErrInvalidSettingsPatch) {
			t.Fatalf("invalid runtime patch accepted: %s, %v", patch, err)
		}
	}
}

func TestRuntimePreferencesRoundTripThroughAgentProfiles(t *testing.T) {
	original := Settings{AgentRuntimes: AgentRuntimeSettings{
		IDE:              &RuntimePreferences{Selected: RuntimeNative, Codex: &CodexRuntimeSettings{Model: "retained", Effort: "low"}},
		General:          &RuntimePreferences{Selected: RuntimeCodex, Codex: &CodexRuntimeSettings{Model: "active"}},
		InteractiveStory: &RuntimePreferences{Selected: RuntimeClaude, Claude: &ClaudeRuntimeSettings{Model: "story-model"}},
	}}
	restored := Settings{}
	for _, kind := range []string{AgentKindIDE, AgentKindGeneral, AgentKindInteractiveStory} {
		content, err := encodeMainAgentProfile(original, fixedAgentProfile{Kind: kind})
		if err != nil {
			t.Fatal(err)
		}
		document, err := decodeMainAgentProfile(kind+".toml", content, kind)
		if err != nil {
			t.Fatal(err)
		}
		if err := applyMainAgentProfile(&restored, document, kind); err != nil {
			t.Fatal(err)
		}
	}
	if !reflect.DeepEqual(original.AgentRuntimes, restored.AgentRuntimes) {
		t.Fatalf("profile pipeline lost runtime preferences: %#v", restored.AgentRuntimes)
	}
	if scoped := workspaceAgentSettings(original); !reflect.DeepEqual(scoped.AgentRuntimes, original.AgentRuntimes) {
		t.Fatal("workspace filtering lost runtime preferences")
	}
	if projected := agentProfileSettings(original); !reflect.DeepEqual(projected.AgentRuntimes, original.AgentRuntimes) {
		t.Fatal("Agent Profile projection lost runtime preferences")
	}
}

func TestRuntimePreferencesPreserveBranchesAndReplaceModelSettings(t *testing.T) {
	parent := AgentRuntimeSettings{IDE: &RuntimePreferences{
		Selected: RuntimeCodex,
		Codex:    &CodexRuntimeSettings{Model: "first-model", Effort: "high"},
	}}
	native := MergeAgentRuntimeSettings(parent, AgentRuntimeSettings{IDE: &RuntimePreferences{Selected: RuntimeNative}})
	if want := (&RuntimePreferences{Selected: RuntimeNative, Codex: &CodexRuntimeSettings{Model: "first-model", Effort: "high"}}); !reflect.DeepEqual(native.IDE, want) {
		t.Fatalf("inactive settings were lost: got %#v, want %#v", native.IDE, want)
	}
	reselected := MergeAgentRuntimeSettings(native, AgentRuntimeSettings{IDE: &RuntimePreferences{Selected: RuntimeCodex}})
	if !reflect.DeepEqual(reselected, parent) {
		t.Fatalf("switching back changed settings: got %#v, want %#v", reselected, parent)
	}
	replaced := MergeAgentRuntimeSettings(parent, AgentRuntimeSettings{IDE: &RuntimePreferences{Codex: &CodexRuntimeSettings{Model: "other-model"}}})
	if replaced.IDE.Selected != RuntimeCodex || *replaced.IDE.Codex != (CodexRuntimeSettings{Model: "other-model"}) {
		t.Fatalf("model branch was recursively merged: %#v", replaced.IDE)
	}
	replaced.IDE.Codex.Model = "changed-copy"
	if parent.IDE.Codex.Model != "first-model" {
		t.Fatal("merged settings alias their parent")
	}
}

func TestRuntimeSelectionOnlyUsesTheActiveBranch(t *testing.T) {
	preferences := RuntimePreferences{Selected: RuntimeNative, Codex: &CodexRuntimeSettings{Model: "unavailable-model"}}
	selected, err := preferences.Selection(AgentKindIDE)
	if err != nil || !reflect.DeepEqual(selected, RuntimeSelection{Kind: RuntimeNative}) {
		t.Fatalf("Native selected an inactive branch: %#v, %v", selected, err)
	}
	preferences.Selected = RuntimeCodex
	selected, err = preferences.Selection(AgentKindIDE)
	if err != nil || selected.Codex.Model != "unavailable-model" {
		t.Fatalf("shape validation depends on a live model catalog: %#v, %v", selected, err)
	}
	selected.Codex.Model = "mutated"
	if preferences.Codex.Model != "unavailable-model" {
		t.Fatal("selection aliases mutable preferences")
	}
	if _, err := preferences.Selection(AgentKindInteractiveStory); err != nil {
		t.Fatalf("game rejected an external runtime: %v", err)
	}
	if err := (RuntimeSelection{Kind: RuntimeNative, Codex: preferences.Codex}).Validate(AgentKindIDE); !errors.Is(err, ErrInvalidAgentRuntime) {
		t.Fatalf("mixed execution branches were accepted: %v", err)
	}
}

func TestMissingRuntimeKeepsReleasedNativeDefaults(t *testing.T) {
	for _, kind := range []string{AgentKindIDE, AgentKindGeneral, AgentKindInteractiveStory, AgentKindImage} {
		selected, err := (AgentRuntimeSettings{}).ForAgent(kind).Selection(kind)
		if err != nil || !reflect.DeepEqual(selected, RuntimeSelection{Kind: RuntimeNative}) {
			t.Fatalf("missing runtime changed %s: %#v, %v", kind, selected, err)
		}
	}
	if err := (RuntimePreferences{Selected: RuntimeCodex}).Validate(); err != nil {
		t.Fatalf("preferences cannot retain a selection requiring configuration: %v", err)
	}
	if _, err := (RuntimePreferences{Selected: RuntimeCodex}).Selection(AgentKindGeneral); !errors.Is(err, ErrInvalidAgentRuntime) {
		t.Fatalf("unconfigured external engine was executable: %v", err)
	}
}
