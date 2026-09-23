package conversationconfig

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"denova/config"
)

func TestExternalConversationDoesNotRequireDormantNativeModel(t *testing.T) {
	runtime := &config.Config{AgentRuntimes: config.AgentRuntimeSettings{IDE: &config.RuntimePreferences{
		Selected: config.RuntimeCodex, Codex: &config.CodexRuntimeSettings{Model: "writing-model"},
	}}}
	selection, err := DefaultWithCustomAgent(runtime, config.AgentKindIDE, "")
	if err != nil {
		t.Fatal(err)
	}
	selection.ProfileID = "removed-native-profile"
	selection.ThinkingLevel = "old-native-value"
	if err := Apply(runtime, selection); err != nil {
		t.Fatalf("inactive Native settings blocked external execution: %v", err)
	}
	if !reflect.DeepEqual(runtime.ActiveAgentRuntime, selection.Runtime) {
		t.Fatalf("execution selection = %#v", runtime.ActiveAgentRuntime)
	}
	native := config.RuntimeSelection{Kind: config.RuntimeNative}
	if _, err := Merge(runtime, selection, Patch{Runtime: &native}); err == nil {
		t.Fatal("switching to Native must validate its model")
	}
	for _, field := range []string{"profile_id", "thinking_level", "approval_mode"} {
		var patch Patch
		if err := json.Unmarshal([]byte(`{"`+field+`":"ask"}`), &patch); err != nil {
			t.Fatal(err)
		}
		if _, err := Merge(runtime, selection, patch); !errors.Is(err, ErrRuntimeCapabilityUnsupported) {
			t.Fatalf("inactive field %s was accepted: %v", field, err)
		}
	}
}

func TestRuntimePatchReplacesBranchAndPreservesAgentSnapshot(t *testing.T) {
	base := Config{AgentKind: config.AgentKindIDE, ProfileID: "default", ThinkingLevel: "medium", ApprovalMode: config.AgentApprovalAsk}
	for _, payload := range []string{
		`{"runtime":null}`, `{"runtime":{"kind":"codex","codex":{"model":"x","unknown":true}}}`,
	} {
		var patch Patch
		if err := json.Unmarshal([]byte(payload), &patch); err == nil {
			t.Fatalf("invalid patch accepted: %s", payload)
		}
	}
	for _, payload := range []string{
		`{"runtime":{"kind":"unknown"}}`, `{"runtime":{"kind":"native","codex":{"model":"x"}}}`,
		`{"runtime":{"kind":"codex"}}`,
	} {
		var patch Patch
		if err := json.Unmarshal([]byte(payload), &patch); err != nil {
			t.Fatal(err)
		}
		if _, err := Merge(&config.Config{}, base, patch); err == nil {
			t.Fatalf("invalid branch accepted: %s", payload)
		}
	}
	var patch Patch
	if err := json.Unmarshal([]byte(`{"runtime":{"kind":"codex","codex":{"model":"x"}}}`), &patch); err != nil {
		t.Fatal(err)
	}
	next, err := Merge(&config.Config{}, base, patch)
	if err != nil {
		t.Fatal(err)
	}
	if next.ProfileID != base.ProfileID || next.ThinkingLevel != base.ThinkingLevel || next.ApprovalMode != base.ApprovalMode || base.Runtime != nil {
		t.Fatal("engine change modified dormant fields or the input snapshot")
	}
	patch.Runtime.Codex.Model = "mutated-caller"
	if next.Engine().Codex.Model != "x" {
		t.Fatal("snapshot aliases the patch")
	}
	game := base
	game.AgentKind = config.AgentKindInteractiveStory
	if selected, err := Merge(&config.Config{}, game, patch); err != nil || selected.Engine().Kind != config.RuntimeCodex {
		t.Fatalf("Game runtime selection: %+v, %v", selected, err)
	}
}

