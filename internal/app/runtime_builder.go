package app

import (
	"context"
	agentchat "denova/internal/agents/chat"
	agentconversation "denova/internal/agents/conversation"
	agentinteractive "denova/internal/agents/interactive"
	"fmt"
	"os"
	"path/filepath"

	"denova/config"
	agents "denova/internal/agents"
	"denova/internal/agents/prompts"
	agentrun "denova/internal/agents/run"
	"denova/internal/agents/session"
	"denova/internal/book"
	"denova/internal/interactive"
	"denova/internal/interactive/teller"
	"denova/internal/style"
	workspacechange "denova/internal/workspace/change"
)

type runtimeState struct {
	projectID              string
	projectStateRoot       string
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
func buildRuntimeExclusively(ctx context.Context, cfg *config.Config, layout ProjectLayout) (*runtimeState, error) {
	workspace := layout.ContentRoot
	changes, err := workspacechange.ForWorkspaceAt(workspace, layout.StateRoot)
	if err != nil {
		return nil, err
	}
	var runtime *runtimeState
	err = changes.WithExclusiveWorkspace(ctx, func() error {
		var buildErr error
		runtime, buildErr = buildRuntime(ctx, cfg, layout)
		return buildErr
	})
	return runtime, err
}

func buildRuntime(ctx context.Context, cfg *config.Config, layout ProjectLayout) (*runtimeState, error) {
	workspace := layout.ContentRoot
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
	store, err := session.NewStore(layout.SessionsDir())
	if err != nil {
		return nil, fmt.Errorf("创建会话存储失败: %w", err)
	}
	keepStore := false
	defer func() {
		if !keepStore {
			_ = store.Close()
		}
	}()
	runtimeCfg := *cfg
	runtimeCfg.Workspace = absWorkspace
	runtimeCfg.ProjectID = layout.ProjectID
	runtimeCfg.ProjectStateDir = layout.StateRoot
	if layered, loadErr := config.LoadLayeredWithStartupConfigAt(runtimeCfg.DataDir(), absWorkspace, layout.ConfigPath()); loadErr == nil {
		applyLayeredSettingsToConfig(&runtimeCfg, layered)
	} else {
		return nil, fmt.Errorf("加载 Project 配置失败: %w", loadErr)
	}
	sess, err := activeUserSessionOrCreate(store, &runtimeCfg)
	if err != nil {
		return nil, fmt.Errorf("创建会话失败: %w", err)
	}
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
		projectID:              layout.ProjectID,
		projectStateRoot:       layout.StateRoot,
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

func buildAgentRunner(ctx context.Context, cfg *config.Config, state *book.State, tellers ...prompts.IDEStoryTeller) (*agents.Runner, error) {
	runner, _, err := buildAgentRunnerWithComposition(ctx, cfg, state, tellers...)
	return runner, err
}

func buildAgentRunnerWithComposition(ctx context.Context, cfg *config.Config, state *book.State, tellers ...prompts.IDEStoryTeller) (*agents.Runner, prompts.SystemPromptComposition, error) {
	teller := ideStoryTellerForConfig(cfg)
	if len(tellers) > 0 {
		teller = tellers[0]
	}
	builtAgent, composition, err := agents.BuildWithCompositionForHost(ctx, cfg, state, teller, agents.AgentHostCapabilities{Interactive: true})
	if err != nil {
		return nil, prompts.SystemPromptComposition{}, fmt.Errorf("构建 Agent 失败: %w", err)
	}
	return agentchat.NewRunnerWithOptions(ctx, builtAgent, agentrun.Options{AgentKind: agentrun.AgentKindIDE, Workspace: cfg.Workspace}), composition, nil
}

func buildProjectAgentRunnerWithComposition(ctx context.Context, runtime ideChatRuntime) (*agents.Runner, prompts.SystemPromptComposition, error) {
	if runtime.agentKind == agentrun.AgentKindGeneral {
		builtAgent, composition, err := agents.BuildGeneralAgentWithCompositionForHost(
			ctx, &runtime.cfg, agents.AgentHostCapabilities{Interactive: true},
		)
		if err != nil {
			return nil, prompts.SystemPromptComposition{}, fmt.Errorf("build General Agent: %w", err)
		}
		return agentchat.NewRunnerWithOptions(ctx, builtAgent, agentrun.Options{
			AgentKind: agentrun.AgentKindGeneral, ProjectID: runtime.projectID,
			StateRoot: runtime.projectState, Workspace: runtime.workspace,
		}), composition, nil
	}
	builtAgent, composition, err := agents.BuildWithCompositionForHost(
		ctx, &runtime.cfg, runtime.state, runtime.ideTeller,
		agents.AgentHostCapabilities{Interactive: true},
	)
	if err != nil {
		return nil, prompts.SystemPromptComposition{}, fmt.Errorf("build Writing Agent: %w", err)
	}
	return agentchat.NewRunnerWithOptions(ctx, builtAgent, agentrun.Options{
		AgentKind: agentrun.AgentKindIDE, ProjectID: runtime.projectID,
		StateRoot: runtime.projectState, Workspace: runtime.workspace,
	}), composition, nil
}

func projectSessionConversation(runtime ideChatRuntime, req agentchat.ChatRequest) *agentconversation.SessionConversation {
	if runtime.agentKind == agentrun.AgentKindGeneral {
		return agentconversation.NewSessionConversationForAgent(runtime.sess, &runtime.cfg, config.AgentKindGeneral)
	}
	runtimeContexts := prompts.IDEWorkspaceRuntimeContextsForContext(runtime.state, req.IDEContext)
	return agentconversation.NewSessionConversationForAgentWithRuntimeContexts(
		runtime.sess, &runtime.cfg, config.AgentKindIDE,
		runtimeContexts.StableTitle, runtimeContexts.Stable,
		runtimeContexts.DynamicTitle, runtimeContexts.Dynamic,
	)
}

func ideStoryTellerForConfig(cfg *config.Config) prompts.IDEStoryTeller {
	if cfg == nil || cfg.DataDir() == "" {
		return prompts.IDEStoryTeller{}
	}
	tellerID := cfg.IDEStoryTellerID
	if tellerID == "" {
		tellerID = style.DefaultID
	}
	teller := loadWritingTeller(cfg.DataDir(), tellerID)
	if teller.ID == "" {
		return prompts.IDEStoryTeller{}
	}
	return prompts.IDEStoryTeller{
		ID:          teller.ID,
		Name:        teller.Name,
		Description: teller.Description,
		Prompt:      teller.PromptForTargets("system", "turn_context"),
	}
}

func ideStoryTellerFromInteractive(teller teller.Definition, styleRules []prompts.StyleRule) prompts.IDEStoryTeller {
	if teller.ID == "" {
		return prompts.IDEStoryTeller{}
	}
	return prompts.IDEStoryTeller{
		ID:          teller.ID,
		Name:        teller.Name,
		Description: teller.Description,
		Prompt:      teller.PromptForTargets("system", "turn_context"),
		StyleRules:  styleRules,
	}
}

func buildInteractiveStoryRunner(ctx context.Context, cfg *config.Config, state *book.State, teller prompts.InteractiveStorySystemInstructionInput, toolContexts ...agentinteractive.InteractiveStoryToolContext) (*agents.Runner, error) {
	runner, _, err := buildInteractiveStoryRunnerWithComposition(ctx, cfg, state, teller, toolContexts...)
	return runner, err
}

func buildInteractiveStoryRunnerWithComposition(ctx context.Context, cfg *config.Config, state *book.State, teller prompts.InteractiveStorySystemInstructionInput, toolContexts ...agentinteractive.InteractiveStoryToolContext) (*agents.Runner, prompts.SystemPromptComposition, error) {
	builtAgent, composition, err := agents.BuildInteractiveStoryWithComposition(ctx, cfg, state, teller, toolContexts...)
	if err != nil {
		return nil, prompts.SystemPromptComposition{}, fmt.Errorf("构建互动故事 Agent 失败: %w", err)
	}
	return agentchat.NewRunnerWithOptions(ctx, builtAgent, agentrun.Options{AgentKind: agentrun.AgentKindInteractiveStory, Workspace: cfg.Workspace}), composition, nil
}

func buildConfigManagerRunner(ctx context.Context, cfg *config.Config, state *book.State, resourceSkills ...prompts.ConfigManagerResourceSkill) (*agents.Runner, error) {
	runner, _, err := buildConfigManagerRunnerWithComposition(ctx, cfg, state, resourceSkills...)
	return runner, err
}

func buildConfigManagerRunnerWithComposition(ctx context.Context, cfg *config.Config, state *book.State, resourceSkills ...prompts.ConfigManagerResourceSkill) (*agents.Runner, prompts.SystemPromptComposition, error) {
	builtAgent, composition, err := agents.BuildConfigManagerAgentWithCompositionForHost(ctx, cfg, state, agents.AgentHostCapabilities{Interactive: true}, resourceSkills...)
	if err != nil {
		return nil, prompts.SystemPromptComposition{}, fmt.Errorf("构建配置管理 Agent 失败: %w", err)
	}
	return agentchat.NewRunnerWithOptions(ctx, builtAgent, agentrun.Options{AgentKind: agentrun.AgentKindConfigManager, Workspace: cfg.Workspace}), composition, nil
}

func buildAutomationAgentRunner(ctx context.Context, cfg *config.Config, state *book.State, task prompts.AutomationTaskInstruction) (*agents.Runner, error) {
	runner, _, err := buildAutomationAgentRunnerWithComposition(ctx, cfg, state, task)
	return runner, err
}

func buildAutomationAgentRunnerWithComposition(ctx context.Context, cfg *config.Config, state *book.State, task prompts.AutomationTaskInstruction) (*agents.Runner, prompts.SystemPromptComposition, error) {
	builtAgent, composition, err := agents.BuildAutomationAgentWithComposition(ctx, cfg, state, task)
	if err != nil {
		return nil, prompts.SystemPromptComposition{}, fmt.Errorf("构建自动化 Agent 失败: %w", err)
	}
	return agentchat.NewRunnerWithOptions(ctx, builtAgent, agentrun.Options{
		AgentKind: agentrun.AgentKindAutomation,
		ProjectID: cfg.ProjectID,
		StateRoot: cfg.ProjectStateDir,
		Workspace: cfg.Workspace,
	}), composition, nil
}

func buildImageAgentRunner(ctx context.Context, cfg *config.Config, state *book.State, systemPrompt string) (*agents.Runner, error) {
	runner, _, err := buildImageAgentRunnerWithComposition(ctx, cfg, state, systemPrompt)
	return runner, err
}

func buildImageAgentRunnerWithComposition(ctx context.Context, cfg *config.Config, state *book.State, systemPrompt string) (*agents.Runner, prompts.SystemPromptComposition, error) {
	builtAgent, composition, err := agents.BuildImageAgentWithComposition(ctx, cfg, state, systemPrompt)
	if err != nil {
		return nil, prompts.SystemPromptComposition{}, fmt.Errorf("构建图像 Agent 失败: %w", err)
	}
	return agentchat.NewRunnerWithOptions(ctx, builtAgent, agentrun.Options{AgentKind: agentrun.AgentKindImage, Workspace: cfg.Workspace}), composition, nil
}
