package agent

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cloudwego/eino/components/tool"

	"denova/config"
	"denova/internal/automation"
)

func TestConfigManagerToolsExposeStableSchema(t *testing.T) {
	tools, err := newConfigManagerTools(&config.Config{}, config.ResolvedAgentToolSettings{})
	if err != nil {
		t.Fatal(err)
	}
	names := configManagerToolNameSet(t, tools)

	for _, name := range []string{"list_style_references", "read_style_references", "write_style_references", "list_tellers", "read_tellers", "write_tellers", "list_actor_states", "read_actor_states", "write_actor_states", "list_image_presets", "read_image_presets", "write_image_presets", "list_lore_items", "read_lore_items", "write_lore_items", "list_skills", "read_skills", "write_skills", "list_automations", "read_automations", "write_automations", "list_agent_configs", "write_agent_configs"} {
		if !names[name] {
			t.Fatalf("stable config manager schema should expose %s, names=%v", name, names)
		}
	}

	for _, tc := range []struct {
		name       string
		capability string
	}{
		{name: "list_tellers", capability: config.AgentToolLoreRead},
		{name: "write_tellers", capability: config.AgentToolLoreWrite},
		{name: "list_style_references", capability: config.AgentToolLoreRead},
		{name: "read_style_references", capability: config.AgentToolLoreRead},
		{name: "write_style_references", capability: config.AgentToolLoreWrite},
		{name: "list_actor_states", capability: config.AgentToolLoreRead},
		{name: "write_actor_states", capability: config.AgentToolLoreWrite},
		{name: "list_automations", capability: config.AgentToolTodo},
		{name: "write_skills", capability: config.AgentToolSkills},
		{name: "list_agent_configs", capability: config.AgentToolAgentConfigRead},
		{name: "write_agent_configs", capability: config.AgentToolAgentConfigWrite},
		{name: "write_lore_items", capability: config.AgentToolLoreWrite},
	} {
		if got := ManifestForTool(tc.name).Capability; got != tc.capability {
			t.Fatalf("%s capability = %q, want %q", tc.name, got, tc.capability)
		}
	}
}

func TestListAutomationsToolUsesTheUserCatalogAcrossWorkspaces(t *testing.T) {
	novaDir := filepath.Join(t.TempDir(), "user")
	workspaceA := filepath.Join(t.TempDir(), "book-a")
	workspaceB := filepath.Join(t.TempDir(), "book-b")
	if _, err := automation.NewStore(novaDir, workspaceA).Create(automation.Task{Scope: automation.ScopeWorkspace, Name: "Task A", Template: automation.TemplateReview}); err != nil {
		t.Fatal(err)
	}
	if _, err := automation.NewStore(novaDir, workspaceB).Create(automation.Task{Scope: automation.ScopeWorkspace, Name: "Task B", Template: automation.TemplateReview}); err != nil {
		t.Fatal(err)
	}

	listTool, err := newListAutomationsTool(novaDir, workspaceA, []string{workspaceA, workspaceB})
	if err != nil {
		t.Fatal(err)
	}
	output, err := listTool.(tool.InvokableTool).InvokableRun(context.Background(), `{}`)
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"Task A", "Task B", "catalog_id:", "target: workspace"} {
		if !strings.Contains(output, required) {
			t.Fatalf("global automation catalog missing %q:\n%s", required, output)
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
		t.Fatalf("subagent with all tools disabled should keep no extra factory schema, got %v", configManagerToolNameSet(t, tools))
	}
}

func TestPresetConfigManagerToolIndexesDescribeFixedModuleOwnership(t *testing.T) {
	novaDir := t.TempDir()
	for _, tc := range []struct {
		name     string
		build    func(string) (tool.BaseTool, error)
		required []string
	}{
		{
			name:     "tellers",
			build:    newListTellersTool,
			required: []string{"适用: 共享模块（写作模式 / 游戏模式）"},
		},
		{
			name:     "image presets",
			build:    newListImagePresetsTool,
			required: []string{"适用: 共享模块（写作模式 / 游戏模式）"},
		},
		{
			name:     "story directors",
			build:    newListStoryDirectorsTool,
			required: []string{"适用: 游戏模式"},
		},
		{
			name:     "actor states",
			build:    newListActorStatesTool,
			required: []string{"适用: 游戏模式"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			base, err := tc.build(novaDir)
			if err != nil {
				t.Fatal(err)
			}
			output, err := base.(tool.InvokableTool).InvokableRun(context.Background(), `{}`)
			if err != nil {
				t.Fatal(err)
			}
			for _, required := range tc.required {
				if !strings.Contains(output, required) {
					t.Fatalf("tool output missing %q:\n%s", required, output)
				}
			}
			forbidden := []string{"mode_scope", "可配置模式"}
			for _, item := range forbidden {
				if strings.Contains(output, item) {
					t.Fatalf("tool output should not expose per-resource mode field %q:\n%s", item, output)
				}
			}
		})
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

	writeTool, err = newWriteAgentConfigsTool(&config.Config{NovaDir: t.TempDir(), Workspace: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writeTool.(tool.InvokableTool).InvokableRun(context.Background(), `{"scope":"workspace","operations":[{"op":"set_agent_override","agent":"ide","model":{"profile_id":"workspace-model"}}]}`); err == nil {
		t.Fatalf("write_agent_configs should keep model selection user-scoped")
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

func TestWriteAutomationsRequiresExplicitCreateTarget(t *testing.T) {
	writeTool, err := newWriteAutomationsTool(t.TempDir(), t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = writeTool.(tool.InvokableTool).InvokableRun(context.Background(), `{
		"operations": [{
			"op": "create",
			"task": {
				"name": "Missing target",
				"template": "custom",
				"prompt": "Run without an implicit workspace",
				"write_mode": "read_only",
				"write_scope": "none",
				"output_policy": "run_record_only"
			}
		}]
	}`)
	if err == nil || !strings.Contains(err.Error(), "target") {
		t.Fatalf("automation create should require an explicit target, got %v", err)
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
