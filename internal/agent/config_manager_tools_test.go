package agent

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
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

	tools, err = newConfigManagerTools(&config.Config{}, config.ResolvedAgentToolSettings{AgentConfigRead: true, AgentConfigWrite: true})
	if err != nil {
		t.Fatal(err)
	}
	names = configManagerToolNameSet(t, tools)
	for _, name := range []string{"list_agent_configs", "write_agent_configs"} {
		if !names[name] {
			t.Fatalf("agent config settings should expose %s, names=%v", name, names)
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

func TestListAgentConfigsReturnsAllLayersWithoutAPIKeys(t *testing.T) {
	novaDir := t.TempDir()
	workspace := t.TempDir()
	if err := config.WriteSettingsFile(config.UserConfigPath(novaDir), config.Settings{
		OpenAIAPIKey: "user-secret",
		ModelProfiles: []config.ModelProfileSettings{{
			ID:           "deepseek",
			OpenAIAPIKey: "profile-secret",
			OpenAIModel:  "deepseek-v3",
		}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := config.WriteSettingsFile(config.WorkspaceConfigPath(workspace), config.Settings{
		SubAgents: []config.SubAgentConfig{{
			ID:           "workspace-researcher",
			Name:         "Workspace Researcher",
			Description:  "Reads workspace context.",
			SystemPrompt: "Return concise findings.",
		}},
	}); err != nil {
		t.Fatal(err)
	}

	listTool, err := newListAgentConfigsTool(&config.Config{NovaDir: novaDir, Workspace: workspace})
	if err != nil {
		t.Fatal(err)
	}
	output, err := listTool.(tool.InvokableTool).InvokableRun(context.Background(), `{}`)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"user-secret", "profile-secret", "openai_api_key"} {
		if strings.Contains(output, forbidden) {
			t.Fatalf("list_agent_configs should not expose %q:\n%s", forbidden, output)
		}
	}
	for _, required := range []string{"\"user\"", "\"workspace\"", "\"effective\"", "workspace-researcher", "agent_config_read", "deepseek-v3"} {
		if !strings.Contains(output, required) {
			t.Fatalf("list_agent_configs missing %q:\n%s", required, output)
		}
	}
}

func TestWriteAgentConfigsRequiresExplicitScopeAndWorkspace(t *testing.T) {
	writeTool, err := newWriteAgentConfigsTool(&config.Config{NovaDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writeTool.(tool.InvokableTool).InvokableRun(context.Background(), `{"operations":[]}`); err == nil {
		t.Fatalf("write_agent_configs should require explicit scope")
	}
	if _, err := writeTool.(tool.InvokableTool).InvokableRun(context.Background(), `{"scope":"workspace","operations":[]}`); err == nil {
		t.Fatalf("write_agent_configs should reject workspace scope without workspace")
	}
}

func TestWriteAgentConfigsPreservesUnrelatedSettings(t *testing.T) {
	novaDir := t.TempDir()
	path := config.UserConfigPath(novaDir)
	off := false
	if err := config.WriteSettingsFile(path, config.Settings{
		Theme:                    "light",
		RemoteAccessPasswordHash: "hash-value",
		AgentTools: config.AgentToolSettings{
			IDE: config.AgentToolOverride{FileRead: &off},
		},
	}); err != nil {
		t.Fatal(err)
	}
	writeTool, err := newWriteAgentConfigsTool(&config.Config{NovaDir: novaDir, Workspace: filepath.Join(t.TempDir(), "workspace")})
	if err != nil {
		t.Fatal(err)
	}
	input := agentConfigWriteInput{
		Scope:   "user",
		Message: "更新 Agent 配置",
		Operations: []agentConfigWriteOperation{
			{
				Op:    "set_agent_override",
				Agent: config.AgentKindIDE,
				Tools: &config.AgentToolOverride{FileWrite: &off},
			},
			{
				Op: "upsert_sub_agent",
				SubAgent: config.SubAgentConfig{
					ID:           "researcher",
					Name:         "Researcher",
					Description:  "Researches delegated context.",
					SystemPrompt: "Return concise findings.",
					Parents:      []string{config.AgentKindIDE},
				},
			},
		},
	}
	data, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writeTool.(tool.InvokableTool).InvokableRun(context.Background(), string(data)); err != nil {
		t.Fatal(err)
	}
	read, err := config.ReadSettingsFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if read.Theme != "light" || read.RemoteAccessPasswordHash != "hash-value" {
		t.Fatalf("unrelated settings should be preserved: %#v", read)
	}
	if read.AgentTools.IDE.FileRead != nil {
		t.Fatalf("set_agent_override should replace the target override, got %#v", read.AgentTools.IDE)
	}
	if read.AgentTools.IDE.FileWrite == nil || *read.AgentTools.IDE.FileWrite {
		t.Fatalf("expected IDE file_write override false, got %#v", read.AgentTools.IDE)
	}
	if len(read.SubAgents) != 1 || read.SubAgents[0].ID != "researcher" {
		t.Fatalf("expected upserted SubAgent, got %#v", read.SubAgents)
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
