package agents

import (
	"context"
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
	todoTool, err := newToolCatalog(nil).WriteTodos()
	if err != nil {
		t.Fatal(err)
	}
	taskTool, err := newToolCatalog(nil).Task(ctx, []agent.Runnable{fakeAgent{name: producttools.GeneralSubAgentName, description: "test"}})
	if err != nil {
		t.Fatal(err)
	}
	builtAgent, err := agent.NewAgent(ctx, agent.AgentConfig{
		Name:          "tool-descriptor-native-agent-test",
		Description:   "verify the final model-visible tool surface",
		Instruction:   "Reply without calling tools.",
		Model:         chatModel,
		MaxIterations: 1,
		Tools:         []agent.BaseTool{todoTool, taskTool},
		Middlewares:   []agent.Middleware{producttools.NewDescriptorGuardMiddleware()},
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
	for _, name := range []string{"write_todos", "task"} {
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
		ExtraToolsFactory: newToolCatalog(cfg).IDE(),
		IncludeCompaction: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	todoTool, err := newToolCatalog(cfg).WriteTodos()
	if err != nil {
		t.Fatal(err)
	}
	taskTool, err := newToolCatalog(cfg).Task(ctx, []agent.Runnable{fakeAgent{name: producttools.GeneralSubAgentName, description: "test"}})
	if err != nil {
		t.Fatal(err)
	}
	tools := append([]agent.BaseTool(nil), assembly.Tools...)
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
		"write_todos", "task", "skill",
		"ls", "read_file", "glob", "grep", "write_file", "edit_file", "execute",
		"list_lore_items", "read_lore_items", "write_lore_items", "generate_image", "web_search",
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
	storyContext := InteractiveStoryToolContext{
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
	directorContext.MaintenanceTask = "director_plan_update"

	tests := []struct {
		name  string
		build func() ([]agent.BaseTool, error)
	}{
		{
			name: "writing",
			build: func() ([]agent.BaseTool, error) {
				return newToolCatalog(cfg).IDE()(config.ResolveAgentTools(cfg, config.AgentKindIDE))
			},
		},
		{
			name: "game",
			build: func() ([]agent.BaseTool, error) {
				return newToolCatalog(cfg).InteractiveStory(projectInteractiveToolContext(storyContext))(config.ResolveAgentTools(cfg, config.AgentKindInteractiveStory))
			},
		},
		{
			name: "game director",
			build: func() ([]agent.BaseTool, error) {
				return newToolCatalog(cfg).InteractiveDirector(projectInteractiveToolContext(directorContext))(config.ResolveAgentTools(cfg, config.AgentKindInteractiveDirector))
			},
		},
		{
			name: "config manager",
			build: func() ([]agent.BaseTool, error) {
				return newToolCatalog(cfg).ConfigManager()(config.ResolveAgentTools(cfg, config.AgentKindConfigManager))
			},
		},
		{
			name: "image",
			build: func() ([]agent.BaseTool, error) {
				return newToolCatalog(cfg).Image()(config.ResolveAgentTools(cfg, config.AgentKindImage))
			},
		},
		{
			name: "automation",
			build: func() ([]agent.BaseTool, error) {
				return newToolCatalog(cfg).Lore(false)(config.ResolveAgentTools(cfg, config.AgentKindAutomation))
			},
		},
		{name: "web search", build: newToolCatalog(cfg).WebSearch},
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
