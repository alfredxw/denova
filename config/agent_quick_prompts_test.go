package config

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"testing"
)

func TestPrepareUserSettingsForWriteNormalizesAgentQuickPrompts(t *testing.T) {
	incoming := Settings{AgentQuickPrompts: AgentQuickPromptRegistry{
		" writing ": {{
			ID:       " next-chapter ",
			Name:     " Next chapter ",
			Prompt:   " Write the next chapter. ",
			Behavior: " ",
			Enabled:  true,
		}},
	}}

	prepared, err := PrepareUserSettingsForWrite(Settings{}, incoming)
	if err != nil {
		t.Fatal(err)
	}
	want := AgentQuickPromptRegistry{
		"writing": {{
			ID:       "next-chapter",
			Name:     "Next chapter",
			Prompt:   "Write the next chapter.",
			Behavior: AgentQuickPromptBehaviorFill,
			Enabled:  true,
		}},
	}
	if !reflect.DeepEqual(prepared.AgentQuickPrompts, want) {
		t.Fatalf("normalized quick prompts = %#v, want %#v", prepared.AgentQuickPrompts, want)
	}
}

func TestPrepareUserSettingsForWriteRejectsInvalidAgentQuickPrompts(t *testing.T) {
	tests := map[string]AgentQuickPromptRegistry{
		"duplicate IDs": {
			"writing": {
				{ID: "same", Name: "One", Prompt: "First", Behavior: AgentQuickPromptBehaviorFill},
				{ID: "same", Name: "Two", Prompt: "Second", Behavior: AgentQuickPromptBehaviorSend},
			},
		},
		"unknown behavior": {
			"skills": {{ID: "create", Name: "Create", Prompt: "Create it", Behavior: "preview"}},
		},
		"invalid scope": {
			"Skills Page": {{ID: "create", Name: "Create", Prompt: "Create it", Behavior: AgentQuickPromptBehaviorFill}},
		},
	}
	for name, registry := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := PrepareUserSettingsForWrite(Settings{}, Settings{AgentQuickPrompts: registry})
			if !errors.Is(err, ErrInvalidAgentQuickPrompt) {
				t.Fatalf("expected ErrInvalidAgentQuickPrompt, got %v", err)
			}
		})
	}
}

func TestApplySettingsMergePatchUpdatesOneAgentQuickPromptScope(t *testing.T) {
	existing := Settings{AgentQuickPrompts: AgentQuickPromptRegistry{
		"writing": {{ID: "write", Name: "Write", Prompt: "Write", Behavior: AgentQuickPromptBehaviorFill}},
		"skills":  {{ID: "review", Name: "Review", Prompt: "Review", Behavior: AgentQuickPromptBehaviorFill}},
	}}

	next, err := ApplySettingsMergePatch(existing, json.RawMessage(`{
		"agent_quick_prompts": {
			"skills": [{"id":"create","name":"Create","prompt":"Create","behavior":"send","enabled":true}]
		}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := next.AgentQuickPrompts["writing"]; !ok {
		t.Fatal("updating one scope removed another scope")
	}
	if got := next.AgentQuickPrompts["skills"]; len(got) != 1 || got[0].ID != "create" || got[0].Behavior != AgentQuickPromptBehaviorSend {
		t.Fatalf("skills quick prompts = %#v", got)
	}

	next, err = ApplySettingsMergePatch(next, json.RawMessage(`{"agent_quick_prompts":{"skills":null}}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := next.AgentQuickPrompts["skills"]; ok {
		t.Fatal("null scope patch should restore built-in defaults by removing the override")
	}
	if _, ok := next.AgentQuickPrompts["writing"]; !ok {
		t.Fatal("removing one scope removed another scope")
	}
}

func TestWriteSettingsFilePreservesEmptyAgentQuickPromptScope(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := WriteSettingsFile(path, Settings{AgentQuickPrompts: AgentQuickPromptRegistry{"writing": {}}}); err != nil {
		t.Fatal(err)
	}
	out, err := ReadSettingsFile(path)
	if err != nil {
		t.Fatal(err)
	}
	prompts, ok := out.AgentQuickPrompts["writing"]
	if !ok || prompts == nil || len(prompts) != 0 {
		t.Fatalf("empty quick prompt scope was not preserved: %#v", out.AgentQuickPrompts)
	}
}

func TestMergeAgentQuickPromptsReplacesAndClonesRegistry(t *testing.T) {
	child := Settings{AgentQuickPrompts: AgentQuickPromptRegistry{
		"skills": {{ID: "create", Name: "Create", Prompt: "Create", Behavior: AgentQuickPromptBehaviorFill}},
	}}
	out := Merge(Settings{AgentQuickPrompts: AgentQuickPromptRegistry{"writing": {}}}, child)
	if _, ok := out.AgentQuickPrompts["writing"]; ok {
		t.Fatal("child registry should replace the parent registry")
	}
	out.AgentQuickPrompts["skills"][0].Name = "Changed"
	if child.AgentQuickPrompts["skills"][0].Name != "Create" {
		t.Fatal("merged registry aliases the child settings")
	}
}
