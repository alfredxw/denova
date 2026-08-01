package agents

import (
	"context"
	agentinteractive "denova/internal/agents/interactive"
	agenttoolruntime "denova/internal/agents/toolruntime"
	"os"
	"path/filepath"
	"testing"

	agent "github.com/alfredxw/denova/agent"

	"denova/config"
	producttools "denova/internal/agents/tools"
	"denova/internal/interactive"
)

func TestNativeAgentBuiltInToolsPassDescriptorGuard(t *testing.T) {
	ctx := context.Background()
	chatModel := &descriptorGuardProbeModel{}
	todoTool, err := agenttoolruntime.NewCatalog(nil).Todo()
	if err != nil {
		t.Fatal(err)
	}
	taskTool, err := agenttoolruntime.NewCatalog(nil).Task(ctx, []agent.Runnable{fakeAgent{name: producttools.GeneralSubAgentName, description: "test"}})
	if err != nil {
		t.Fatal(err)
	}
	builtAgent, err := agent.NewAgent(ctx, agent.AgentConfig{
		Name:          "tool-descriptor-native-agent-test",
		Description:   "verify the final model-visible tool surface",
		Instruction:   "Reply without calling tools.",
		Model:         chatModel,
		MaxIterations: 1,
		Tools:         []agent.ToolDefinition{todoTool, taskTool},
	})
	if err != nil {
		t.Fatal(err)
	}

	iterator := agent.NewRunner(agent.RunnerConfig{Agent: builtAgent}).Query(ctx, "hello")
	for {
		event, ok := iterator.Next()
		if !ok {
			break
		}
		if event.Err != nil {
			t.Fatalf("native Agent rejected its final tool surface before the provider call: %v", event.Err)
		}
	}
	if chatModel.calls != 1 {
		t.Fatalf("provider calls = %d, want 1", chatModel.calls)
	}
	toolNames := make(map[string]bool, len(chatModel.tools))
	for _, info := range chatModel.tools {
		if info != nil {
			toolNames[info.Name] = true
		}
	}
	for _, name := range []string{"todo", "task"} {
		if !toolNames[name] {
			t.Fatalf("native Agent provider tool surface missing %q: %v", name, toolNames)
		}
	}
}

