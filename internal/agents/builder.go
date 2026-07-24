package agents

import (
	"context"
	"fmt"
	"strings"

	agent "github.com/alfredxw/denova/agent"
	"github.com/alfredxw/denova/agent/model/openai"
	providercompat "github.com/alfredxw/denova/agent/model/openai/compat"

	"denova/config"
	novaskills "denova/internal/agents/skills"
	producttools "denova/internal/agents/tools"
	"denova/internal/book"
	"denova/internal/prompts"
)

var newNativeAgent = func(ctx context.Context, cfg agent.AgentConfig) (agent.Runnable, error) {
	return agent.NewAgent(ctx, cfg)
}

// Build 构建小说创作 Agent（native loop + 文件系统工具 + Skills）。
func Build(ctx context.Context, cfg *config.Config, state *book.State, teller IDEStoryTeller) (agent.Runnable, error) {
	built, _, err := BuildWithComposition(ctx, cfg, state, teller)
	return built, err
}

// BuildWithComposition returns the exact admitted prompt artifact consumed by
// the constructed Agent so RunOptions never needs to rebuild mutable sources.
func BuildWithComposition(ctx context.Context, cfg *config.Config, state *book.State, teller IDEStoryTeller) (agent.Runnable, SystemPromptComposition, error) {
	composition, err := ComposeInstruction(cfg, state, teller)
	if err != nil {
		return nil, SystemPromptComposition{}, err
	}
	built, err := buildAgent(ctx, cfg, agentBuildSpec{
		Kind:              config.AgentKindIDE,
		Name:              "DenovaAgent",
		Description:       "AI 小说创作助手",
		Composition:       composition,
		EnableSkills:      true,
		ExtraToolsFactory: newToolCatalog(cfg).IDE(),
	})
	return built, composition, err
}

func BuildInteractiveStory(ctx context.Context, cfg *config.Config, state *book.State, teller prompts.InteractiveStorySystemInstructionInput, toolContexts ...InteractiveStoryToolContext) (agent.Runnable, error) {
	built, _, err := BuildInteractiveStoryWithComposition(ctx, cfg, state, teller, toolContexts...)
	return built, err
}

func BuildInteractiveStoryWithComposition(ctx context.Context, cfg *config.Config, state *book.State, teller prompts.InteractiveStorySystemInstructionInput, toolContexts ...InteractiveStoryToolContext) (agent.Runnable, SystemPromptComposition, error) {
	handlers := []agent.Middleware{newInteractiveStoryToolMiddleware()}
	var outputGuard func(context.Context, *agent.RetryContext) *agent.RetryDecision
	if len(toolContexts) > 0 && toolContexts[0].TurnResultReady != nil {
		completionTokens, _ := EstimateContextProjectionReserves(cfg, config.AgentKindInteractiveStory, teller.ReplyTargetChars)
		handlers = append(handlers, newInteractiveTurnProtocolMiddleware(toolContexts[0].TurnResultReady, completionTokens))
		outputGuard = newInteractiveCompletionGuard(toolContexts[0].TurnResultReady)
	}
	composition, err := ComposeInteractiveStoryInstruction(cfg, state, teller)
	if err != nil {
		return nil, SystemPromptComposition{}, err
	}
	built, err := buildAgent(ctx, cfg, agentBuildSpec{
		Kind:              config.AgentKindInteractiveStory,
		Name:              "DenovaInteractiveStoryAgent",
		Description:       "AI 互动故事叙事助手",
		Composition:       composition,
		EnableSkills:      true,
		DisableWriteTodos: true,
		ExtraMiddlewares:  handlers,
		ExtraToolsFactory: newToolCatalog(cfg).InteractiveStory(projectInteractiveToolContext(toolContexts...)),
		ModelOutputGuard:  outputGuard,
	})
	return built, composition, err
}

func BuildInteractiveDirector(ctx context.Context, cfg *config.Config, state *book.State, toolContexts ...InteractiveStoryToolContext) (agent.Runnable, error) {
	built, _, err := BuildInteractiveDirectorWithComposition(ctx, cfg, state, toolContexts...)
	return built, err
}

