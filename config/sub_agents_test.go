package config

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestSubAgentsReadWriteMergeSanitize(t *testing.T) {
	on := true
	off := false
	parent := Settings{SubAgents: []SubAgentConfig{{
		ID:           "Researcher",
		Name:         "Researcher",
		Description:  "Researches continuity",
		SystemPrompt: "Stay focused.",
		Enabled:      &on,
		Parents:      []string{AgentKindIDE, "invalid"},
		Tools:        AgentToolOverride{FileRead: &on, FileWrite: &on},
	}}}
	child := Settings{SubAgents: []SubAgentConfig{{
		ID:           "researcher",
		Description:  "Updated description",
		SystemPrompt: "Updated prompt.",
		Enabled:      &off,
		Parents:      []string{AgentKindInteractiveStory},
		Tools:        AgentToolOverride{FileWrite: &off},
	}}}

	merged := Merge(parent, child)
	if len(merged.SubAgents) != 1 {
		t.Fatalf("expected one merged subagent, got %d", len(merged.SubAgents))
	}
	sub := merged.SubAgents[0]
	if sub.ID != "researcher" || sub.Description != "Updated description" || sub.SystemPrompt != "Updated prompt." {
		t.Fatalf("unexpected merged subagent: %#v", sub)
	}
	if SubAgentEnabled(sub) {
		t.Fatalf("explicit disabled subagent should stay disabled")
	}
	if len(sub.Parents) != 1 || sub.Parents[0] != AgentKindInteractiveStory {
		t.Fatalf("parents should be sanitized and overridden: %#v", sub.Parents)
	}
	if sub.Tools.FileRead == nil || !*sub.Tools.FileRead || sub.Tools.FileWrite == nil || *sub.Tools.FileWrite {
		t.Fatalf("tool overrides should merge by field: %#v", sub.Tools)
	}

	path := filepath.Join(t.TempDir(), "config.toml")
	if err := WriteSettingsFile(path, merged); err != nil {
		t.Fatal(err)
	}
	read, err := ReadSettingsFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(read.SubAgents) != 1 || read.SubAgents[0].ID != "researcher" {
		t.Fatalf("sub_agents should round-trip through TOML: %#v", read.SubAgents)
	}
}

func TestConfigTemplatePreseedsWritingSubAgentsAsEditableConfig(t *testing.T) {
	settings, err := ReadSettingsFile(filepath.Join("..", "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"context-planner", "writer", "reviewer", "fixer", "final-gate", "memory-patcher"}
	if got := subAgentIDs(settings.SubAgents); !reflect.DeepEqual(got, want) {
		t.Fatalf("template writing subagents = %#v, want %#v", got, want)
	}
	for _, sub := range settings.SubAgents {
		if !SubAgentEnabled(sub) {
			t.Fatalf("template writing subagent should be enabled: %#v", sub)
		}
		if len(sub.Parents) != 1 || sub.Parents[0] != AgentKindIDE {
			t.Fatalf("template writing subagent should only belong to IDE: %#v", sub)
		}
		if sub.SystemPrompt == "" || containsASCIIOnly(sub.SystemPrompt) {
			t.Fatalf("template writing subagent prompt should be Chinese and non-empty: %#v", sub)
		}
	}
}

func TestLoadLayeredWithStartupConfigKeepsGlobalSubAgents(t *testing.T) {
	root := t.TempDir()
	novaDir := filepath.Join(root, ".nova")
	t.Chdir(root)
	t.Setenv("NOVA_DIR", novaDir)

	global := Settings{SubAgents: []SubAgentConfig{
		testSubAgent("context-planner"),
		testSubAgent("writer"),
		testSubAgent("reviewer"),
		testSubAgent("fixer"),
		testSubAgent("final-gate"),
		testSubAgent("memory-patcher"),
	}}
	user := Settings{SubAgents: []SubAgentConfig{
		testSubAgent("context-planner"),
		testSubAgent("memory-patcher"),
		testSubAgent("subagent-1"),
		testSubAgent("reviewer"),
	}}
	if err := WriteSettingsFile(filepath.Join(root, "config.toml"), global); err != nil {
		t.Fatal(err)
	}
	if err := WriteSettingsFile(filepath.Join(novaDir, "config.toml"), user); err != nil {
		t.Fatal(err)
	}

	layered, err := LoadLayeredWithStartupConfig(novaDir, "")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"context-planner", "writer", "reviewer", "fixer", "final-gate", "memory-patcher", "subagent-1"}
	if got := subAgentIDs(layered.Effective.SubAgents); !reflect.DeepEqual(got, want) {
		t.Fatalf("effective subagents = %#v, want %#v", got, want)
	}
}

func testSubAgent(id string) SubAgentConfig {
	return SubAgentConfig{
		ID:           id,
		Description:  "Test " + id,
		SystemPrompt: "Handle " + id + ".",
		Parents:      []string{AgentKindIDE},
	}
}

func subAgentIDs(subAgents []SubAgentConfig) []string {
	ids := make([]string, 0, len(subAgents))
	for _, sub := range subAgents {
		ids = append(ids, sub.ID)
	}
	return ids
}

func containsASCIIOnly(value string) bool {
	for _, r := range value {
		if r > 127 {
			return false
		}
	}
	return true
}

func TestResolveSubAgentToolsCapsParentPermissions(t *testing.T) {
	on := true
	parent := ResolvedAgentToolSettings{
		FileRead:  true,
		FileWrite: false,
		WebSearch: false,
		Skills:    true,
	}
	resolved := ResolveSubAgentTools(parent, AgentToolOverride{
		FileRead:  &on,
		FileWrite: &on,
		WebSearch: &on,
		Skills:    &on,
	})
	if !resolved.FileRead || !resolved.Skills {
		t.Fatalf("parent-allowed tools should remain enabled: %+v", resolved)
	}
	if resolved.FileWrite || resolved.WebSearch {
		t.Fatalf("subagent must not gain tools disabled on parent: %+v", resolved)
	}
}

func TestGeneralSubAgentSettingsMergeAndResolve(t *testing.T) {
	on := true
	off := false
	settings := Merge(
		Settings{GeneralSubAgents: AgentGeneralSubAgentSettings{IDE: &on}},
		Settings{GeneralSubAgents: AgentGeneralSubAgentSettings{IDE: &off}},
	)
	cfg := &Config{GeneralSubAgents: settings.GeneralSubAgents}
	if GeneralSubAgentEnabled(cfg, AgentKindIDE) {
		t.Fatalf("explicit IDE setting should disable the general subagent")
	}
	if !GeneralSubAgentEnabled(cfg, AgentKindInteractiveStory) {
		t.Fatalf("unset parent should inherit enabled default")
	}
}
