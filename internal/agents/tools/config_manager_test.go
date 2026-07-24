package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	agenttools "github.com/alfredxw/denova/agent/tools"

	agent "github.com/alfredxw/denova/agent"

	"denova/config"
	"denova/internal/automation"
)

func TestConfigManagerToolsExposeStableSchema(t *testing.T) {
	tools, err := newConfigManagerTools(&config.Config{}, config.ResolvedAgentToolSettings{})
	if err != nil {
		t.Fatal(err)
	}
	names := configManagerToolNameSet(t, tools)

	for _, name := range []string{"list_style_references", "write_style_references", "list_tellers", "read_tellers", "write_tellers", "list_actor_states", "read_actor_states", "write_actor_states", "list_image_presets", "read_image_presets", "write_image_presets", "list_lore_items", "read_lore_items", "write_lore_items", "list_skills", "read_skills", "write_skills", "list_automations", "read_automations", "write_automations", "list_agent_configs", "write_agent_configs"} {
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
		{name: "write_style_references", capability: config.AgentToolLoreWrite},
		{name: "list_actor_states", capability: config.AgentToolLoreRead},
		{name: "write_actor_states", capability: config.AgentToolLoreWrite},
		{name: "list_automations", capability: config.AgentToolTodo},
		{name: "write_skills", capability: config.AgentToolSkills},
		{name: "list_agent_configs", capability: config.AgentToolAgentConfigRead},
		{name: "write_agent_configs", capability: config.AgentToolAgentConfigWrite},
		{name: "write_lore_items", capability: config.AgentToolLoreWrite},
	} {
		var selected agent.BaseTool
		for _, tool := range tools {
			info, infoErr := tool.Info(context.Background())
			if infoErr == nil && info != nil && info.Name == tc.name {
				selected = tool
				break
			}
		}
		if selected == nil {
			t.Fatalf("tool %q not found", tc.name)
		}
		info, err := selected.Info(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		descriptor, ok := agenttools.DescriptorFromInfo(info)
		if !ok {
			t.Fatalf("tool %q has no descriptor", tc.name)
		}
		if got := descriptor.Capability; got != tc.capability {
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
	output, err := listTool.(agent.InvokableTool).InvokableRun(context.Background(), `{}`)
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
		build    func(string) (agent.BaseTool, error)
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
			output, err := base.(agent.InvokableTool).InvokableRun(context.Background(), `{}`)
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
	output, err := listTool.(agent.InvokableTool).InvokableRun(context.Background(), `{}`)
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
	if _, err := writeTool.(agent.InvokableTool).InvokableRun(context.Background(), `{"operations":[]}`); err == nil {
		t.Fatalf("write_agent_configs should require explicit scope")
	}
	if _, err := writeTool.(agent.InvokableTool).InvokableRun(context.Background(), `{"scope":"workspace","operations":[]}`); err == nil {
		t.Fatalf("write_agent_configs should reject workspace scope without workspace")
	}

	writeTool, err = newWriteAgentConfigsTool(&config.Config{NovaDir: t.TempDir(), Workspace: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writeTool.(agent.InvokableTool).InvokableRun(context.Background(), `{"scope":"workspace","operations":[{"op":"set_agent_override","agent":"ide","model":{"profile_id":"workspace-model"}}]}`); err == nil {
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
	if _, err := writeTool.(agent.InvokableTool).InvokableRun(context.Background(), string(data)); err != nil {
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

func TestMutateAgentConfigsPreservesAConcurrentSettingsMutation(t *testing.T) {
	novaDir := t.TempDir()
	workspace := t.TempDir()
	path := config.UserConfigPath(novaDir)
	if err := config.WriteSettingsFile(path, config.Settings{Theme: "dark"}); err != nil {
		t.Fatal(err)
	}
	layered, err := config.LoadLayeredWithStartupConfig(novaDir, workspace)
	if err != nil {
		t.Fatal(err)
	}

	firstEntered := make(chan struct{})
	releaseFirst := make(chan struct{})
	secondStarted := make(chan struct{})
	errs := make([]error, 2)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		defer func() {
			if recovered := recover(); recovered != nil {
				errs[0] = fmt.Errorf("concurrent settings mutation panic: %v", recovered)
			}
		}()
		_, errs[0] = config.MutateSettingsFile(path, "", func(current config.Settings) (config.Settings, error) {
			close(firstEntered)
			<-releaseFirst
			current.AgentPrompts.ConfigManager.SystemPrompt = "concurrent prompt"
			return current, nil
		})
	}()
	<-firstEntered
	off := false
	go func() {
		defer wg.Done()
		defer func() {
			if recovered := recover(); recovered != nil {
				errs[1] = fmt.Errorf("agent settings mutation panic: %v", recovered)
			}
		}()
		close(secondStarted)
		_, errs[1] = mutateAgentConfigSettings(path, "user", layered, []agentConfigWriteOperation{{
			Op:    "set_agent_override",
			Agent: config.AgentKindIDE,
			Tools: &config.AgentToolOverride{FileWrite: &off},
		}})
	}()
	<-secondStarted
	close(releaseFirst)
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			t.Fatalf("concurrent mutation failed: %v", err)
		}
	}

	persisted, err := config.ReadSettingsFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.AgentPrompts.ConfigManager.SystemPrompt != "concurrent prompt" {
		t.Fatalf("Agent write lost the concurrent prompt: %#v", persisted.AgentPrompts.ConfigManager)
	}
	if persisted.AgentTools.IDE.FileWrite == nil || *persisted.AgentTools.IDE.FileWrite {
		t.Fatalf("Agent tool override was not persisted: %#v", persisted.AgentTools.IDE)
	}
}

func TestWriteAutomationsRequiresExplicitCreateTarget(t *testing.T) {
	writeTool, err := newWriteAutomationsTool(t.TempDir(), t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = writeTool.(agent.InvokableTool).InvokableRun(context.Background(), `{
		"operations": [{
			"op": "create",
			"task": {
				"name": "Missing target",
				"template": "custom_prompt",
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

func TestConfigManagerWriteSchemasKeepConditionalFieldsOptional(t *testing.T) {
	novaDir := t.TempDir()
	cfg := &config.Config{NovaDir: novaDir, Workspace: t.TempDir()}
	tests := []struct {
		name              string
		build             func() (agent.BaseTool, error)
		optionalOperation []string
		requiredOperation []string
	}{
		{name: "style references", build: func() (agent.BaseTool, error) { return newWriteStyleReferencesTool(novaDir) }, optionalOperation: []string{"path", "reference"}},
		{name: "tellers", build: func() (agent.BaseTool, error) { return newWriteTellersTool(novaDir) }, optionalOperation: []string{"id", "teller"}},
		{name: "story directors", build: func() (agent.BaseTool, error) { return newWriteStoryDirectorsTool(novaDir) }, optionalOperation: []string{"id", "director"}},
		{name: "event packages", build: func() (agent.BaseTool, error) { return newWriteEventPackagesTool(novaDir) }, optionalOperation: []string{"id", "package"}},
		{name: "actor states", build: func() (agent.BaseTool, error) { return newWriteActorStatesTool(novaDir) }, optionalOperation: []string{"id", "actor_state"}},
		{name: "image presets", build: func() (agent.BaseTool, error) { return newWriteImagePresetsTool(novaDir) }, optionalOperation: []string{"id", "preset"}},
		{name: "automations", build: func() (agent.BaseTool, error) { return newWriteAutomationsTool(novaDir, cfg.Workspace, nil) }, optionalOperation: []string{"id", "task"}},
		{name: "skills", build: func() (agent.BaseTool, error) { return newWriteSkillsTool(cfg) }, optionalOperation: []string{"description", "agents", "content"}, requiredOperation: []string{"scope", "name"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			base, err := test.build()
			if err != nil {
				t.Fatal(err)
			}
			info, err := base.Info(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			schema, err := info.ToJSONSchema()
			if err != nil {
				t.Fatal(err)
			}
			if !configManagerSchemaRequires(schema.Required, "operations") || configManagerSchemaRequires(schema.Required, "message") {
				t.Fatalf("top-level required fields = %v, want operations without message", schema.Required)
			}
			operations, ok := schema.Properties.Get("operations")
			if !ok || operations.Items == nil {
				t.Fatalf("operations schema missing: %#v", operations)
			}
			if operations.MinItems == nil || *operations.MinItems != 1 {
				t.Fatalf("operations minItems = %v, want 1", operations.MinItems)
			}
			operation := operations.Items
			if !configManagerSchemaRequires(operation.Required, "op") {
				t.Fatalf("operation required fields = %v, want op", operation.Required)
			}
			for _, field := range test.optionalOperation {
				if configManagerSchemaRequires(operation.Required, field) {
					t.Fatalf("operation field %q must be conditional, required=%v", field, operation.Required)
				}
			}
			for _, field := range test.requiredOperation {
				if !configManagerSchemaRequires(operation.Required, field) {
					t.Fatalf("operation field %q must be required, required=%v", field, operation.Required)
				}
			}
		})
	}
}

func TestWriteAutomationsSchemaOnlyExposesEditableDefinition(t *testing.T) {
	base, err := newWriteAutomationsTool(t.TempDir(), t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	info, err := base.Info(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	schema, err := info.ToJSONSchema()
	if err != nil {
		t.Fatal(err)
	}
	operations, _ := schema.Properties.Get("operations")
	operation := operations.Items
	task, ok := operation.Properties.Get("task")
	if !ok {
		t.Fatal("write_automations schema is missing task")
	}
	if len(task.Required) != 0 {
		t.Fatalf("automation definition fields are action-dependent or defaulted, required=%v", task.Required)
	}
	for _, field := range []string{"revision", "target", "enabled", "name", "template", "prompt", "model_profile_id", "schedule", "triggers", "write_mode", "write_scope", "output_policy", "output_path"} {
		if _, exists := task.Properties.Get(field); !exists {
			t.Fatalf("editable automation field %q is missing", field)
		}
	}
	for _, field := range []string{"id", "catalog_id", "scope", "default_action_policy", "trigger_state", "last_run", "recent_runs", "created_at", "updated_at", "archived_at"} {
		if _, exists := task.Properties.Get(field); exists {
			t.Fatalf("runtime or derived automation field %q leaked into write schema", field)
		}
	}
	target, _ := task.Properties.Get("target")
	if !configManagerSchemaRequires(target.Required, "kind") || configManagerSchemaRequires(target.Required, "workspace") {
		t.Fatalf("target required fields = %v, want only kind", target.Required)
	}
	schedule, _ := task.Properties.Get("schedule")
	if len(schedule.Required) != 0 {
		t.Fatalf("defaulted schedule fields must be optional, required=%v", schedule.Required)
	}
	if _, exists := schedule.Properties.Get("cron"); exists {
		t.Fatal("derived cron field leaked into write schema")
	}
}

func TestWriteAutomationsAcceptsMinimalDefinitionAndRejectsUnknownFields(t *testing.T) {
	novaDir := t.TempDir()
	workspace := t.TempDir()
	writeTool, err := newWriteAutomationsTool(novaDir, workspace, nil)
	if err != nil {
		t.Fatal(err)
	}
	invokable := writeTool.(agent.InvokableTool)
	if _, err := invokable.InvokableRun(context.Background(), `{
		"operations": [{"op": "create", "task": {"target": {"kind": "user"}, "name": "Minimal"}}]
	}`); err != nil {
		t.Fatalf("minimal automation definition should use backend defaults: %v", err)
	}
	tasks, err := configManagerAutomationStore(novaDir, workspace, nil).List()
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 {
		t.Fatalf("created tasks = %d, want 1", len(tasks))
	}
	created := tasks[0]
	if created.Template != automation.TemplateCustomPrompt || created.WriteMode != automation.WriteModeReadOnly || created.OutputPolicy != automation.OutputPolicyRunRecordOnly {
		t.Fatalf("minimal definition did not receive defaults: %#v", created)
	}
	_, err = invokable.InvokableRun(context.Background(), `{
		"operations": [{"op": "create", "task": {"target": {"kind": "user"}, "created_at": "2026-01-01T00:00:00Z"}}]
	}`)
	if err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("runtime and unknown fields must remain rejected, got %v", err)
	}
}

func TestWriteAutomationsPartialUpdatePreservesOmittedDefinitionFields(t *testing.T) {
	novaDir := t.TempDir()
	workspace := t.TempDir()
	store := automation.NewStore(novaDir, workspace)
	created, err := store.Create(automation.Task{
		Target:   automation.ExecutionTarget{Kind: automation.TargetKindWorkspace, Workspace: workspace},
		Enabled:  true,
		Name:     "Before",
		Template: automation.TemplateReview,
		Prompt:   "keep this prompt",
	})
	if err != nil {
		t.Fatal(err)
	}
	name := "After"
	payload, err := json.Marshal(automationWriteInput{Operations: []automationWriteOperation{{
		Op: "update",
		ID: created.CatalogID,
		Task: &automationTaskWriteInput{
			Revision: created.Revision,
			Name:     &name,
		},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	writeTool, err := newWriteAutomationsTool(novaDir, workspace, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writeTool.(agent.InvokableTool).InvokableRun(context.Background(), string(payload)); err != nil {
		t.Fatal(err)
	}
	updated, err := store.Get(created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Name != name || !updated.Enabled || updated.Template != automation.TemplateReview || updated.Prompt != "keep this prompt" {
		t.Fatalf("partial update lost omitted definition fields: %#v", updated)
	}
}

func TestWriteAutomationsRejectsAStaleAgentDefinition(t *testing.T) {
	novaDir := filepath.Join(t.TempDir(), "user")
	workspace := filepath.Join(t.TempDir(), "book")
	store := automation.NewStore(novaDir, workspace)
	created, err := store.Create(automation.Task{
		Scope:    automation.ScopeWorkspace,
		Name:     "Original",
		Template: automation.TemplateReview,
		Prompt:   "original prompt",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpdateIfRevision(created.ID, automation.Task{Prompt: "user autosave"}, created.Revision); err != nil {
		t.Fatal(err)
	}
	staleAgentName := "stale Agent name"
	payload, err := json.Marshal(automationWriteInput{Operations: []automationWriteOperation{{
		Op: "update",
		ID: created.CatalogID,
		Task: &automationTaskWriteInput{
			Revision: created.Revision,
			Name:     &staleAgentName,
		},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	writeTool, err := newWriteAutomationsTool(novaDir, workspace, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = writeTool.(agent.InvokableTool).InvokableRun(context.Background(), string(payload))
	if err == nil || !strings.Contains(err.Error(), "revision conflict") {
		t.Fatalf("stale Agent update should fail with revision conflict, got %v", err)
	}
	latest, err := store.Get(created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if latest.Prompt != "user autosave" || latest.Name == "stale Agent name" {
		t.Fatalf("stale Agent update overwrote user definition: %#v", latest)
	}
}

func configManagerToolNameSet(t *testing.T, tools []agent.BaseTool) map[string]bool {
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

func configManagerSchemaRequires(required []string, field string) bool {
	for _, item := range required {
		if item == field {
			return true
		}
	}
	return false
}