func BuildInteractiveDirectorWithComposition(ctx context.Context, cfg *config.Config, state *book.State, toolContexts ...InteractiveStoryToolContext) (agent.Runnable, SystemPromptComposition, error) {
	composition, err := ComposeInteractiveDirectorInstruction(cfg, state)
	if err != nil {
		return nil, SystemPromptComposition{}, err
	}
	built, err := buildAgent(ctx, cfg, agentBuildSpec{
		Kind:              config.AgentKindInteractiveDirector,
		Name:              "DenovaInteractiveDirectorAgent",
		Description:       "AI 互动故事后台导演",
		Composition:       composition,
		EnableSkills:      false,
		DisableWriteTodos: true,
		ExtraMiddlewares:  []agent.Middleware{newInteractiveDirectorPlanFileMiddleware()},
		ExtraToolsFactory: newToolCatalog(cfg).InteractiveDirector(projectInteractiveToolContext(toolContexts...)),
	})
	return built, composition, err
}

// BuildConfigManagerAgent 构建统一配置管理 Agent（native loop + 通用工具 + Skills + 模块资源工具）。
func BuildConfigManagerAgent(ctx context.Context, cfg *config.Config, state *book.State, resourceSkills ...ConfigManagerResourceSkill) (agent.Runnable, error) {
	built, _, err := BuildConfigManagerAgentWithComposition(ctx, cfg, state, resourceSkills...)
	return built, err
}

func BuildConfigManagerAgentWithComposition(ctx context.Context, cfg *config.Config, state *book.State, resourceSkills ...ConfigManagerResourceSkill) (agent.Runnable, SystemPromptComposition, error) {
	composition, err := ComposeConfigManagerInstruction(cfg, state, resourceSkills...)
	if err != nil {
		return nil, SystemPromptComposition{}, err
	}
	built, err := buildAgent(ctx, cfg, agentBuildSpec{
		Kind:              config.AgentKindConfigManager,
		Name:              "DenovaConfigManagerAgent",
		Description:       "AI 配置与资源管理助手",
		Composition:       composition,
		EnableSkills:      true,
		ExtraToolsFactory: newToolCatalog(cfg).ConfigManager(),
	})
	return built, composition, err
}

// BuildAutomationAgent 构建后台自动化 Agent。工具权限由调用方按任务写入策略提前收敛到 cfg.AgentTools.Automation。
func BuildAutomationAgent(ctx context.Context, cfg *config.Config, state *book.State, task AutomationTaskInstruction) (agent.Runnable, error) {
	built, _, err := BuildAutomationAgentWithComposition(ctx, cfg, state, task)
	return built, err
}

func BuildAutomationAgentWithComposition(ctx context.Context, cfg *config.Config, state *book.State, task AutomationTaskInstruction) (agent.Runnable, SystemPromptComposition, error) {
	composition, err := ComposeAutomationInstruction(cfg, state, task)
	if err != nil {
		return nil, SystemPromptComposition{}, err
	}
	built, err := buildAgent(ctx, cfg, agentBuildSpec{
		Kind:              config.AgentKindAutomation,
		Name:              "DenovaAutomationAgent",
		Description:       "AI 自动化任务助手",
		Composition:       composition,
		EnableSkills:      true,
		ExtraToolsFactory: newToolCatalog(cfg).Lore(false),
	})
	return built, composition, err
}

// BuildImageAgent 构建通用图像 Agent。调用方通过运行时上下文和 Skill 约束具体用途。
func BuildImageAgent(ctx context.Context, cfg *config.Config, state *book.State, systemPrompt string) (agent.Runnable, error) {
	built, _, err := BuildImageAgentWithComposition(ctx, cfg, state, systemPrompt)
	return built, err
}

func BuildImageAgentWithComposition(ctx context.Context, cfg *config.Config, state *book.State, systemPrompt string) (agent.Runnable, SystemPromptComposition, error) {
	composition, err := ComposeImageInstruction(cfg, state, systemPrompt)
	if err != nil {
		return nil, SystemPromptComposition{}, err
	}
	built, err := buildAgent(ctx, cfg, agentBuildSpec{
		Kind:              config.AgentKindImage,
		Name:              "DenovaImageAgent",
		Description:       "AI 图像生成助手",
		Composition:       composition,
		EnableSkills:      true,
		DisableWriteTodos: true,
		ExtraToolsFactory: newToolCatalog(cfg).Image(),
	})
	return built, composition, err
}

