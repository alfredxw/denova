package agents

import (
	"context"
	agentdelegation "denova/internal/agents/delegation"
	agentinteractive "denova/internal/agents/interactive"
	agenttoolruntime "denova/internal/agents/toolruntime"
	"os"
	"path/filepath"
	"testing"

	agent "github.com/alfredxw/denova/agent"
	publictools "github.com/alfredxw/denova/agent/tools"

	"denova/config"
	producttools "denova/internal/agents/tools"
	"denova/internal/interactive"
)

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
		Workspace:     workspace,
		SkillsDir:     skillsDir,
		OpenAIBaseURL: "https://example.invalid",
		OpenAIModel:   "descriptor-test-model",
		AgentTools:    config.DefaultAgentToolSettings(),
	}
	cfg.SetDataDir(filepath.Join(root, "data"))
	definition, err := buildAgentDefinition(ctx, cfg, agentBuildSpec{
		Kind: config.AgentKindIDE, Name: "DenovaAgent", Description: "tool surface test",
		Composition:       mustTestPromptComposition(t, config.AgentKindIDE, "Reply without calling tools."),
		EnableSkills:      true,
		ExtraToolsFactory: agenttoolruntime.NewCatalog(cfg).IDE(),
	})
	if err != nil {
		t.Fatal(err)
	}
	toolset := definition.Tools
	if catalog, ok := agentdelegation.AsCatalog(toolset); ok {
		children := catalog.Children()
		candidates := make([]publictools.LocalTaskAgent, len(children))
		for index, child := range children {
			candidates[index] = publictools.LocalTaskAgent{
				Name: child.Name, Description: child.Description,
				Opener: descriptorTestSessionOpener{}, Identity: child.Identity,
			}
		}
		executor, bindErr := publictools.NewLocalTasks(candidates...)
		if bindErr != nil {
			t.Fatal(bindErr)
		}
		toolset, bindErr = catalog.Bind(executor)
		if bindErr != nil {
			t.Fatal(bindErr)
		}
	}
	tools, err := toolset.PrepareTools(ctx, agent.ToolRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := agent.NewRegistry(ctx, tools...); err != nil {
		t.Fatalf("writing Agent rejected its final tool surface: %v", err)
	}
	toolNames := toolNameSet(t, tools)
	for _, name := range []string{
		"todo", "task", "skill",
		"read", "glob", "grep", "write", "edit", "bash",
		"list_lore_items", "read_lore_items", "write_lore_items", "generate_image", "web_search", "web_fetch",
	} {
		if !toolNames[name] {
			t.Fatalf("writing Agent provider tool surface missing %q: %v", name, toolNames)
		}
	}
	if catalog, ok := agentdelegation.AsCatalog(definition.Tools); !ok || len(catalog.Children()) == 0 {
		t.Fatal("writing Agent did not retain a durable task catalog")
	}
}

type descriptorTestSessionOpener struct{}

func (descriptorTestSessionOpener) Session(context.Context, agent.SessionKey) (*agent.Session, error) {
	return nil, nil
}

func (descriptorTestSessionOpener) ListSessions(context.Context, agent.SessionSelector) ([]agent.SessionKey, error) {
	return nil, nil
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
		config.AgentToolFilesystemRead: true,
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
		filesystemReadOK bool
		forcedCapability []string
	}{
		{
			kind:             config.AgentKindInteractiveStory,
			filesystemReadOK: true,
			forcedCapability: []string{config.AgentToolWorkspaceWrite, config.AgentToolShell},
		},
		{
			kind: config.AgentKindInteractiveDirector,
			forcedCapability: []string{
				config.AgentToolFilesystemRead, config.AgentToolWorkspaceWrite, config.AgentToolShell,
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
			if got := names["read"]; got != test.filesystemReadOK {
				t.Fatalf("%s workspace read registration = %t, want %t; names=%v", test.kind, got, test.filesystemReadOK, names)
			}
			for _, name := range []string{"write", "edit", "bash", "pwsh", "web_search", "web_fetch", "browser"} {
				if names[name] {
					t.Fatalf("%s registered forbidden generic tool %s after a forced override: %v", test.kind, name, names)
				}
			}
		})
	}
}