func TestWritingAgentFinalRuntimeToolSurfacePassesDescriptorGuard(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	skillsDir := filepath.Join(root, "skills")
	if err := os.MkdirAll(filepath.Join(skillsDir, "descriptor-check"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillsDir, "descriptor-check", "SKILL.md"), []byte("---\nname: descriptor-check\ndescription: Verify the runtime Skill tool surface.\n---\n\n# Descriptor check\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{
		Workspace:  workspace,
		SkillsDir:  skillsDir,
		AgentTools: config.DefaultAgentToolSettings(),
	}
	cfg.SetDataDir(filepath.Join(root, "data"))
	settings := config.ResolveAgentTools(cfg, config.AgentKindIDE)
	assembly, err := buildChatModelAgentAssembly(ctx, cfg, chatModelAgentAssemblySpec{
		Kind:              config.AgentKindIDE,
		ToolSettings:      settings,
		EnableSkills:      true,
		ExtraToolsFactory: agenttoolruntime.NewCatalog(cfg).IDE(),
		IncludeCompaction: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	todoTool, err := agenttoolruntime.NewCatalog(cfg).Todo()
	if err != nil {
		t.Fatal(err)
	}
	taskTool, err := agenttoolruntime.NewCatalog(cfg).Task(ctx, []agent.Runnable{fakeAgent{name: producttools.GeneralSubAgentName, description: "test"}})
	if err != nil {
		t.Fatal(err)
	}
	tools := append([]agent.ToolDefinition(nil), assembly.Tools...)
	tools = append(tools, todoTool, taskTool)

	chatModel := &descriptorGuardProbeModel{}
	builtAgent, err := agent.NewAgent(ctx, agent.AgentConfig{
		Name:          "writing-tool-surface-test",
		Description:   "verify the writing Agent's final model-visible tool surface",
		Instruction:   "Reply without calling tools.",
		Model:         chatModel,
		MaxIterations: 1,
		Middlewares:   assembly.Middlewares,
		Tools:         tools,
	})
	if err != nil {
		t.Fatal(err)
	}

	iterator := agent.NewRunner(agent.RunnerConfig{Agent: builtAgent}).Query(ctx, "hello")
	for {
		event, ok := iterator.Next()
		if !ok {
			break
		}
		if event.Err != nil {
			t.Fatalf("writing Agent rejected its final tool surface: %v", event.Err)
		}
	}
	toolNames := make(map[string]bool, len(chatModel.tools))
	for _, info := range chatModel.tools {
		if info != nil {
			toolNames[info.Name] = true
		}
	}
	for _, name := range []string{
		"todo", "task", "skill",
		"read", "glob", "grep", "write", "edit", "bash",
		"list_lore_items", "read_lore_items", "write_lore_items", "generate_image", "web_search", "web_fetch",
	} {
		if !toolNames[name] {
			t.Fatalf("writing Agent provider tool surface missing %q: %v", name, toolNames)
		}
	}
}

func TestProductToolFactoriesDeclareEveryConcreteTool(t *testing.T) {
	ctx := context.Background()
	cfg := &config.Config{Workspace: t.TempDir()}
	cfg.SetDataDir(t.TempDir())
	storyContext := agentinteractive.InteractiveStoryToolContext{
		Store:     interactive.NewStore(t.TempDir()),
		StoryID:   "story-tool-descriptor-test",
		BranchID:  "main",
		TurnID:    "turn-1",
		CommandID: "command-tool-descriptor-test",
		SubmitStateSchemaBatch: func(context.Context, interactive.ActorStateSchemaBatch) (interactive.ActorStateSchemaBatchResult, error) {
			return interactive.ActorStateSchemaBatchResult{}, nil
		},
		SubmitDirectorPlanUpdate: func(context.Context, interactive.DirectorPlanUpdateSubmission) (interactive.DirectorPlanUpdateReceipt, error) {
			return interactive.DirectorPlanUpdateReceipt{}, nil
		},
		PrepareTurn: func(context.Context, interactive.TurnCheckRequest) (interactive.RuleResolution, error) {
			return interactive.RuleResolution{}, nil
		},
		SubmitTurnResult: func(context.Context, interactive.TurnSubmissionInput) (interactive.TurnSubmissionReceipt, error) {
			return interactive.TurnSubmissionReceipt{}, nil
		},
	}
	directorContext := storyContext
	// Descriptor validation does not need a persisted story. Event-card scope
	// construction is covered separately with a real event catalog.
	directorContext.MaintenanceTask = ""

	tests := []struct {
		name  string
		build func() ([]agent.ToolDefinition, error)
	}{
		{
			name: "writing",
			build: func() ([]agent.ToolDefinition, error) {
				return agenttoolruntime.NewCatalog(cfg).IDE()(config.ResolveAgentTools(cfg, config.AgentKindIDE))
			},
		},
		{
			name: "game",
			build: func() ([]agent.ToolDefinition, error) {
				return agenttoolruntime.NewCatalog(cfg).InteractiveStory(agenttoolruntime.ProjectInteractiveContext(storyContext))(config.ResolveAgentTools(cfg, config.AgentKindInteractiveStory))
			},
		},
		{
			name: "game director",
			build: func() ([]agent.ToolDefinition, error) {
				return agenttoolruntime.NewCatalog(cfg).InteractiveDirector(agenttoolruntime.ProjectInteractiveContext(directorContext))(config.ResolveAgentTools(cfg, config.AgentKindInteractiveDirector))
			},
		},
		{
			name: "config manager",
			build: func() ([]agent.ToolDefinition, error) {
				return agenttoolruntime.NewCatalog(cfg).ConfigManager()(config.ResolveAgentTools(cfg, config.AgentKindConfigManager))
			},
		},
		{
			name: "image",
			build: func() ([]agent.ToolDefinition, error) {
				return agenttoolruntime.NewCatalog(cfg).Image()(config.ResolveAgentTools(cfg, config.AgentKindImage))
			},
		},
		{
			name: "automation",
			build: func() ([]agent.ToolDefinition, error) {
				return agenttoolruntime.NewCatalog(cfg).Lore(false)(config.ResolveAgentTools(cfg, config.AgentKindAutomation))
			},
		},
		{name: "web access", build: func() ([]agent.ToolDefinition, error) {
			return agenttoolruntime.NewCatalog(cfg).WebAccess(config.ResolveAgentTools(cfg, config.AgentKindIDE))
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tools, err := tt.build()
			if err != nil {
				t.Fatal(err)
			}
			if len(tools) == 0 {
				t.Fatal("tool factory returned no tools")
			}
			if err := producttools.Validate(ctx, tools); err != nil {
				t.Fatalf("tool factory exposed an undeclared tool: %v", err)
			}
		})
	}
}

func TestRestrictedAgentOverridesDoNotReachGenericToolCatalog(t *testing.T) {
	forcedDirectorTools := config.AgentToolOverride{
		config.AgentToolWorkspaceRead:  true,
		config.AgentToolWorkspaceWrite: true,
		config.AgentToolShell:          true,
		config.AgentToolWebSearch:      true,
		config.AgentToolWebFetch:       true,
		config.AgentToolBrowser:        true,
	}
	cfg := &config.Config{
		Workspace: t.TempDir(),
		AgentTools: config.AgentToolSettings{
			InteractiveStory: config.AgentToolOverride{
				config.AgentToolWorkspaceWrite: true,
				config.AgentToolShell:          true,
			},
			InteractiveDirector: forcedDirectorTools,
		},
	}
	catalog := agenttoolruntime.NewCatalog(cfg)

	tests := []struct {
		kind             string
		workspaceReadOK  bool
		forcedCapability []string
	}{
		{
			kind:             config.AgentKindInteractiveStory,
			workspaceReadOK:  true,
			forcedCapability: []string{config.AgentToolWorkspaceWrite, config.AgentToolShell},
		},
		{
			kind: config.AgentKindInteractiveDirector,
			forcedCapability: []string{
				config.AgentToolWorkspaceRead, config.AgentToolWorkspaceWrite, config.AgentToolShell,
				config.AgentToolWebSearch, config.AgentToolWebFetch, config.AgentToolBrowser,
			},
		},
	}
	for _, test := range tests {
		t.Run(test.kind, func(t *testing.T) {
			settings := config.ResolveAgentTools(cfg, test.kind)
			for _, capability := range test.forcedCapability {
				if settings.Allows(capability) {
					t.Fatalf("forced capability %s escaped the %s ceiling", capability, test.kind)
				}
			}

			definitions, err := catalog.Workspace(settings)
			if err != nil {
				t.Fatal(err)
			}
			web, err := catalog.WebAccess(settings)
			if err != nil {
				t.Fatal(err)
			}
			definitions = append(definitions, web...)
			browser, err := catalog.Browser(context.Background(), settings)
			if err != nil {
				t.Fatal(err)
			}
			definitions = append(definitions, browser...)
			names := toolNameSet(t, definitions)
			if got := names["read"]; got != test.workspaceReadOK {
				t.Fatalf("%s workspace read registration = %t, want %t; names=%v", test.kind, got, test.workspaceReadOK, names)
			}
			for _, name := range []string{"write", "edit", "bash", "pwsh", "web_search", "web_fetch", "browser"} {
				if names[name] {
					t.Fatalf("%s registered forbidden generic tool %s after a forced override: %v", test.kind, name, names)
				}
			}
		})
	}
}

type descriptorGuardProbeModel struct {
	calls int
	tools []*agent.ToolInfo
}

func (m *descriptorGuardProbeModel) Generate(_ context.Context, _ []*agent.Message, opts ...agent.ModelOption) (*agent.Message, error) {
	m.calls++
	m.tools = agent.GetCommonOptions(&agent.Options{}, opts...).Tools
	return agent.AssistantMessage("ok", nil), nil
}

func (m *descriptorGuardProbeModel) Stream(ctx context.Context, messages []*agent.Message, opts ...agent.ModelOption) (*agent.StreamReader[*agent.Message], error) {
	message, err := m.Generate(ctx, messages, opts...)
	if err != nil {
		return nil, err
	}
	return agent.StreamReaderFromArray([]*agent.Message{message}), nil
}