type agentBuildSpec struct {
	Kind              string
	Name              string
	Description       string
	Instruction       string
	Composition       SystemPromptComposition
	EnableSkills      bool
	DisableWriteTodos bool
	ExtraMiddlewares  []agent.Middleware
	ExtraTools        []agent.BaseTool
	ExtraToolsFactory func(config.ResolvedAgentToolSettings) ([]agent.BaseTool, error)
	ModelOutputGuard  func(context.Context, *agent.RetryContext) *agent.RetryDecision
}

func buildAgent(ctx context.Context, cfg *config.Config, spec agentBuildSpec) (agent.Runnable, error) {
	toolCatalog := newToolCatalog(cfg)
	composition, err := resolveAgentSystemPrompt(cfg, spec)
	if err != nil {
		return nil, err
	}
	modelCfg := chatModelConfigForAgent(cfg, spec.Kind)
	toolSettings := config.ResolveAgentTools(cfg, spec.Kind)
	cm, err := openai.New(ctx, &modelCfg)
	if err != nil {
		return nil, fmt.Errorf("创建模型失败: %w", err)
	}
	// providercompat 决定是否要为这个 provider 加包装层（修复工具调用格式、剥离内联 think 等）。
	// agent 包不感知具体 provider；新增 provider 的兼容性处理只需在 providercompat 里加。
	chatModel := providercompat.Wrap(cm, modelCfg)

	assembly, err := buildChatModelAgentAssembly(ctx, cfg, chatModelAgentAssemblySpec{
		Kind:                  spec.Kind,
		ModelCfg:              modelCfg,
		ToolSettings:          toolSettings,
		EnableSkills:          spec.EnableSkills,
		ExtraMiddlewares:      spec.ExtraMiddlewares,
		ExtraTools:            spec.ExtraTools,
		ExtraToolsFactory:     spec.ExtraToolsFactory,
		IncludeCompaction:     true,
		ContextWindowTokens:   config.ResolveAgentModel(cfg, spec.Kind).ContextWindowTokens,
		ProviderInputMaxBytes: config.ResolveAgentContext(cfg, spec.Kind).MaxProviderInputBytes,
	})
	if err != nil {
		return nil, err
	}
	configuredSubAgents, err := buildConfiguredSubAgents(ctx, cfg, spec, toolSettings)
	if err != nil {
		return nil, err
	}

	taskAgents := append([]agent.Runnable(nil), configuredSubAgents...)
	if config.GeneralSubAgentEnabled(cfg, spec.Kind) {
		general, err := newNativeAgent(ctx, agent.AgentConfig{
			Name:          producttools.GeneralSubAgentName,
			Description:   "通用子 Agent，用于研究复杂问题、搜索代码和执行独立的多步骤任务。",
			Instruction:   composition.Instruction(),
			Model:         chatModel,
			Tools:         assembly.Tools,
			Middlewares:   assembly.Middlewares,
			MaxIterations: configMaxIteration(cfg),
			Retry:         modelRetryConfig(cfg, nil),
		})
		if err != nil {
			return nil, fmt.Errorf("创建通用子 Agent 失败: %w", err)
		}
		taskAgents = append([]agent.Runnable{general}, taskAgents...)
	}

	tools := append([]agent.BaseTool(nil), assembly.Tools...)
	if !spec.DisableWriteTodos && toolSettings.Todo {
		todoTool, err := toolCatalog.WriteTodos()
		if err != nil {
			return nil, fmt.Errorf("创建 write_todos 工具失败: %w", err)
		}
		tools = append(tools, todoTool)
	}
	if taskTool, err := toolCatalog.Task(ctx, taskAgents); err != nil {
		return nil, fmt.Errorf("创建 task 工具失败: %w", err)
	} else if taskTool != nil {
		tools = append(tools, taskTool)
	}
	if err := producttools.Validate(ctx, tools); err != nil {
		return nil, err
	}

	return newNativeAgent(ctx, agent.AgentConfig{
		Name:          spec.Name,
		Description:   spec.Description,
		Instruction:   composition.Instruction(),
		Model:         chatModel,
		Tools:         tools,
		Middlewares:   assembly.Middlewares,
		MaxIterations: configMaxIteration(cfg),
		Retry:         modelRetryConfig(cfg, spec.ModelOutputGuard),
	})
}

