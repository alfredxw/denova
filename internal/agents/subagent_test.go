package agents

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"denova/config"
	agentdelegation "denova/internal/agents/delegation"
	"denova/internal/agents/harnessstate"
	agenttoolruntime "denova/internal/agents/toolruntime"

	agent "github.com/alfredxw/denova/agent"
	agentstate "github.com/alfredxw/denova/agent/state"
)

func TestConfigMaxIterationDefaultsToNativeUnlimited(t *testing.T) {
	if got := configMaxIteration(&config.Config{}); got != 0 {
		t.Fatalf("default max iteration = %d, want native unlimited zero", got)
	}
	if got := configMaxIteration(&config.Config{MaxIteration: 32}); got != 32 {
		t.Fatalf("configured max iteration = %d, want 32", got)
	}
}

func TestBuildAgentExposesGeneralAndConfiguredSubAgentsThroughTask(t *testing.T) {
	childWindow := 32_000
	cfg := &config.Config{
		DenovaDir:     filepath.Join(t.TempDir(), ".denova"),
		OpenAIBaseURL: "https://example.invalid",
		OpenAIModel:   "test-model",
		ModelProfiles: []config.ModelProfileSettings{{
			ID: "child-small", Model: "child-test-model", ContextWindowTokens: &childWindow,
		}},
		AgentTools: config.AgentToolSettings{
			Default: config.AgentToolOverride{
				config.AgentToolFilesystemRead: false,
				config.AgentToolWorkspaceWrite: false,
				config.AgentToolShell:          false,
				config.AgentToolSkills:         false,
				config.AgentToolLoreRead:       false,
				config.AgentToolLoreWrite:      false,
				config.AgentToolTodo:           false,
				config.AgentToolWebSearch:      false,
				config.AgentToolWebFetch:       false,
			},
		},
	}
	cfg.Labs.DeveloperMode = true
	manager, err := harnessstate.Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	current, err := manager.Store().Current(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Store().Update(context.Background(), agentstate.ChangeSet{
		BaseRevision: current.Revision,
		Changes: []agentstate.Change{{Path: "subagents/researcher.md", Content: []byte(`---
id: researcher
name: Researcher
description: Researches delegated context
parents: [ide]
model_profile: child-small
tools: []
---

Return concise findings.`)}},
	}); err != nil {
		t.Fatal(err)
	}
	definition, err := buildAgentDefinition(context.Background(), cfg, agentBuildSpec{
		Kind:        config.AgentKindIDE,
		Name:        "DenovaAgent",
		Description: "test",
		Composition: mustTestPromptComposition(t, config.AgentKindIDE, "test"),
	})
	if err != nil {
		t.Fatal(err)
	}
	catalog, ok := agentdelegation.AsCatalog(definition.Tools)
	if !ok {
		t.Fatal("root Definition did not expose the durable child catalog")
	}
	children := catalog.Children()
	if len(children) != 2 || children[0].Name != "general-purpose" || children[1].Name != "researcher" {
		names := make([]string, 0, len(children))
		for _, child := range children {
			names = append(names, child.Name)
		}
		t.Fatalf("delegated child names = %v", names)
	}
	var generalOrchestrator, rootOrchestrator *agenttoolruntime.OrchestratorMiddleware
	for _, middleware := range children[0].Definition.Middlewares {
		if current, ok := agent.MiddlewareImplementation(middleware).(*agenttoolruntime.OrchestratorMiddleware); ok {
			generalOrchestrator = current
		}
	}
	for _, middleware := range definition.Middlewares {
		if current, ok := agent.MiddlewareImplementation(middleware).(*agenttoolruntime.OrchestratorMiddleware); ok {
			rootOrchestrator = current
		}
	}
	if generalOrchestrator == nil || rootOrchestrator == nil || generalOrchestrator == rootOrchestrator {
		t.Fatalf("general/root orchestrators must be independent: general=%p root=%p", generalOrchestrator, rootOrchestrator)
	}
	for _, child := range children {
		if child.Definition.Goal != nil {
			t.Fatalf("delegated child %q unexpectedly inherited root Goal authority", child.Name)
		}
		if child.Definition.Cleanup == nil || child.Definition.Compaction == nil || child.Definition.ResultProcessor == nil || child.Definition.Permission == nil {
			t.Fatalf("delegated child %q lost public lifecycle capabilities: %#v", child.Name, child.Definition)
		}
		if child.Name == "researcher" {
			messages := []*agent.Message{agent.UserMessage("inspect the child Cleanup budget")}
			plan, planErr := child.Definition.Cleanup.Plan(context.Background(), agent.CleanupPlanRequest{
				Messages: messages, ModelRequest: messages, CompactionAvailable: true,
			})
			if planErr != nil {
				t.Fatal(planErr)
			}
			if plan.Metrics.ContextWindowTokens != childWindow {
				t.Fatalf("delegated child Cleanup window = %d, want model profile window %d", plan.Metrics.ContextWindowTokens, childWindow)
			}
		}
	}
}

func TestBuildSubAgentInstructionInheritsParentSystemPrompt(t *testing.T) {
	parentInstruction := "# Denova 运行时契约（不可覆盖）\n\n作品根目录：/tmp/book\n父级工具权限边界。"
	instruction := buildSubAgentInstruction(agentBuildSpec{
		Kind:        config.AgentKindIDE,
		Composition: mustTestPromptComposition(t, config.AgentKindIDE, parentInstruction),
	}, config.SubAgentConfig{
		ID:           "researcher",
		Name:         "Researcher",
		Description:  "Researches delegated context",
		SystemPrompt: "Return concise findings.",
	})

	for _, required := range []string{
		"Denova 运行时契约",
		"/tmp/book",
		"父级工具权限边界",
		"SubAgent-specific Instructions",
		"Researcher",
		"Researches delegated context",
		"Return concise findings.",
	} {
		if !strings.Contains(instruction, required) {
			t.Fatalf("subagent instruction missing %q:\n%s", required, instruction)
		}
	}
	if strings.Contains(instruction, "cannot override the parent Agent's runtime contract") {
		t.Fatalf("subagent prompt should inherit the parent contract without repeating a generic precedence wrapper:\n%s", instruction)
	}
	if parentIndex, subIndex := strings.Index(instruction, parentInstruction), strings.Index(instruction, "SubAgent-specific Instructions"); parentIndex < 0 || subIndex < 0 || parentIndex >= subIndex {
		t.Fatalf("parent prompt should appear before subagent prompt:\n%s", instruction)
	}
}

func TestBuildSubAgentInstructionInheritsInteractiveStoryBoundary(t *testing.T) {
	parentComposition := mustTestPromptComposition(t, config.AgentKindInteractiveStory, "互动故事父级内置规则")
	instruction := buildSubAgentInstruction(agentBuildSpec{
		Kind:        config.AgentKindInteractiveStory,
		Composition: parentComposition,
	}, config.SubAgentConfig{
		ID:           "story-researcher",
		Name:         "Story Researcher",
		Description:  "Reads story context for the parent.",
		SystemPrompt: "Only return context findings.",
	})

	for _, required := range []string{
		"Game Mode is read-only for workspace files",
		"Output only the story prose that can be shown on the story stage for this turn",
		"Only return context findings.",
	} {
		if !strings.Contains(instruction, required) {
			t.Fatalf("interactive subagent instruction missing %q:\n%s", required, instruction)
		}
	}
}

func TestBuildAgentCanDisableGeneralSubAgent(t *testing.T) {
	off := false
	definition, err := buildAgentDefinition(context.Background(), &config.Config{
		OpenAIBaseURL: "https://example.invalid",
		OpenAIModel:   "test-model",
		GeneralSubAgents: config.AgentGeneralSubAgentSettings{
			IDE: &off,
		},
		AgentTools: config.AgentToolSettings{
			Default: config.AgentToolOverride{
				config.AgentToolFilesystemRead: false,
				config.AgentToolWorkspaceWrite: false,
				config.AgentToolShell:          false,
				config.AgentToolSkills:         false,
				config.AgentToolLoreRead:       false,
				config.AgentToolLoreWrite:      false,
				config.AgentToolTodo:           false,
				config.AgentToolWebSearch:      false,
				config.AgentToolWebFetch:       false,
				config.AgentToolDelegation:     false,
			},
		},
	}, agentBuildSpec{
		Kind:        config.AgentKindIDE,
		Name:        "DenovaAgent",
		Description: "test",
		Composition: mustTestPromptComposition(t, config.AgentKindIDE, "test"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := agentdelegation.AsCatalog(definition.Tools); ok {
		t.Fatal("delegation catalog should be absent when no child Agent is enabled")
	}
	tools, err := definition.Tools.PrepareTools(context.Background(), agent.ToolRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if toolNamesForTest(t, tools)["task"] {
		t.Fatalf("task tool should be absent without any available subagent")
	}
}

func TestSubAgentAssemblyUsesParentToolPolicyKind(t *testing.T) {
	assembly, err := buildChatModelAgentAssembly(context.Background(), &config.Config{}, chatModelAgentAssemblySpec{
		Kind:           "researcher",
		ToolPolicyKind: config.AgentKindInteractiveStory,
		ToolSettings:   config.ResolvedAgentToolSettings{},
	})
	if err != nil {
		t.Fatal(err)
	}
	var orchestrator *agenttoolruntime.OrchestratorMiddleware
	for _, handler := range assembly.Middlewares {
		if middleware, ok := handler.(*agenttoolruntime.OrchestratorMiddleware); ok {
			orchestrator = middleware
			break
		}
	}
	if orchestrator == nil {
		t.Fatalf("expected tool orchestrator middleware")
	}
	if got := orchestrator.Configuration().PolicyKind; got != config.AgentKindInteractiveStory {
		t.Fatalf("subagent tool policy should use parent kind, got %q", got)
	}
}

func TestBuildChatModelAgentAssemblyPassesToolResultLimit(t *testing.T) {
	assembly, err := buildChatModelAgentAssembly(context.Background(), &config.Config{AgentToolResultLimitKB: 64}, chatModelAgentAssemblySpec{
		Kind:         config.AgentKindIDE,
		ToolSettings: config.ResolvedAgentToolSettings{},
	})
	if err != nil {
		t.Fatal(err)
	}
	var orchestrator *agenttoolruntime.OrchestratorMiddleware
	for _, handler := range assembly.Middlewares {
		if middleware, ok := handler.(*agenttoolruntime.OrchestratorMiddleware); ok {
			orchestrator = middleware
			break
		}
	}
	if orchestrator == nil {
		t.Fatalf("expected tool orchestrator middleware")
	}
	if got := orchestrator.Configuration().ToolResultMaxBytes; got != 64*1024 {
		t.Fatalf("tool result limit bytes = %d, want %d", got, 64*1024)
	}
}

func toolNamesForTest(t *testing.T, tools []agent.ToolDefinition) map[string]bool {
	t.Helper()
	names := make(map[string]bool, len(tools))
	for _, current := range tools {
		info, err := current.Tool.Info(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		names[info.Name] = true
	}
	return names
}
