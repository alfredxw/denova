package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"denova/config"
	agents "denova/internal/agents"
	"denova/internal/agents/session"
	"denova/internal/book"
	"denova/internal/interactive"
	"denova/internal/narrativestyle"
	"denova/internal/prompts"
	"denova/internal/workspacechange"
)

type runtimeState struct {
	workspace              string
	bookState              *book.State
	bookService            *book.Service
	interactive            *interactive.Store
	sessionStore           *session.Store
	session                *session.Session
	agentRunner            *agents.Runner
	interactiveStoryRunner *agents.Runner
	versionService         *book.VersionService
}

// buildRuntimeExclusively initializes a runtime while holding the same
// per-workspace mutation boundary used by editors and agents. This matters for
// inactive automation targets too: selecting that workspace cannot rebuild
// session/story projections concurrently with a background write.
func buildRuntimeExclusively(ctx context.Context, cfg *config.Config, workspace string) (*runtimeState, error) {
	changes, err := workspacechange.ForWorkspace(workspace)
	if err != nil {
		return nil, err
	}
	var runtime *runtimeState
	err = changes.WithExclusiveWorkspace(ctx, func() error {
		var buildErr error
		runtime, buildErr = buildRuntime(ctx, cfg, workspace)
		return buildErr
	})
	return runtime, err
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
	keepStore := false
	defer func() {
		if !keepStore {
			_ = store.Close()
		}
	}()
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
	interactiveStore := interactive.NewStoreWithNovaDir(absWorkspace, runtimeCfg.DataDir())

	runtime := &runtimeState{
		workspace:              absWorkspace,
		bookState:              state,
		bookService:            book.NewService(absWorkspace),
		interactive:            interactiveStore,
		sessionStore:           store,
		session:                sess,
		agentRunner:            agentRunner,
		interactiveStoryRunner: interactiveStoryRunner,
		versionService:         book.NewVersionService(absWorkspace),
	}
	keepStore = true
	return runtime, nil
}

func buildAgentRunner(ctx context.Context, cfg *config.Config, state *book.State, tellers ...agents.IDEStoryTeller) (*agents.Runner, error) {
	runner, _, err := buildAgentRunnerWithComposition(ctx, cfg, state, tellers...)
	return runner, err
}

func buildAgentRunnerWithComposition(ctx context.Context, cfg *config.Config, state *book.State, tellers ...agents.IDEStoryTeller) (*agents.Runner, agents.SystemPromptComposition, error) {
	teller := ideStoryTellerForConfig(cfg)
	if len(tellers) > 0 {
		teller = tellers[0]
	}
	builtAgent, composition, err := agents.BuildWithCompositionForHost(ctx, cfg, state, teller, agents.AgentHostCapabilities{Interactive: true})
	if err != nil {
		return nil, agents.SystemPromptComposition{}, fmt.Errorf("构建 Agent 失败: %w", err)
	}
	return agents.NewRunnerWithOptions(ctx, builtAgent, agents.RunOptions{AgentKind: agents.AgentKindIDE, Workspace: cfg.Workspace}), composition, nil
}

func ideStoryTellerForConfig(cfg *config.Config) agents.IDEStoryTeller {
	if cfg == nil || cfg.DataDir() == "" {
		return agents.IDEStoryTeller{}
	}
	tellerID := cfg.IDEStoryTellerID
	if tellerID == "" {
		tellerID = narrativestyle.DefaultID
	}
	teller := loadWritingTeller(cfg.DataDir(), tellerID)
	if teller.ID == "" {
		return agents.IDEStoryTeller{}
	}
	return agents.IDEStoryTeller{
		ID:          teller.ID,
		Name:        teller.Name,
		Description: teller.Description,
		Prompt:      teller.PromptForTargets("system", "turn_context"),
	}
}

func ideStoryTellerFromInteractive(teller interactive.Teller, styleRules []agents.StyleRule) agents.IDEStoryTeller {
	if teller.ID == "" {
		return agents.IDEStoryTeller{}
	}
	return agents.IDEStoryTeller{
		ID:          teller.ID,
		Name:        teller.Name,
		Description: teller.Description,
		Prompt:      teller.PromptForTargets("system", "turn_context"),
		StyleRules:  styleRules,
	}
}