func resolveAgentSystemPrompt(cfg *config.Config, spec agentBuildSpec) (SystemPromptComposition, error) {
	composition := spec.Composition
	if composition.Err() != nil {
		return SystemPromptComposition{}, composition.Err()
	}
	if composition.Instruction() != "" {
		if composition.agentKind != "" && composition.agentKind != strings.TrimSpace(spec.Kind) {
			return SystemPromptComposition{}, fmt.Errorf("system prompt agent kind mismatch: composition=%s spec=%s", composition.agentKind, spec.Kind)
		}
		if composition.InstructionHash() != systemPromptSHA(composition.Instruction()) {
			return SystemPromptComposition{}, fmt.Errorf("system prompt composition hash mismatch: agent=%s", spec.Kind)
		}
		return composition, nil
	}
	instruction := strings.TrimSpace(spec.Instruction)
	if instruction == "" {
		return SystemPromptComposition{}, fmt.Errorf("system prompt composition is required: agent=%s", spec.Kind)
	}
	return composeSystemPrompt(cfg, spec.Kind, spec.Kind, "", []SystemPromptFragment{{
		ID: "agent_instruction", Source: "agent build specification", Title: "Agent system instruction",
		Purpose: "provide a compatibility instruction for internal Agent construction", Content: instruction,
		Required: true, Overflow: SystemPromptOverflowReject,
	}})
}

func workspaceForPrompt(cfg *config.Config, state *book.State) string {
	if cfg != nil && strings.TrimSpace(cfg.Workspace) != "" {
		return strings.TrimSpace(cfg.Workspace)
	}
	if state != nil {
		return strings.TrimSpace(state.Workspace())
	}
	return ""
}

type chatModelAgentAssemblySpec struct {
	Kind                  string
	ToolPolicyKind        string
	ModelCfg              openai.Config
	ToolSettings          config.ResolvedAgentToolSettings
	EnableSkills          bool
	ExtraMiddlewares      []agent.Middleware
	ExtraTools            []agent.BaseTool
	ExtraToolsFactory     func(config.ResolvedAgentToolSettings) ([]agent.BaseTool, error)
	IncludeCompaction     bool
	ContextWindowTokens   int
	ProviderInputMaxBytes int
}

type chatModelAgentAssembly struct {
	Tools       []agent.BaseTool
	Middlewares []agent.Middleware
}

func buildChatModelAgentAssembly(ctx context.Context, cfg *config.Config, spec chatModelAgentAssemblySpec) (chatModelAgentAssembly, error) {
	workspace := ""
	if cfg != nil {
		workspace = cfg.Workspace
	}
	executionGate := sharedToolExecutionGate(workspace)
	toolCatalog := newToolCatalog(cfg)
	settings := spec.ToolSettings
	middlewares := append([]agent.Middleware(nil), spec.ExtraMiddlewares...)
	if spec.IncludeCompaction {
		middlewares = append(middlewares, &contextCompactionMiddleware{
			BaseMiddleware: &agent.BaseMiddleware{},
			agentKind:      spec.Kind,
		})
	}
	middlewares = append(middlewares,
		&toolOrchestratorMiddleware{
			agentKind:           spec.Kind,
			policyKind:          firstNonEmpty(spec.ToolPolicyKind, spec.Kind),
			toolSettings:        spec.ToolSettings,
			enforceToolSettings: true,
			toolResultMaxBytes:  configToolResultMaxBytes(cfg),
			executionGate:       executionGate,
		},
		&modelInputLoggingMiddleware{
			BaseMiddleware:        &agent.BaseMiddleware{},
			agentKind:             spec.Kind,
			config:                spec.ModelCfg,
			contextWindowTokens:   spec.ContextWindowTokens,
			providerInputMaxBytes: spec.ProviderInputMaxBytes,
		},
	)
	tools := append([]agent.BaseTool(nil), spec.ExtraTools...)
	if settings.FileRead || settings.FileWrite || settings.ShellExecute {
		filesystemTools, err := toolCatalog.Filesystem(settings)
		if err != nil {
			return chatModelAgentAssembly{}, err
		}
		tools = append(tools, filesystemTools...)
	}
	if settings.Skills {
		skillTools, err := buildSkillTools(ctx, cfg, spec.Kind, spec.EnableSkills, settings)
		if err != nil {
			return chatModelAgentAssembly{}, err
		}
		tools = append(tools, skillTools...)
	}
	if spec.ExtraToolsFactory != nil {
		extraTools, err := spec.ExtraToolsFactory(settings)
		if err != nil {
			return chatModelAgentAssembly{}, err
		}
		tools = append(tools, extraTools...)
	}
	if toolCatalog.WebAccessEnabled(firstNonEmpty(spec.ToolPolicyKind, spec.Kind), settings) {
		webTools, err := toolCatalog.WebAccess()
		if err != nil {
			return chatModelAgentAssembly{}, err
		}
		tools = append(tools, webTools...)
	}
	if err := producttools.Validate(ctx, tools); err != nil {
		return chatModelAgentAssembly{}, err
	}
	// Keep this last: it validates the final concrete tool set immediately
	// before every model run.
	middlewares = append(middlewares, producttools.NewDescriptorGuardMiddleware())
	return chatModelAgentAssembly{Tools: tools, Middlewares: middlewares}, nil
}

