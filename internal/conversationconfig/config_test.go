package conversationconfig

import (
	"encoding/json"
	"testing"

	"denova/config"
)

func TestApplyOverridesOnlyConversationOwnedRuntimeFields(t *testing.T) {
	temperature := 0.35
	runtimeCfg := config.Config{
		OpenAIModel:       "default-model",
		AgentApprovalMode: config.AgentApprovalWrite,
		ModelProfiles: []config.ModelProfileSettings{
			{ID: "fast", OpenAIModel: "fast-model"},
		},
		AgentModels: config.AgentModelSettings{
			IDE:     config.AgentModelOverride{ProfileID: "default", ThinkingLevel: "medium", Temperature: &temperature},
			General: config.AgentModelOverride{ProfileID: "default", ThinkingLevel: "low"},
		},
	}
	selection := Config{
		AgentKind: config.AgentKindIDE, ProfileID: "fast", ThinkingLevel: "high",
		ApprovalMode: config.AgentApprovalFullAccess,
	}
	if err := Apply(&runtimeCfg, selection); err != nil {
		t.Fatal(err)
	}

	resolved := config.ResolveAgentModel(&runtimeCfg, config.AgentKindIDE)
	if resolved.ProfileID != "fast" || resolved.OpenAIModel != "fast-model" || resolved.ThinkingLevel != "high" {
		t.Fatalf("resolved IDE selection = %#v", resolved)
	}
	if resolved.Temperature == nil || *resolved.Temperature != temperature {
		t.Fatalf("Settings-owned temperature changed: %#v", resolved.Temperature)
	}
	general := config.ResolveAgentModel(&runtimeCfg, config.AgentKindGeneral)
	if general.ProfileID != "default" || general.ThinkingLevel != "low" {
		t.Fatalf("unrelated Agent model changed: %#v", general)
	}
	if runtimeCfg.AgentApprovalMode != config.AgentApprovalFullAccess {
		t.Fatalf("approval mode = %q", runtimeCfg.AgentApprovalMode)
	}
}

func TestPatchJSONRejectsNullAndUnknownFields(t *testing.T) {
	for _, input := range []string{
		`{"profile_id":null}`,
		`{"thinking_effort":"high"}`,
	} {
		var patch Patch
		if err := json.Unmarshal([]byte(input), &patch); err == nil {
			t.Fatalf("expected invalid patch %s to fail", input)
		}
	}
	var patch Patch
	if err := json.Unmarshal([]byte(`{"thinking_level":"high"}`), &patch); err != nil {
		t.Fatal(err)
	}
	if patch.ThinkingLevel == nil || *patch.ThinkingLevel != "high" || patch.ProfileID != nil || patch.ApprovalMode != nil {
		t.Fatalf("decoded patch = %#v", patch)
	}
}

func TestMergeRejectsUnavailableProfilesWithoutMutatingBase(t *testing.T) {
	runtimeCfg := config.Config{AgentApprovalMode: config.AgentApprovalWrite}
	base := Config{
		AgentKind: config.AgentKindIDE, ProfileID: "default", ThinkingLevel: "medium",
		ApprovalMode: config.AgentApprovalWrite,
	}
	missing := "removed-profile"
	if _, err := Merge(&runtimeCfg, base, Patch{ProfileID: &missing}); err == nil {
		t.Fatal("missing profile should be rejected")
	}
	if base.ProfileID != "default" {
		t.Fatalf("base mutated after failed merge: %#v", base)
	}
}
