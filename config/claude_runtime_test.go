package config

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestClaudePreferencesPreserveOtherEnginesAndReplaceWholeModel(t *testing.T) {
	parent := Settings{AgentRuntimes: AgentRuntimeSettings{IDE: &RuntimePreferences{Selected: RuntimeCodex, Codex: &CodexRuntimeSettings{Model: "codex-model", Effort: "high"}, Claude: &ClaudeRuntimeSettings{Model: "opus", Effort: "max"}}}}
	next, err := ApplySettingsMergePatch(parent, json.RawMessage(`{"agent_runtimes":{"ide":{"selected":"claude","claude":{"model":"sonnet"}}}}`))
	if err != nil {
		t.Fatal(err)
	}
	want := &RuntimePreferences{Selected: RuntimeClaude, Codex: parent.AgentRuntimes.IDE.Codex, Claude: &ClaudeRuntimeSettings{Model: "sonnet"}}
	if !reflect.DeepEqual(next.AgentRuntimes.IDE, want) {
		t.Fatalf("merged preferences: %#v", next.AgentRuntimes.IDE)
	}
	selection, err := next.AgentRuntimes.IDE.Selection(AgentKindIDE)
	if err != nil || selection.Codex != nil || selection.Claude.Model != "sonnet" {
		t.Fatalf("active selection: %#v %v", selection, err)
	}
	selection.Claude.Model = "modified"
	if next.AgentRuntimes.IDE.Claude.Model != "sonnet" || parent.AgentRuntimes.IDE.Claude.Model != "opus" {
		t.Fatal("selection mutated saved preferences")
	}
	for _, kind := range []string{AgentKindImage} {
		if _, err := next.AgentRuntimes.IDE.Selection(kind); err == nil {
			t.Fatalf("external execution allowed for %s", kind)
		}
	}
}