func buildSkillTools(ctx context.Context, cfg *config.Config, agentKind string, enabled bool, settings config.ResolvedAgentToolSettings) ([]agent.BaseTool, error) {
	if !enabled || !settings.Skills || cfg == nil {
		return nil, nil
	}
	skillBackend := novaskills.NewAgentBackend(
		novaskills.NewDirectories(cfg.SkillsDir, cfg.DataDir(), cfg.Workspace),
		agentKind,
		config.ResolveAgentSkillOverrides(cfg, agentKind),
	)
	skillTool, err := newToolCatalog(cfg).Skill(ctx, skillBackend, config.ResolveAgentContext(cfg, agentKind).MaxFragmentBytes)
	if err != nil {
		return nil, fmt.Errorf("创建 Skill 工具失败 agent=%s: %w", agentKind, err)
	}
	if skillTool == nil {
		return nil, nil
	}
	return []agent.BaseTool{skillTool}, nil
}

func buildConfiguredSubAgents(ctx context.Context, cfg *config.Config, parent agentBuildSpec, parentTools config.ResolvedAgentToolSettings) ([]agent.Runnable, error) {
	if cfg == nil || !config.IsSubAgentParentKind(parent.Kind) {
		return nil, nil
	}
	subConfigs := config.SanitizeSubAgents(cfg.SubAgents)
	if len(subConfigs) == 0 {
		return nil, nil
	}
	subAgents := make([]agent.Runnable, 0, len(subConfigs))
	for _, sub := range subConfigs {
		if !config.SubAgentAllowedForParent(sub, parent.Kind) {
			continue
		}
		subAgent, err := buildConfiguredSubAgent(ctx, cfg, parent, parentTools, sub)
		if err != nil {
			return nil, err
		}
		subAgents = append(subAgents, subAgent)
	}
	return subAgents, nil
}

func buildConfiguredSubAgent(ctx context.Context, cfg *config.Config, parent agentBuildSpec, parentTools config.ResolvedAgentToolSettings, sub config.SubAgentConfig) (agent.Runnable, error) {
	composition, err := composeSubAgentInstruction(cfg, parent, sub)
	if err != nil {
		return nil, fmt.Errorf("assemble sub Agent system prompt id=%s: %w", sub.ID, err)
	}
	resolvedModel := config.ResolveSubAgentModel(cfg, parent.Kind, sub)
	modelCfg := chatModelConfigFromResolved(resolvedModel)
	cm, err := openai.New(ctx, &modelCfg)
	if err != nil {
		return nil, fmt.Errorf("创建子 Agent 模型失败 id=%s: %w", sub.ID, err)
	}
	subChatModel := providercompat.Wrap(cm, modelCfg)
	toolSettings := config.ResolveSubAgentTools(parentTools, sub.Tools)
	assembly, err := buildChatModelAgentAssembly(ctx, cfg, chatModelAgentAssemblySpec{
		Kind:                  sub.ID,
		ToolPolicyKind:        parent.Kind,
		ModelCfg:              modelCfg,
		ToolSettings:          toolSettings,
		EnableSkills:          parent.EnableSkills,
		ExtraToolsFactory:     parent.ExtraToolsFactory,
		IncludeCompaction:     false,
		ContextWindowTokens:   resolvedModel.ContextWindowTokens,
		ProviderInputMaxBytes: config.ResolveAgentContext(cfg, parent.Kind).MaxProviderInputBytes,
	})
	if err != nil {
		return nil, err
	}
	return newNativeAgent(ctx, agent.AgentConfig{
		Name:          sub.ID,
		Description:   sub.Description,
		Instruction:   composition.Instruction(),
		Model:         subChatModel,
		MaxIterations: configMaxIteration(cfg),
		Middlewares:   assembly.Middlewares,
		Tools:         assembly.Tools,
		Retry:         modelRetryConfig(cfg, nil),
	})
}

