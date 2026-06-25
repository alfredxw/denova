package agent

import (
	"context"
	"testing"

	"github.com/cloudwego/eino/components/tool"

	"nova/config"
)

func TestConfigManagerToolsRespectToolSettings(t *testing.T) {
	tools, err := newConfigManagerTools(&config.Config{}, config.ResolvedAgentToolSettings{})
	if err != nil {
		t.Fatal(err)
	}
	if len(tools) != 0 {
		t.Fatalf("disabled settings should not expose config manager tools, got %v", configManagerToolNameSet(t, tools))
	}

	tools, err = newConfigManagerTools(&config.Config{}, config.ResolvedAgentToolSettings{LoreRead: true})
	if err != nil {
		t.Fatal(err)
	}
	names := configManagerToolNameSet(t, tools)
	for _, name := range []string{"list_tellers", "read_tellers", "list_story_memory_structures", "list_story_memory_records", "read_story_memory_records"} {
		if !names[name] {
			t.Fatalf("lore read should expose %s, names=%v", name, names)
		}
	}
	for _, name := range []string{"write_tellers", "write_story_memory_structures", "write_story_memory_records", "list_skills", "write_skills", "list_automations", "write_automations"} {
		if names[name] {
			t.Fatalf("lore read should not expose %s, names=%v", name, names)
		}
	}
}

func TestConfigManagerSubAgentToolsAreCappedBySubAgentOverride(t *testing.T) {
	off := false
	parentTools := config.ResolvedAgentToolSettings{
		FileRead:     true,
		FileWrite:    true,
		ShellExecute: true,
		Skills:       true,
		LoreRead:     true,
		LoreWrite:    true,
		Todo:         true,
		WebSearch:    true,
	}
	subTools := config.ResolveSubAgentTools(parentTools, config.AgentToolOverride{
		FileRead:     &off,
		FileWrite:    &off,
		ShellExecute: &off,
		Skills:       &off,
		LoreRead:     &off,
		LoreWrite:    &off,
		Todo:         &off,
		WebSearch:    &off,
	})
	tools, err := configManagerToolsFactory(&config.Config{})(subTools)
	if err != nil {
		t.Fatal(err)
	}
	if len(tools) != 0 {
		t.Fatalf("subagent with all tools disabled should not expose config manager tools, got %v", configManagerToolNameSet(t, tools))
	}
}

func configManagerToolNameSet(t *testing.T, tools []tool.BaseTool) map[string]bool {
	t.Helper()
	names := make(map[string]bool, len(tools))
	for _, item := range tools {
		info, err := item.Info(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		names[info.Name] = true
	}
	return names
}
