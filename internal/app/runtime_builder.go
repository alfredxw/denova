package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/cloudwego/eino/adk"

	"nova/config"
	"nova/internal/agent"
	"nova/internal/book"
	"nova/internal/interactive"
	"nova/internal/prompts"
	"nova/internal/session"
)

type runtimeState struct {
	workspace              string
	bookState              *book.State
	bookService            *book.Service
	interactive            *interactive.Store
	sessionStore           *session.Store
	session                *session.Session
	agentRunner            *adk.Runner
	interactiveStoryRunner *adk.Runner
	versionService         *book.VersionService
}

func buildRuntime(ctx context.Context, cfg *config.Config, workspace string) (*runtimeState, error) {
	absWorkspace, err := filepath.Abs(workspace)
	if err != nil {
		return nil, fmt.Errorf("解析工作目录失败: %w", err)
	}

	state := book.NewState(absWorkspace)
	if err := state.InitWorkspace(); err != nil {
		return nil, fmt.Errorf("初始化工作目录失败: %w", err)
	}
	if err := os.MkdirAll(book.UserStyleDir(cfg.NovaDir), 0o755); err != nil {
		return nil, fmt.Errorf("初始化用户风格参考目录失败: %w", err)
	}

	store, err := session.NewStore(state.SessionDir())
	if err != nil {
		return nil, fmt.Errorf("创建会话存储失败: %w", err)
	}
	sess, err := activeUserSessionOrCreate(store)
	if err != nil {
		return nil, fmt.Errorf("创建会话失败: %w", err)
	}

	runtimeCfg := *cfg
	runtimeCfg.Workspace = absWorkspace
	agentRunner, err := buildAgentRunner(ctx, &runtimeCfg, state)
	if err != nil {
		return nil, err
	}
	interactiveStoryRunner, err := buildInteractiveStoryRunner(ctx, &runtimeCfg, state, prompts.InteractiveStorySystemInstructionInput{})
	if err != nil {
		return nil, err
	}

	return &runtimeState{
		workspace:              absWorkspace,
		bookState:              state,
		bookService:            book.NewServiceWithStyleRoot(absWorkspace, book.UserStyleDir(cfg.NovaDir)),
		interactive:            interactive.NewStore(absWorkspace),
		sessionStore:           store,
		session:                sess,
		agentRunner:            agentRunner,
		interactiveStoryRunner: interactiveStoryRunner,
		versionService:         book.NewVersionService(absWorkspace),
	}, nil
}

func buildAgentRunner(ctx context.Context, cfg *config.Config, state *book.State) (*adk.Runner, error) {
	builtAgent, err := agent.Build(ctx, cfg, state, ideStoryTellerForConfig(cfg))
	if err != nil {
		return nil, fmt.Errorf("构建 Agent 失败: %w", err)
	}
	return agent.NewRunnerWithOptions(ctx, builtAgent, agent.RunOptions{AgentKind: agent.AgentKindIDE, Workspace: cfg.Workspace}), nil
}

func ideStoryTellerForConfig(cfg *config.Config) agent.IDEStoryTeller {
	if cfg == nil || cfg.NovaDir == "" {
		return agent.IDEStoryTeller{}
	}
	tellerID := cfg.IDEStoryTellerID
	if tellerID == "" {
		tellerID = "classic"
	}
	teller := loadInteractiveTeller(cfg.NovaDir, tellerID)
	if teller.ID == "" {
		return agent.IDEStoryTeller{}
	}
	return agent.IDEStoryTeller{
		ID:          teller.ID,
		Name:        teller.Name,
		Description: teller.Description,
		Prompt:      teller.PromptForTargets("system", "turn_context"),
	}
}

func buildInteractiveStoryRunner(ctx context.Context, cfg *config.Config, state *book.State, teller prompts.InteractiveStorySystemInstructionInput, toolContexts ...agent.InteractiveStoryToolContext) (*adk.Runner, error) {
	builtAgent, err := agent.BuildInteractiveStory(ctx, cfg, state, teller, toolContexts...)
	if err != nil {
		return nil, fmt.Errorf("构建互动故事 Agent 失败: %w", err)
	}
	return agent.NewRunnerWithOptions(ctx, builtAgent, agent.RunOptions{AgentKind: agent.AgentKindInteractiveStory, Workspace: cfg.Workspace}), nil
}

func buildLoreAgentRunner(ctx context.Context, cfg *config.Config, state *book.State) (*adk.Runner, error) {
	builtAgent, err := agent.BuildLoreAgent(ctx, cfg, state)
	if err != nil {
		return nil, fmt.Errorf("构建资料库 Agent 失败: %w", err)
	}
	return agent.NewRunnerWithOptions(ctx, builtAgent, agent.RunOptions{AgentKind: agent.AgentKindLoreEditor, Workspace: cfg.Workspace}), nil
}

func buildAutomationAgentRunner(ctx context.Context, cfg *config.Config, state *book.State, task agent.AutomationTaskInstruction) (*adk.Runner, error) {
	builtAgent, err := agent.BuildAutomationAgent(ctx, cfg, state, task)
	if err != nil {
		return nil, fmt.Errorf("构建自动化 Agent 失败: %w", err)
	}
	return agent.NewRunnerWithOptions(ctx, builtAgent, agent.RunOptions{AgentKind: agent.AgentKindAutomation, Workspace: cfg.Workspace}), nil
}
