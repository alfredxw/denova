package agent

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/adk/prebuilt/deep"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"

	"denova/config"
	"denova/internal/interactive"
)

func TestDeepAgentBuiltInToolsPassDescriptorGuard(t *testing.T) {
	ctx := context.Background()
	chatModel := &descriptorGuardProbeModel{}
	builtAgent, err := deep.New(ctx, &deep.Config{
		Name:                   "tool-descriptor-deep-agent-test",
		Description:            "verify the final model-visible tool surface",
		Instruction:            "Reply without calling tools.",
		ChatModel:              chatModel,
		MaxIteration:           1,
		WithoutWriteTodos:      false,
		WithoutGeneralSubAgent: false,
		Handlers:               []adk.ChatModelAgentMiddleware{newToolDescriptorGuardMiddleware()},
	})
	if err != nil {
		t.Fatal(err)
	}

	iterator := adk.NewRunner(ctx, adk.RunnerConfig{Agent: builtAgent}).Query(ctx, "hello")
	for {
		event, ok := iterator.Next()
		if !ok {
			break
		}
		if event.Err != nil {
			t.Fatalf("deep agent rejected its final tool surface before the provider call: %v", event.Err)
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
			t.Fatalf("deep agent provider tool surface missing %q: %v", name, toolNames)
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
		ExtraToolsFactory: ideToolsFactory(cfg),
		IncludeCompaction: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	chatModel := &descriptorGuardProbeModel{}
	builtAgent, err := deep.New(ctx, &deep.Config{
		Name:                   "writing-tool-surface-test",
		Description:            "verify the writing Agent's final model-visible tool surface",
		Instruction:            "Reply without calling tools.",
		ChatModel:              chatModel,
		MaxIteration:           1,
		WithoutWriteTodos:      !settings.Todo,
		WithoutGeneralSubAgent: false,
		Handlers:               assembly.Handlers,
		ToolsConfig: adk.ToolsConfig{ToolsNodeConfig: compose.ToolsNodeConfig{
			Tools: assembly.Tools,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}

	iterator := adk.NewRunner(ctx, adk.RunnerConfig{Agent: builtAgent}).Query(ctx, "hello")
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
		build func() ([]tool.BaseTool, error)
	}{
		{
			name: "writing",
			build: func() ([]tool.BaseTool, error) {
				return ideToolsFactory(cfg)(config.ResolveAgentTools(cfg, config.AgentKindIDE))
			},
		},
		{
			name: "game",
			build: func() ([]tool.BaseTool, error) {
				return interactiveStoryToolsFactory(cfg, storyContext)(config.ResolveAgentTools(cfg, config.AgentKindInteractiveStory))
			},
		},
		{
			name: "game director",
			build: func() ([]tool.BaseTool, error) {
				return interactiveDirectorToolsFactory(cfg, directorContext)(config.ResolveAgentTools(cfg, config.AgentKindInteractiveDirector))
			},
		},
		{
			name: "config manager",
			build: func() ([]tool.BaseTool, error) {
				return configManagerToolsFactory(cfg)(config.ResolveAgentTools(cfg, config.AgentKindConfigManager))
			},
		},
		{
			name: "image",
			build: func() ([]tool.BaseTool, error) {
				return imageToolsFactory(cfg)(config.ResolveAgentTools(cfg, config.AgentKindImage))
			},
		},
		{
			name: "automation",
			build: func() ([]tool.BaseTool, error) {
				return loreToolsFactory(cfg, false)(config.ResolveAgentTools(cfg, config.AgentKindAutomation))
			},
		},
		{name: "web search", build: newWebSearchTools},
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
			if err := validateToolDescriptors(ctx, tools); err != nil {
				t.Fatalf("tool factory exposed an undeclared tool: %v", err)
			}
		})
	}
}

type descriptorGuardProbeModel struct {
	calls int
	tools []*schema.ToolInfo
}

func (m *descriptorGuardProbeModel) Generate(_ context.Context, _ []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	m.calls++
	m.tools = model.GetCommonOptions(&model.Options{}, opts...).Tools
	return schema.AssistantMessage("ok", nil), nil
}

func (m *descriptorGuardProbeModel) Stream(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	message, err := m.Generate(ctx, messages, opts...)
	if err != nil {
		return nil, err
	}
	return schema.StreamReaderFromArray([]*schema.Message{message}), nil
}
