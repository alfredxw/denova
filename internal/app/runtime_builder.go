package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/cloudwego/eino/adk"

	"denova/config"
	"denova/internal/agent"
	"denova/internal/book"
	"denova/internal/interactive"
	"denova/internal/prompts"
	"denova/internal/session"
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
	canonicalWorkspace, err := filepath.EvalSymlinks(absWorkspace)
	if err != nil {
		return nil, fmt.Errorf("解析工作目录真实路径失败: %w", err)
	}
	info, err := os.Stat(canonicalWorkspace)
	if err != nil || !info.IsDir() {
		return nil, fmt.Errorf("工作目录不存在: %s", canonicalWorkspace)
	}
	absWorkspace = filepath.Clean(canonicalWorkspace)

	state := book.NewState(absWorkspace)
	if err := state.InitWorkspace(); err != nil {
		return nil, fmt.Errorf("初始化工作目录失败: %w", err)
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
	interactiveStore := interactive.NewStoreWithNovaDir(absWorkspace, runtimeCfg.NovaDir)

	return &runtimeState{
		workspace:              absWorkspace,
		bookState:              state,
		bookService:            book.NewService(absWorkspace),
		interactive:            interactiveStore,
		sessionStore:           store,
		session:                sess,
		agentRunner:            agentRunner,
		interactiveStoryRunner: interactiveStoryRunner,
		versionService:         book.NewVersionService(absWorkspace),
	}, nil
}

func buildAgentRunner(ctx context.Context, cfg *config.Config, state *book.State, tellers ...agent.IDEStoryTeller) (*adk.Runner, error) {
	teller := ideStoryTellerForConfig(cfg)
	if len(tellers) > 0 {
		teller = tellers[0]
	}
	builtAgent, err := agent.Build(ctx, cfg, state, teller)
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

func ideStoryTellerFromInteractive(teller interactive.Teller, styleRules []agent.StyleRule) agent.IDEStoryTeller {
	if teller.ID == "" {
		return agent.IDEStoryTeller{}
	}
	return agent.IDEStoryTeller{
		ID:          teller.ID,
		Name:        teller.Name,
		Description: teller.Description,
		Prompt:      teller.PromptForTargets("system", "turn_context"),
		StyleRules:  styleRules,
	}
}

func buildInteractiveStoryRunner(ctx context.Context, cfg *config.Config, state *book.State, teller prompts.InteractiveStorySystemInstructionInput, toolContexts ...agent.InteractiveStoryToolContext) (*adk.Runner, error) {
	builtAgent, err := agent.BuildInteractiveStory(ctx, cfg, state, teller, toolContexts...)
	if err != nil {
		return nil, fmt.Errorf("构建互动故事 Agent 失败: %w", err)
	}
	return agent.NewRunnerWithOptions(ctx, builtAgent, agent.RunOptions{AgentKind: agent.AgentKindInteractiveStory, Workspace: cfg.Workspace}), nil
}

func buildConfigManagerRunner(ctx context.Context, cfg *config.Config, state *book.State, resourceSkills ...agent.ConfigManagerResourceSkill) (*adk.Runner, error) {
	builtAgent, err := agent.BuildConfigManagerAgent(ctx, cfg, state, resourceSkills...)
	if err != nil {
		return nil, fmt.Errorf("构建配置管理 Agent 失败: %w", err)
	}
	return agent.NewRunnerWithOptions(ctx, builtAgent, agent.RunOptions{AgentKind: agent.AgentKindConfigManager, Workspace: cfg.Workspace}), nil
}

func buildAutomationAgentRunner(ctx context.Context, cfg *config.Config, state *book.State, task agent.AutomationTaskInstruction) (*adk.Runner, error) {
	builtAgent, err := agent.BuildAutomationAgent(ctx, cfg, state, task)
	if err != nil {
		return nil, fmt.Errorf("构建自动化 Agent 失败: %w", err)
	}
	return agent.NewRunnerWithOptions(ctx, builtAgent, agent.RunOptions{AgentKind: agent.AgentKindAutomation, Workspace: cfg.Workspace}), nil
}

func buildImageAgentRunner(ctx context.Context, cfg *config.Config, state *book.State, systemPrompt string) (*adk.Runner, error) {
	builtAgent, err := agent.BuildImageAgent(ctx, cfg, state, systemPrompt)
	if err != nil {
		return nil, fmt.Errorf("构建图像 Agent 失败: %w", err)
	}
	return agent.NewRunnerWithOptions(ctx, builtAgent, agent.RunOptions{AgentKind: agent.AgentKindImage, Workspace: cfg.Workspace}), nil
}