func buildInteractiveStoryRunner(ctx context.Context, cfg *config.Config, state *book.State, teller prompts.InteractiveStorySystemInstructionInput, toolContexts ...agents.InteractiveStoryToolContext) (*agents.Runner, error) {
	runner, _, err := buildInteractiveStoryRunnerWithComposition(ctx, cfg, state, teller, toolContexts...)
	return runner, err
}

func buildInteractiveStoryRunnerWithComposition(ctx context.Context, cfg *config.Config, state *book.State, teller prompts.InteractiveStorySystemInstructionInput, toolContexts ...agents.InteractiveStoryToolContext) (*agents.Runner, agents.SystemPromptComposition, error) {
	builtAgent, composition, err := agents.BuildInteractiveStoryWithComposition(ctx, cfg, state, teller, toolContexts...)
	if err != nil {
		return nil, agents.SystemPromptComposition{}, fmt.Errorf("构建互动故事 Agent 失败: %w", err)
	}
	return agents.NewRunnerWithOptions(ctx, builtAgent, agents.RunOptions{AgentKind: agents.AgentKindInteractiveStory, Workspace: cfg.Workspace}), composition, nil
}

func buildConfigManagerRunner(ctx context.Context, cfg *config.Config, state *book.State, resourceSkills ...agents.ConfigManagerResourceSkill) (*agents.Runner, error) {
	runner, _, err := buildConfigManagerRunnerWithComposition(ctx, cfg, state, resourceSkills...)
	return runner, err
}

func buildConfigManagerRunnerWithComposition(ctx context.Context, cfg *config.Config, state *book.State, resourceSkills ...agents.ConfigManagerResourceSkill) (*agents.Runner, agents.SystemPromptComposition, error) {
	builtAgent, composition, err := agents.BuildConfigManagerAgentWithCompositionForHost(ctx, cfg, state, agents.AgentHostCapabilities{Interactive: true}, resourceSkills...)
	if err != nil {
		return nil, agents.SystemPromptComposition{}, fmt.Errorf("构建配置管理 Agent 失败: %w", err)
	}
	return agents.NewRunnerWithOptions(ctx, builtAgent, agents.RunOptions{AgentKind: agents.AgentKindConfigManager, Workspace: cfg.Workspace}), composition, nil
}

func buildAutomationAgentRunner(ctx context.Context, cfg *config.Config, state *book.State, task agents.AutomationTaskInstruction) (*agents.Runner, error) {
	runner, _, err := buildAutomationAgentRunnerWithComposition(ctx, cfg, state, task)
	return runner, err
}

func buildAutomationAgentRunnerWithComposition(ctx context.Context, cfg *config.Config, state *book.State, task agents.AutomationTaskInstruction) (*agents.Runner, agents.SystemPromptComposition, error) {
	builtAgent, composition, err := agents.BuildAutomationAgentWithComposition(ctx, cfg, state, task)
	if err != nil {
		return nil, agents.SystemPromptComposition{}, fmt.Errorf("构建自动化 Agent 失败: %w", err)
	}
	return agents.NewRunnerWithOptions(ctx, builtAgent, agents.RunOptions{AgentKind: agents.AgentKindAutomation, Workspace: cfg.Workspace}), composition, nil
}

func buildImageAgentRunner(ctx context.Context, cfg *config.Config, state *book.State, systemPrompt string) (*agents.Runner, error) {
	runner, _, err := buildImageAgentRunnerWithComposition(ctx, cfg, state, systemPrompt)
	return runner, err
}

func buildImageAgentRunnerWithComposition(ctx context.Context, cfg *config.Config, state *book.State, systemPrompt string) (*agents.Runner, agents.SystemPromptComposition, error) {
	builtAgent, composition, err := agents.BuildImageAgentWithComposition(ctx, cfg, state, systemPrompt)
	if err != nil {
		return nil, agents.SystemPromptComposition{}, fmt.Errorf("构建图像 Agent 失败: %w", err)
	}
	return agents.NewRunnerWithOptions(ctx, builtAgent, agents.RunOptions{AgentKind: agents.AgentKindImage, Workspace: cfg.Workspace}), composition, nil
}