func TestCustomRuntimeIsCapturedAndDoesNotInheritBuiltInDefaults(t *testing.T) {
	runtime := &config.Config{
		AgentRuntimes: config.AgentRuntimeSettings{IDE: &config.RuntimePreferences{Selected: config.RuntimeCodex, Codex: &config.CodexRuntimeSettings{Model: "built-in"}}},
		CustomAgents:  []config.CustomAgentConfig{{ID: "writer", Name: "Writer", Contract: config.AgentContractWritingPrimary}},
	}
	native, err := DefaultWithCustomAgent(runtime, config.AgentKindIDE, "writer")
	if err != nil || native.Engine().Kind != config.RuntimeNative {
		t.Fatalf("custom inherited built-in engine: %#v, %v", native, err)
	}
	runtime.CustomAgents[0].Runtime = &config.RuntimePreferences{Selected: config.RuntimeCodex, Codex: &config.CodexRuntimeSettings{Model: "custom"}}
	external, err := DefaultWithCustomAgent(runtime, config.AgentKindIDE, "writer")
	if err != nil {
		t.Fatal(err)
	}
	runtime.CustomAgents[0].Runtime.Codex.Model = "later-change"
	external.Runtime.Codex.Model = "session-selection"
	if err := Apply(runtime, external); err != nil {
		t.Fatal(err)
	}
	if runtime.ActiveAgentRuntime.Codex.Model != "session-selection" || external.CustomAgent.Runtime.Codex.Model != "custom" {
		t.Fatal("definition or later defaults replaced explicit session selection")
	}
	legacy := LegacyDefault(runtime, config.AgentKindIDE)
	if legacy.Runtime != nil || legacy.Engine().Kind != config.RuntimeNative {
		t.Fatal("legacy initialization followed external defaults")
	}
}

func TestCodexModelPatchPreservesEngineAndReplacesSelection(t *testing.T) {
	for _, kind := range []string{config.AgentKindIDE, config.AgentKindGeneral, config.AgentKindInteractiveStory} {
		t.Run(kind, func(t *testing.T) {
			base := Config{AgentKind: kind, ProfileID: "dormant", ThinkingLevel: "medium", ApprovalMode: config.AgentApprovalAsk,
				Runtime: &config.RuntimeSelection{Kind: config.RuntimeCodex, Codex: &config.CodexRuntimeSettings{Model: "first", Effort: "high"}}}
			for _, payload := range []string{`{"codex":{"model":"first","effort":"low"}}`, `{"codex":{"model":"second"}}`} {
				var patch Patch
				if err := json.Unmarshal([]byte(payload), &patch); err != nil {
					t.Fatal(err)
				}
				next, err := Merge(&config.Config{}, base, patch)
				if err != nil {
					t.Fatal(err)
				}
				want := base.Clone()
				want.Runtime.Codex = patch.Codex
				if !reflect.DeepEqual(next, want) {
					t.Fatalf("model edit changed other configuration: %#v", next)
				}
				patch.Codex.Model = "mutated"
				if next.Runtime.Codex.Model == "mutated" || base.Runtime.Codex.Model != "first" {
					t.Fatal("model edit aliases its inputs")
				}
			}
			for _, payload := range []string{`{"codex":{}}`, `{"codex":{"model":" "}}`, `{"codex":{"model":"x"},"runtime":{"kind":"codex","codex":{"model":"y"}}}`, `{"codex":{"model":"x"},"custom_agent_id":"other"}`} {
				var patch Patch
				if err := json.Unmarshal([]byte(payload), &patch); err != nil {
					t.Fatal(err)
				}
				if _, err := Merge(&config.Config{}, base, patch); err == nil {
					t.Fatalf("invalid patch accepted: %s", payload)
				}
			}
			base.Runtime = nil
			if _, err := Merge(&config.Config{}, base, Patch{Codex: &config.CodexRuntimeSettings{Model: "x"}}); !errors.Is(err, ErrRuntimeCapabilityUnsupported) {
				t.Fatalf("model edit switched a Native engine: %v", err)
			}
		})
	}
	for _, payload := range []string{`{"codex":null}`, `{"codex":{"model":"x","unknown":true}}`} {
		var patch Patch
		if err := json.Unmarshal([]byte(payload), &patch); err == nil {
			t.Fatalf("invalid patch shape accepted: %s", payload)
		}
	}
}