func modelRetryConfig(cfg *config.Config, outputGuard func(context.Context, *agent.RetryContext) *agent.RetryDecision) *agent.RetryConfig {
	retryConfig := &agent.RetryConfig{
		MaxRetries:  configModelMaxRetries(cfg),
		IsRetryable: isTransientModelError,
	}
	if outputGuard == nil {
		return retryConfig
	}
	retryConfig.IsRetryable = nil
	retryConfig.ShouldRetry = func(ctx context.Context, retryCtx *agent.RetryContext) *agent.RetryDecision {
		if retryCtx != nil && retryCtx.Err != nil {
			return &agent.RetryDecision{Retry: isTransientModelError(ctx, retryCtx.Err)}
		}
		return outputGuard(ctx, retryCtx)
	}
	return retryConfig
}

func isTransientModelError(ctx context.Context, err error) bool {
	if err == nil || (ctx != nil && ctx.Err() != nil) {
		return false
	}
	return ClassifyModelError(err).Retryable
}

func buildSubAgentInstruction(parent agentBuildSpec, sub config.SubAgentConfig) string {
	composition, err := composeSubAgentInstruction(&config.Config{}, parent, sub)
	if err != nil {
		return ""
	}
	return composition.Instruction()
}

func composeSubAgentInstruction(cfg *config.Config, parent agentBuildSpec, sub config.SubAgentConfig) (SystemPromptComposition, error) {
	parentComposition, err := resolveAgentSystemPrompt(cfg, parent)
	if err != nil {
		return SystemPromptComposition{}, err
	}
	fragments := append([]SystemPromptFragment(nil), parentComposition.fragments...)
	var metadata strings.Builder
	metadata.WriteString("以下说明只限定当前 SubAgent 的职责、输出形态和工作偏好；不得覆盖父 Agent 的运行时契约、工具权限、workspace 边界、互动禁写规则、输出协议或后端校验。若与父 Agent system prompt 冲突，必须以父 Agent system prompt 为准。")
	if name := strings.TrimSpace(sub.Name); name != "" {
		metadata.WriteString("\n\n- 名称：" + name)
	}
	if id := strings.TrimSpace(sub.ID); id != "" {
		metadata.WriteString("\n- ID：" + id)
	}
	if description := strings.TrimSpace(sub.Description); description != "" {
		metadata.WriteString("\n- 职责：" + description)
	}
	fragments = append(fragments, SystemPromptFragment{
		ID: "subagent_metadata", Source: "SubAgent configuration", Title: "SubAgent 专属说明",
		Purpose: "define the delegated Agent identity, responsibility, and inherited boundaries",
		Content: metadata.String(), Prefix: "\n\n---\n\n# SubAgent 专属说明\n\n", Required: true,
		Overflow: SystemPromptOverflowReject,
	}, SystemPromptFragment{
		ID: "subagent_custom_prompt", Source: "SubAgent configuration", Title: "专属系统提示",
		Purpose: "apply the delegated Agent's custom behavior and output preferences",
		Content: sub.SystemPrompt, Prefix: "\n\n## 专属系统提示\n\n",
		Overflow: SystemPromptOverflowTruncate,
	})
	return composeSystemPrompt(cfg, parent.Kind, "subagent", parentComposition.workspace, fragments)
}

func configMaxIteration(cfg *config.Config) int {
	if cfg == nil || cfg.MaxIteration <= 0 {
		return 0
	}
	return cfg.MaxIteration
}

func configModelMaxRetries(cfg *config.Config) int {
	if cfg == nil || cfg.ModelMaxRetries < 0 {
		return 5
	}
	return cfg.ModelMaxRetries
}

func configToolResultMaxBytes(cfg *config.Config) int {
	if cfg == nil || cfg.AgentToolResultLimitKB <= 0 {
		return defaultToolResultMaxBytes
	}
	return cfg.AgentToolResultLimitKB * 1024
}
