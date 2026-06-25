package config

import (
	"path/filepath"
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
