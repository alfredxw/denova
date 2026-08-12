package agents

import (
	"context"
	"crypto/sha256"
	agentchat "denova/internal/agents/chat"
	agentinteractive "denova/internal/agents/interactive"
	agenttoolruntime "denova/internal/agents/toolruntime"
	"encoding/hex"
	"encoding/json"
	"fmt"

	agent "github.com/alfredxw/denova/agent"
	"github.com/alfredxw/denova/agent/providers"

	"denova/config"
	agentcompaction "denova/internal/agents/context/compaction"
	"denova/internal/agents/harnessstate"
	"denova/internal/agents/modelio"
	"denova/internal/agents/prompts"
	agentrun "denova/internal/agents/run"
	novaskills "denova/internal/agents/skills"
	"denova/internal/agents/toolresult"
	producttools "denova/internal/agents/tools"
	"denova/internal/book"
)

var newNativeAgent = func(ctx context.Context, cfg agent.LoopConfig) (agent.Runnable, error) {
	return agent.NewLoop(ctx, cfg)
}

// ToolDefinition keeps application packages on Denova's Agent boundary rather
// than importing the provider runtime directly.
type ToolDefinition = agent.ToolDefinition
type Definition = agent.Definition

// Build 构建小说创作 Agent（native loop + 文件系统工具 + Skills）。
func Build(ctx context.Context, cfg *config.Config, state *book.State, teller prompts.IDEStoryTeller) (agent.Runnable, error) {
	built, _, err := BuildWithComposition(ctx, cfg, state, teller)
	return built, err
}

// BuildWithComposition returns the exact admitted prompt artifact consumed by
// the constructed Agent so agentrun.Options never needs to rebuild mutable sources.
func BuildWithComposition(ctx context.Context, cfg *config.Config, state *book.State, teller prompts.IDEStoryTeller) (agent.Runnable, prompts.SystemPromptComposition, error) {
	return buildWithCompositionForHost(ctx, cfg, state, teller, AgentHostCapabilities{})
}

// AgentHostCapabilities are runtime surfaces supplied by the caller. Tool
// settings authorize a capability; they cannot manufacture an interactive UI.
type AgentHostCapabilities struct {
	Interactive bool
	// RootTools are host-owned session tools. They are intentionally excluded
	// from every sub-Agent assembly.
	RootTools []agent.ToolDefinition
	// ReadAdapters extend the single read tool with application-owned URI
	// resources without exposing extra model-visible state-management tools.
	ReadAdapters []producttools.ReadAdapterBinding
	// CompletionGuard lets an interactive host reject a premature final answer
	// and return actionable feedback without adding a model-visible tool.
	CompletionGuard func(context.Context, *agent.RetryContext) *agent.RetryDecision
}

// BuildWithCompositionForHost builds the top-level IDE Agent for a concrete
// host. Headless callers should keep using BuildWithComposition.
func BuildWithCompositionForHost(ctx context.Context, cfg *config.Config, state *book.State, teller prompts.IDEStoryTeller, host AgentHostCapabilities) (agent.Runnable, prompts.SystemPromptComposition, error) {
	return buildWithCompositionForHost(ctx, cfg, state, teller, host)
}

// BuildDefinitionWithCompositionForHost returns the complete public Agent
// composition for a writing/IDE Project Session.
func BuildDefinitionWithCompositionForHost(ctx context.Context, cfg *config.Config, state *book.State, teller prompts.IDEStoryTeller, host AgentHostCapabilities) (agent.Definition, prompts.SystemPromptComposition, error) {
	composition, err := prompts.ComposeInstruction(cfg, state, teller)
	if err != nil {
		return agent.Definition{}, prompts.SystemPromptComposition{}, err
	}
	definition, err := buildAgentDefinition(ctx, cfg, agentBuildSpec{
		Kind:              config.AgentKindIDE,
		Name:              "DenovaAgent",
		Description:       "AI 小说创作助手",
		Composition:       composition,
		EnableSkills:      true,
		InteractiveHost:   host.Interactive,
		ExtraTools:        host.RootTools,
		ExtraToolsFactory: agenttoolruntime.NewCatalog(cfg).IDE(),
	})
	return definition, composition, err
}

// BuildGeneralAgentWithCompositionForHost builds a general-purpose Agent for
// a user-added Project. It intentionally assembles only generic workspace,
// web, browser, Skill, planning, ask, delegation and context tools.
func BuildGeneralAgentWithCompositionForHost(ctx context.Context, cfg *config.Config, host AgentHostCapabilities) (agent.Runnable, prompts.SystemPromptComposition, error) {
	composition, err := prompts.ComposeGeneralInstruction(cfg)
	if err != nil {
		return nil, prompts.SystemPromptComposition{}, err
	}
	built, err := buildAgent(ctx, cfg, agentBuildSpec{
		Kind:            config.AgentKindGeneral,
		Name:            "DenovaGeneralAgent",
		Description:     "General-purpose project Agent",
		Composition:     composition,
		EnableSkills:    true,
		InteractiveHost: host.Interactive,
		ExtraTools:      host.RootTools,
		ReadAdapters:    host.ReadAdapters,
	})
	return built, composition, err
}

// BuildGeneralDefinitionWithCompositionForHost returns the complete public
// Agent composition for a general Project Session.
func BuildGeneralDefinitionWithCompositionForHost(ctx context.Context, cfg *config.Config, host AgentHostCapabilities) (agent.Definition, prompts.SystemPromptComposition, error) {
	composition, err := prompts.ComposeGeneralInstruction(cfg)
	if err != nil {
		return agent.Definition{}, prompts.SystemPromptComposition{}, err
	}
	definition, err := buildAgentDefinition(ctx, cfg, agentBuildSpec{
		Kind:            config.AgentKindGeneral,
		Name:            "DenovaGeneralAgent",
		Description:     "General-purpose project Agent",
		Composition:     composition,
		EnableSkills:    true,
		InteractiveHost: host.Interactive,
		ExtraTools:      host.RootTools,
		ReadAdapters:    host.ReadAdapters,
	})
	return definition, composition, err
}

// BuildHarnessOptimizerDefinitionWithCompositionForHost assembles the
// user-level continual-learning Agent over an isolated State draft. Its common
// capability policy is resolved from the same preset as General Agent.
func BuildHarnessOptimizerDefinitionWithCompositionForHost(ctx context.Context, cfg *config.Config, host AgentHostCapabilities) (agent.Definition, prompts.SystemPromptComposition, error) {
	composition, err := prompts.ComposeHarnessOptimizerInstruction(cfg)
	if err != nil {
		return agent.Definition{}, prompts.SystemPromptComposition{}, err
	}
	definition, err := buildAgentDefinition(ctx, cfg, agentBuildSpec{
		Kind:             config.AgentKindHarnessOptimizer,
		Name:             "DenovaHarnessOptimizer",
		Description:      "Optimizes user-level Harness State from trajectory evidence",
		Composition:      composition,
		EnableSkills:     true,
		InteractiveHost:  host.Interactive,
		ExtraTools:       host.RootTools,
		ReadAdapters:     host.ReadAdapters,
		ModelOutputGuard: host.CompletionGuard,
	})
	return definition, composition, err
}

func buildWithCompositionForHost(ctx context.Context, cfg *config.Config, state *book.State, teller prompts.IDEStoryTeller, host AgentHostCapabilities) (agent.Runnable, prompts.SystemPromptComposition, error) {
	composition, err := prompts.ComposeInstruction(cfg, state, teller)
	if err != nil {
		return nil, prompts.SystemPromptComposition{}, err
	}
	built, err := buildAgent(ctx, cfg, agentBuildSpec{
		Kind:              config.AgentKindIDE,
		Name:              "DenovaAgent",
		Description:       "AI 小说创作助手",
		Composition:       composition,
		EnableSkills:      true,
		InteractiveHost:   host.Interactive,
		ExtraTools:        host.RootTools,
		ExtraToolsFactory: agenttoolruntime.NewCatalog(cfg).IDE(),
	})
	return built, composition, err
}

func BuildInteractiveStory(ctx context.Context, cfg *config.Config, state *book.State, teller prompts.InteractiveStorySystemInstructionInput, toolContexts ...agentinteractive.InteractiveStoryToolContext) (agent.Runnable, error) {
	built, _, err := BuildInteractiveStoryWithComposition(ctx, cfg, state, teller, toolContexts...)
	return built, err
}

func BuildInteractiveStoryWithComposition(ctx context.Context, cfg *config.Config, state *book.State, teller prompts.InteractiveStorySystemInstructionInput, toolContexts ...agentinteractive.InteractiveStoryToolContext) (agent.Runnable, prompts.SystemPromptComposition, error) {
	handlers := []agent.Middleware{agenttoolruntime.NewInteractiveStoryMiddleware()}
	var outputGuard func(context.Context, *agent.RetryContext) *agent.RetryDecision
	if len(toolContexts) > 0 && toolContexts[0].TurnResultReady != nil {
		handlers = append(handlers, agentinteractive.NewTurnProtocolMiddleware(toolContexts[0].TurnResultReady))
		outputGuard = agentinteractive.NewCompletionGuard(toolContexts[0].TurnResultReady)
	}
	composition, err := prompts.ComposeInteractiveStoryInstruction(cfg, state, teller)
	if err != nil {
		return nil, prompts.SystemPromptComposition{}, err
	}
	built, err := buildAgent(ctx, cfg, agentBuildSpec{
		Kind:              config.AgentKindInteractiveStory,
		Name:              "DenovaInteractiveStoryAgent",
		Description:       "AI 互动故事叙事助手",
		Composition:       composition,
		EnableSkills:      true,
		DisableWriteTodos: true,
		ExtraMiddlewares:  handlers,
		ExtraToolsFactory: agenttoolruntime.NewCatalog(cfg).InteractiveStory(agenttoolruntime.ProjectInteractiveContext(toolContexts...)),
		ModelOutputGuard:  outputGuard,
	})
	return built, composition, err
}

func BuildInteractiveStoryDefinitionWithComposition(ctx context.Context, cfg *config.Config, state *book.State, teller prompts.InteractiveStorySystemInstructionInput, toolContexts ...agentinteractive.InteractiveStoryToolContext) (agent.Definition, prompts.SystemPromptComposition, error) {
	handlers := []agent.Middleware{agenttoolruntime.NewInteractiveStoryMiddleware()}
	var outputGuard func(context.Context, *agent.RetryContext) *agent.RetryDecision
	if len(toolContexts) > 0 && toolContexts[0].TurnResultReady != nil {
		handlers = append(handlers, agentinteractive.NewTurnProtocolMiddleware(toolContexts[0].TurnResultReady))
		outputGuard = agentinteractive.NewCompletionGuard(toolContexts[0].TurnResultReady)
	}
	composition, err := prompts.ComposeInteractiveStoryInstruction(cfg, state, teller)
	if err != nil {
		return agent.Definition{}, prompts.SystemPromptComposition{}, err
	}
	definition, err := buildAgentDefinition(ctx, cfg, agentBuildSpec{
		Kind:              config.AgentKindInteractiveStory,
		Name:              "DenovaInteractiveStoryAgent",
		Description:       "AI 互动故事叙事助手",
		Composition:       composition,
		EnableSkills:      true,
		DisableWriteTodos: true,
		ExtraMiddlewares:  handlers,
		ExtraToolsFactory: agenttoolruntime.NewCatalog(cfg).InteractiveStory(agenttoolruntime.ProjectInteractiveContext(toolContexts...)),
		ModelOutputGuard:  outputGuard,
	})
	return definition, composition, err
}

func BuildInteractiveDirector(ctx context.Context, cfg *config.Config, state *book.State, toolContexts ...agentinteractive.InteractiveStoryToolContext) (agent.Runnable, error) {
	built, _, err := BuildInteractiveDirectorWithComposition(ctx, cfg, state, toolContexts...)
	return built, err
}

func BuildInteractiveDirectorWithComposition(ctx context.Context, cfg *config.Config, state *book.State, toolContexts ...agentinteractive.InteractiveStoryToolContext) (agent.Runnable, prompts.SystemPromptComposition, error) {
	composition, err := prompts.ComposeInteractiveDirectorInstruction(cfg, state)
	if err != nil {
		return nil, prompts.SystemPromptComposition{}, err
	}
	toolContext := agenttoolruntime.ProjectInteractiveContext(toolContexts...)
	built, err := buildAgent(ctx, cfg, agentBuildSpec{
		Kind:                config.AgentKindInteractiveDirector,
		Name:                "DenovaInteractiveDirectorAgent",
		Description:         "AI 互动故事后台导演",
		Composition:         composition,
		EnableSkills:        false,
		DisableWriteTodos:   true,
		ExtraMiddlewares:    []agent.Middleware{agenttoolruntime.NewInteractiveDirectorPlanFileMiddleware()},
		ReadAdaptersFactory: agenttoolruntime.NewCatalog(cfg).InteractiveDirectorRead(toolContext),
		ExtraToolsFactory:   agenttoolruntime.NewCatalog(cfg).InteractiveDirector(toolContext),
	})
	return built, composition, err
}

func BuildInteractiveDirectorDefinitionWithComposition(ctx context.Context, cfg *config.Config, state *book.State, toolContexts ...agentinteractive.InteractiveStoryToolContext) (agent.Definition, prompts.SystemPromptComposition, error) {
	composition, err := prompts.ComposeInteractiveDirectorInstruction(cfg, state)
	if err != nil {
		return agent.Definition{}, prompts.SystemPromptComposition{}, err
	}
	toolContext := agenttoolruntime.ProjectInteractiveContext(toolContexts...)
	definition, err := buildAgentDefinition(ctx, cfg, agentBuildSpec{
		Kind:                config.AgentKindInteractiveDirector,
		Name:                "DenovaInteractiveDirectorAgent",
		Description:         "AI 互动故事后台导演",
		Composition:         composition,
		EnableSkills:        false,
		DisableWriteTodos:   true,
		ExtraMiddlewares:    []agent.Middleware{agenttoolruntime.NewInteractiveDirectorPlanFileMiddleware()},
		ReadAdaptersFactory: agenttoolruntime.NewCatalog(cfg).InteractiveDirectorRead(toolContext),
		ExtraToolsFactory:   agenttoolruntime.NewCatalog(cfg).InteractiveDirector(toolContext),
	})
	return definition, composition, err
}

// BuildConfigManagerAgent 构建统一配置管理 Agent（native loop + 通用工具 + Skills + 模块资源工具）。
func BuildConfigManagerAgent(ctx context.Context, cfg *config.Config, state *book.State, resourceSkills ...prompts.ConfigManagerResourceSkill) (agent.Runnable, error) {
	built, _, err := BuildConfigManagerAgentWithComposition(ctx, cfg, state, resourceSkills...)
	return built, err
}

func BuildConfigManagerAgentWithComposition(ctx context.Context, cfg *config.Config, state *book.State, resourceSkills ...prompts.ConfigManagerResourceSkill) (agent.Runnable, prompts.SystemPromptComposition, error) {
	return buildConfigManagerAgentWithCompositionForHost(ctx, cfg, state, AgentHostCapabilities{}, resourceSkills...)
}

func BuildConfigManagerAgentWithCompositionForHost(ctx context.Context, cfg *config.Config, state *book.State, host AgentHostCapabilities, resourceSkills ...prompts.ConfigManagerResourceSkill) (agent.Runnable, prompts.SystemPromptComposition, error) {
	return buildConfigManagerAgentWithCompositionForHost(ctx, cfg, state, host, resourceSkills...)
}

func BuildConfigManagerDefinitionWithCompositionForHost(ctx context.Context, cfg *config.Config, state *book.State, host AgentHostCapabilities, resourceSkills ...prompts.ConfigManagerResourceSkill) (agent.Definition, prompts.SystemPromptComposition, error) {
	composition, err := prompts.ComposeConfigManagerInstruction(cfg, state, resourceSkills...)
	if err != nil {
		return agent.Definition{}, prompts.SystemPromptComposition{}, err
	}
	definition, err := buildAgentDefinition(ctx, cfg, agentBuildSpec{
		Kind:              config.AgentKindConfigManager,
		Name:              "DenovaConfigManagerAgent",
		Description:       "AI 配置与资源管理助手",
		Composition:       composition,
		EnableSkills:      true,
		InteractiveHost:   host.Interactive,
		ExtraToolsFactory: agenttoolruntime.NewCatalog(cfg).ConfigManager(),
	})
	return definition, composition, err
}

func buildConfigManagerAgentWithCompositionForHost(ctx context.Context, cfg *config.Config, state *book.State, host AgentHostCapabilities, resourceSkills ...prompts.ConfigManagerResourceSkill) (agent.Runnable, prompts.SystemPromptComposition, error) {
	composition, err := prompts.ComposeConfigManagerInstruction(cfg, state, resourceSkills...)
	if err != nil {
		return nil, prompts.SystemPromptComposition{}, err
	}
	built, err := buildAgent(ctx, cfg, agentBuildSpec{
		Kind:              config.AgentKindConfigManager,
		Name:              "DenovaConfigManagerAgent",
		Description:       "AI 配置与资源管理助手",
		Composition:       composition,
		EnableSkills:      true,
		InteractiveHost:   host.Interactive,
		ExtraToolsFactory: agenttoolruntime.NewCatalog(cfg).ConfigManager(),
	})
	return built, composition, err
}

// BuildImageAgent 构建通用图像 Agent。调用方通过运行时上下文和 Skill 约束具体用途。
func BuildImageAgent(ctx context.Context, cfg *config.Config, state *book.State, systemPrompt string) (agent.Runnable, error) {
	built, _, err := BuildImageAgentWithComposition(ctx, cfg, state, systemPrompt)
	return built, err
}

func BuildImageAgentWithComposition(ctx context.Context, cfg *config.Config, state *book.State, systemPrompt string) (agent.Runnable, prompts.SystemPromptComposition, error) {
	composition, err := prompts.ComposeImageInstruction(cfg, state, systemPrompt)
	if err != nil {
		return nil, prompts.SystemPromptComposition{}, err
	}
	built, err := buildAgent(ctx, cfg, agentBuildSpec{
		Kind:              config.AgentKindImage,
		Name:              "DenovaImageAgent",
		Description:       "AI 图像生成助手",
		Composition:       composition,
		EnableSkills:      true,
		DisableWriteTodos: true,
		ExtraToolsFactory: agenttoolruntime.NewCatalog(cfg).Image(),
	})
	return built, composition, err
}

func BuildImageDefinitionWithComposition(ctx context.Context, cfg *config.Config, state *book.State, systemPrompt string) (agent.Definition, prompts.SystemPromptComposition, error) {
	composition, err := prompts.ComposeImageInstruction(cfg, state, systemPrompt)
	if err != nil {
		return agent.Definition{}, prompts.SystemPromptComposition{}, err
	}
	definition, err := buildAgentDefinition(ctx, cfg, agentBuildSpec{
		Kind:              config.AgentKindImage,
		Name:              "DenovaImageAgent",
		Description:       "AI 图像生成助手",
		Composition:       composition,
		EnableSkills:      true,
		DisableWriteTodos: true,
		ExtraToolsFactory: agenttoolruntime.NewCatalog(cfg).Image(),
	})
	return definition, composition, err
}

type agentBuildSpec struct {
	Kind                string
	Name                string
	Description         string
	Composition         prompts.SystemPromptComposition
	EnableSkills        bool
	InteractiveHost     bool
	DisableWriteTodos   bool
	ExtraMiddlewares    []agent.Middleware
	ExtraTools          []agent.ToolDefinition
	ReadAdapters        []producttools.ReadAdapterBinding
	ReadAdaptersFactory producttools.ReadAdapterFactory
	ExtraToolsFactory   func(config.ResolvedAgentToolSettings) ([]agent.ToolDefinition, error)
	ModelOutputGuard    func(context.Context, *agent.RetryContext) *agent.RetryDecision
}

func buildAgent(ctx context.Context, cfg *config.Config, spec agentBuildSpec) (agent.Runnable, error) {
	definition, err := buildAgentDefinition(ctx, cfg, spec)
	if err != nil {
		return nil, err
	}
	return RunnableFromDefinition(ctx, definition)
}

// RunnableFromDefinition adapts an exact Definition for bounded child model
// tasks that deliberately have no durable user Session. Conversational roots
// use Agent Sessions; this helper never creates a second root execution path.
func RunnableFromDefinition(ctx context.Context, definition agent.Definition) (agent.Runnable, error) {
	if definition.Tools == nil {
		definition.Tools = agent.StaticTools()
	}
	tools, err := definition.Tools.PrepareTools(ctx, agent.ToolRequest{})
	if err != nil {
		return nil, err
	}
	middlewares := make([]agent.Middleware, len(definition.Middlewares))
	for index, middleware := range definition.Middlewares {
		middlewares[index] = agent.MiddlewareImplementation(middleware)
	}
	return newNativeAgent(ctx, agent.LoopConfig{
		Name:            definition.Name,
		Description:     definition.Description,
		Instruction:     definition.Instructions,
		Model:           definition.Model,
		Tools:           tools,
		Middlewares:     middlewares,
		MaxIterations:   definition.Execution.MaxIterations,
		ToolParallelism: definition.Execution.ToolParallelism,
		Retry:           definition.Execution.Retry,
	})
}

// buildAgentDefinition is Denova's composition root. Durable Agent Sources and
// bounded child tasks consume this exact Definition so prompts, tools,
// middleware, and execution policy cannot drift between execution scopes.
func buildAgentDefinition(ctx context.Context, cfg *config.Config, spec agentBuildSpec) (agent.Definition, error) {
	toolCatalog := agenttoolruntime.NewCatalogWithContext(ctx, cfg)
	composition, err := resolveAgentSystemPrompt(cfg, spec)
	if err != nil {
		return agent.Definition{}, err
	}
	harness, err := harnessstate.Load(ctx, cfg)
	if err != nil {
		return agent.Definition{}, fmt.Errorf("load Harness State for Agent %s: %w", spec.Kind, err)
	}
	childComposition, err := prompts.AppendUserStatePrompt(cfg, composition, harness.Prompt(spec.Kind))
	if err != nil {
		return agent.Definition{}, fmt.Errorf("compose Harness State prompt for child Agents of %s: %w", spec.Kind, err)
	}
	childSpec := spec
	childSpec.Composition = childComposition
	modelCfg, err := modelio.ConfigForAgent(cfg, spec.Kind)
	if err != nil {
		return agent.Definition{}, fmt.Errorf("resolve model configuration: %w", err)
	}
	toolSettings := config.ResolveAgentTools(cfg, spec.Kind)
	chatModel, err := modelio.NewChatModel(ctx, modelCfg)
	if err != nil {
		return agent.Definition{}, fmt.Errorf("创建模型失败: %w", err)
	}
	modelIdentity, err := providers.ModelIdentity(modelCfg)
	if err != nil {
		return agent.Definition{}, fmt.Errorf("resolve model capability identity: %w", err)
	}

	assembly, err := buildChatModelAgentAssembly(ctx, cfg, chatModelAgentAssemblySpec{
		Kind:                spec.Kind,
		ModelCfg:            modelCfg,
		ToolSettings:        toolSettings,
		EnableSkills:        spec.EnableSkills,
		ExtraMiddlewares:    spec.ExtraMiddlewares,
		ExtraTools:          spec.ExtraTools,
		ReadAdapters:        spec.ReadAdapters,
		ReadAdaptersFactory: spec.ReadAdaptersFactory,
		ExtraToolsFactory:   spec.ExtraToolsFactory,
		// Agent.Definition.Compaction is the only root checkpoint authority.
		// The older model middleware writes product-session checkpoints and must
		// never be assembled into a public Agent Definition.
		ContextWindowTokens:   config.ResolveAgentModel(cfg, spec.Kind).ContextWindowTokens,
		ProviderInputMaxBytes: config.ResolveAgentContext(cfg, spec.Kind).MaxProviderInputBytes,
	})
	if err != nil {
		return agent.Definition{}, err
	}
	var taskAgents []agent.Runnable
	if toolSettings.Allows(config.AgentToolDelegation) {
		configuredSubAgents, err := buildConfiguredSubAgents(ctx, cfg, childSpec, toolSettings, harness.SubAgents())
		if err != nil {
			return agent.Definition{}, err
		}
		taskAgents = append(taskAgents, configuredSubAgents...)
		if config.GeneralSubAgentEnabled(cfg, spec.Kind) {
			generalAssembly, err := buildChatModelAgentAssembly(ctx, cfg, chatModelAgentAssemblySpec{
				Kind:                  producttools.GeneralSubAgentName,
				ToolPolicyKind:        spec.Kind,
				ModelCfg:              modelCfg,
				ToolSettings:          toolSettings,
				EnableSkills:          spec.EnableSkills,
				ExtraToolsFactory:     spec.ExtraToolsFactory,
				ContextWindowTokens:   config.ResolveAgentModel(cfg, spec.Kind).ContextWindowTokens,
				ProviderInputMaxBytes: config.ResolveAgentContext(cfg, spec.Kind).MaxProviderInputBytes,
			})
			if err != nil {
				return agent.Definition{}, fmt.Errorf("创建通用子 Agent 工具装配失败: %w", err)
			}
			general, err := newNativeAgent(ctx, agent.LoopConfig{
				Name:            producttools.GeneralSubAgentName,
				Description:     "通用子 Agent，用于研究复杂问题、搜索代码和执行独立的多步骤任务。",
				Instruction:     childComposition.Instruction(),
				Model:           chatModel,
				Tools:           generalAssembly.Tools,
				Middlewares:     generalAssembly.Middlewares,
				MaxIterations:   configMaxIteration(cfg),
				ToolParallelism: configToolParallelism(cfg),
				Retry:           modelRetryConfig(cfg, nil),
			})
			if err != nil {
				return agent.Definition{}, fmt.Errorf("创建通用子 Agent 失败: %w", err)
			}
			taskAgents = append([]agent.Runnable{general}, taskAgents...)
		}
	}

	tools := append([]agent.ToolDefinition(nil), assembly.Tools...)
	if !spec.DisableWriteTodos && toolSettings.Allows(config.AgentToolTodo) {
		todoTool, err := toolCatalog.Todo()
		if err != nil {
			return agent.Definition{}, fmt.Errorf("创建 todo 工具失败: %w", err)
		}
		tools = append(tools, todoTool)
	}
	if spec.InteractiveHost && toolSettings.Allows(config.AgentToolAsk) && (spec.Kind == config.AgentKindGeneral || spec.Kind == config.AgentKindIDE || spec.Kind == config.AgentKindConfigManager || spec.Kind == config.AgentKindHarnessOptimizer) {
		askTool, err := toolCatalog.Ask()
		if err != nil {
			return agent.Definition{}, fmt.Errorf("创建 ask 工具失败: %w", err)
		}
		tools = append(tools, askTool)
	}
	if toolSettings.Allows(config.AgentToolDelegation) {
		if taskTool, err := toolCatalog.Task(ctx, taskAgents); err != nil {
			return agent.Definition{}, fmt.Errorf("创建 task 工具失败: %w", err)
		} else if taskTool.Tool != nil {
			tools = append(tools, taskTool)
		}
	}
	tools = harness.ApplyToolDescriptions(tools)
	if err := producttools.Validate(ctx, tools); err != nil {
		return agent.Definition{}, err
	}

	middlewares := identifyDenovaMiddlewares(spec.Kind, cfg, assembly.Middlewares)
	retry := modelRetryConfig(cfg, spec.ModelOutputGuard)
	compaction, err := agentcompaction.NewAgentManager(cfg, spec.Kind, chatModel, modelIdentity)
	if err != nil {
		return agent.Definition{}, fmt.Errorf("create Agent Compaction manager kind=%s: %w", spec.Kind, err)
	}
	return agent.Definition{
		Key:           "denova." + spec.Kind,
		Name:          spec.Name,
		Description:   spec.Description,
		Model:         chatModel,
		ModelIdentity: modelIdentity,
		Instructions:  composition.Instruction(),
		Context:       harness.ContextSource(cfg, spec.Kind),
		Tools: agent.StaticToolsIdentified(denovaCapabilityIdentity("denova.tools", struct {
			Kind      string
			ProjectID string
			Workspace string
			Settings  config.ResolvedAgentToolSettings
		}{spec.Kind, configProjectID(cfg), configWorkspace(cfg), toolSettings}), tools...),
		Middlewares: middlewares,
		Compaction:  compaction,
		Execution: agent.ExecutionPolicy{
			Retry: retry,
			RetryIdentity: denovaCapabilityIdentity("denova.retry", struct {
				MaxRetries  int
				OutputGuard bool
			}{configModelMaxRetries(cfg), spec.ModelOutputGuard != nil}),
			MaxIterations: configMaxIteration(cfg), ToolParallelism: configToolParallelism(cfg),
		},
	}, nil
}

func identifyDenovaMiddlewares(kind string, cfg *config.Config, middlewares []agent.Middleware) []agent.Middleware {
	identified := make([]agent.Middleware, len(middlewares))
	for index, middleware := range middlewares {
		identified[index] = agent.IdentifyMiddleware(middleware, denovaCapabilityIdentity("denova.middleware", struct {
			Kind      string
			Index     int
			Type      string
			ProjectID string
			Workspace string
		}{kind, index, fmt.Sprintf("%T", middleware), configProjectID(cfg), configWorkspace(cfg)}))
	}
	return identified
}

func denovaCapabilityIdentity(kind string, configuration any) agent.CapabilityIdentity {
	encoded, _ := json.Marshal(configuration)
	digest := sha256.Sum256(encoded)
	return agent.CapabilityIdentity{Kind: kind, Version: 1, ConfigHash: hex.EncodeToString(digest[:])}
}

func configProjectID(cfg *config.Config) string {
	if cfg == nil {
		return ""
	}
	return cfg.ProjectID
}

func configWorkspace(cfg *config.Config) string {
	if cfg == nil {
		return ""
	}
	return cfg.Workspace
}

func resolveAgentSystemPrompt(_ *config.Config, spec agentBuildSpec) (prompts.SystemPromptComposition, error) {
	composition := spec.Composition
	if err := composition.ValidateForAgent(spec.Kind); err != nil {
		return prompts.SystemPromptComposition{}, err
	}
	return composition, nil
}

type chatModelAgentAssemblySpec struct {
	Kind                  string
	ToolPolicyKind        string
	ModelCfg              providers.ModelConfig
	ToolSettings          config.ResolvedAgentToolSettings
	EnableSkills          bool
	ExtraMiddlewares      []agent.Middleware
	ExtraTools            []agent.ToolDefinition
	ReadAdapters          []producttools.ReadAdapterBinding
	ReadAdaptersFactory   producttools.ReadAdapterFactory
	ExtraToolsFactory     func(config.ResolvedAgentToolSettings) ([]agent.ToolDefinition, error)
	ContextWindowTokens   int
	ProviderInputMaxBytes int
}

type chatModelAgentAssembly struct {
	Tools       []agent.ToolDefinition
	Middlewares []agent.Middleware
}

func buildChatModelAgentAssembly(ctx context.Context, cfg *config.Config, spec chatModelAgentAssemblySpec) (chatModelAgentAssembly, error) {
	workspace := ""
	projectID := ""
	var approvalRules []config.AgentApprovalRule
	if cfg != nil {
		workspace = cfg.Workspace
		projectID = cfg.ProjectID
		approvalRules = config.NormalizeAgentApprovalRules(cfg.AgentApprovalRules)
	}
	approvalMode := config.AgentApprovalAsk
	if cfg != nil {
		approvalMode = config.NormalizeAgentApprovalMode(cfg.AgentApprovalMode)
	}
	toolCatalog := agenttoolruntime.NewCatalogWithContext(ctx, cfg)
	settings := spec.ToolSettings
	middlewares := append([]agent.Middleware(nil), spec.ExtraMiddlewares...)
	middlewares = append(middlewares,
		agenttoolruntime.NewOrchestratorMiddleware(agenttoolruntime.OrchestratorConfig{
			AgentKind: spec.Kind, PolicyKind: firstNonEmpty(spec.ToolPolicyKind, spec.Kind),
			ToolSettings: spec.ToolSettings, EnforceToolSettings: true,
			EnforceApprovalPolicy: true, ApprovalMode: approvalMode,
			ProjectID: projectID, Workspace: workspace, ApprovalRules: approvalRules,
			ToolResultMaxBytes:       toolresult.LimitBytes(cfg),
			ToolResultEagerMinTokens: config.DefaultToolResultEagerMinTokens,
			ContextWindowTokens:      spec.ContextWindowTokens,
		}),
		agentrun.NewModelInputLoggingMiddleware(
			spec.Kind, spec.ModelCfg, spec.ContextWindowTokens, spec.ProviderInputMaxBytes,
		),
	)
	// Context maintenance must observe the final model call after every
	// mode-specific option and tool decision has been applied.
	middlewares = append(middlewares, agentchat.NewModelContextMiddlewares()...)
	tools := append([]agent.ToolDefinition(nil), spec.ExtraTools...)
	skillTools, readAdapters, err := buildSkillTools(ctx, cfg, spec.Kind, spec.EnableSkills, settings)
	if err != nil {
		return chatModelAgentAssembly{}, err
	}
	readAdapters = append(readAdapters, spec.ReadAdapters...)
	if spec.ReadAdaptersFactory != nil {
		extraReadAdapters, err := spec.ReadAdaptersFactory(settings)
		if err != nil {
			return chatModelAgentAssembly{}, err
		}
		readAdapters = append(readAdapters, extraReadAdapters...)
	}
	workspaceTools, err := toolCatalog.Workspace(settings, readAdapters...)
	if err != nil {
		return chatModelAgentAssembly{}, err
	}
	tools = append(tools, workspaceTools...)
	tools = append(tools, skillTools...)
	if spec.ExtraToolsFactory != nil {
		extraTools, err := spec.ExtraToolsFactory(settings)
		if err != nil {
			return chatModelAgentAssembly{}, err
		}
		tools = append(tools, extraTools...)
	}
	webTools, err := toolCatalog.WebAccess(settings)
	if err != nil {
		return chatModelAgentAssembly{}, err
	}
	tools = append(tools, webTools...)
	browserTools, err := toolCatalog.Browser(ctx, settings)
	if err != nil {
		return chatModelAgentAssembly{}, err
	}
	tools = append(tools, browserTools...)
	if err := producttools.Validate(ctx, tools); err != nil {
		return chatModelAgentAssembly{}, err
	}
	return chatModelAgentAssembly{Tools: tools, Middlewares: middlewares}, nil
}

func buildSkillTools(ctx context.Context, cfg *config.Config, agentKind string, enabled bool, settings config.ResolvedAgentToolSettings) ([]agent.ToolDefinition, []producttools.ReadAdapterBinding, error) {
	if !enabled || !settings.Allows(config.AgentToolSkills) || cfg == nil {
		return nil, nil, nil
	}
	skillBackend := novaskills.NewAgentBackend(
		novaskills.NewDirectories(cfg.SkillsDir, cfg.DataDir(), cfg.Workspace),
		agentKind,
		config.ResolveAgentSkillOverrides(cfg, agentKind),
	)
	skillTool, err := agenttoolruntime.NewCatalog(cfg).Skill(ctx, skillBackend, config.ResolveAgentContext(cfg, agentKind).MaxFragmentBytes)
	if err != nil {
		return nil, nil, fmt.Errorf("创建 Skill 工具失败 agent=%s: %w", agentKind, err)
	}
	if skillTool.Tool == nil {
		return nil, nil, nil
	}
	referenceAdapter, err := agenttoolruntime.NewCatalog(cfg).SkillReference(skillBackend)
	if err != nil {
		return nil, nil, fmt.Errorf("创建 Skill reference Adapter 失败 agent=%s: %w", agentKind, err)
	}
	return []agent.ToolDefinition{skillTool}, []producttools.ReadAdapterBinding{referenceAdapter}, nil
}

func buildConfiguredSubAgents(ctx context.Context, cfg *config.Config, parent agentBuildSpec, parentTools config.ResolvedAgentToolSettings, subConfigs []config.SubAgentConfig) ([]agent.Runnable, error) {
	if cfg == nil || !config.IsSubAgentParentKind(parent.Kind) {
		return nil, nil
	}
	subConfigs = config.SanitizeSubAgents(subConfigs)
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
	modelCfg, err := modelio.ConfigFromResolved(resolvedModel)
	if err != nil {
		return nil, fmt.Errorf("resolve sub Agent model configuration id=%s: %w", sub.ID, err)
	}
	subChatModel, err := modelio.NewChatModel(ctx, modelCfg)
	if err != nil {
		return nil, fmt.Errorf("创建子 Agent 模型失败 id=%s: %w", sub.ID, err)
	}
	toolSettings := config.ResolveSubAgentTools(parentTools, sub.Tools)
	assembly, err := buildChatModelAgentAssembly(ctx, cfg, chatModelAgentAssemblySpec{
		Kind:                  sub.ID,
		ToolPolicyKind:        parent.Kind,
		ModelCfg:              modelCfg,
		ToolSettings:          toolSettings,
		EnableSkills:          parent.EnableSkills,
		ExtraToolsFactory:     parent.ExtraToolsFactory,
		ContextWindowTokens:   resolvedModel.ContextWindowTokens,
		ProviderInputMaxBytes: config.ResolveAgentContext(cfg, parent.Kind).MaxProviderInputBytes,
	})
	if err != nil {
		return nil, err
	}
	return newNativeAgent(ctx, agent.LoopConfig{
		Name:            sub.ID,
		Description:     sub.Description,
		Instruction:     composition.Instruction(),
		Model:           subChatModel,
		MaxIterations:   configMaxIteration(cfg),
		ToolParallelism: configToolParallelism(cfg),
		Middlewares:     assembly.Middlewares,
		Tools:           assembly.Tools,
		Retry:           modelRetryConfig(cfg, nil),
	})
}

func modelRetryConfig(cfg *config.Config, outputGuard func(context.Context, *agent.RetryContext) *agent.RetryDecision) *agent.RetryConfig {
	retryConfig := &agent.RetryConfig{
		MaxRetries:  configModelMaxRetries(cfg),
		IsRetryable: modelio.IsRetryable,
	}
	if outputGuard == nil {
		return retryConfig
	}
	retryConfig.IsRetryable = nil
	retryConfig.ShouldRetry = func(ctx context.Context, retryCtx *agent.RetryContext) *agent.RetryDecision {
		if retryCtx != nil && retryCtx.Err != nil {
			return &agent.RetryDecision{Retry: modelio.IsRetryable(ctx, retryCtx.Err)}
		}
		return outputGuard(ctx, retryCtx)
	}
	return retryConfig
}

func buildSubAgentInstruction(parent agentBuildSpec, sub config.SubAgentConfig) string {
	composition, err := composeSubAgentInstruction(&config.Config{}, parent, sub)
	if err != nil {
		return ""
	}
	return composition.Instruction()
}

func composeSubAgentInstruction(cfg *config.Config, parent agentBuildSpec, sub config.SubAgentConfig) (prompts.SystemPromptComposition, error) {
	parentComposition, err := resolveAgentSystemPrompt(cfg, parent)
	if err != nil {
		return prompts.SystemPromptComposition{}, err
	}
	return prompts.ComposeSubAgentInstruction(cfg, parentComposition, sub)
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

func configToolParallelism(cfg *config.Config) int {
	if cfg == nil || cfg.AgentToolParallelism <= 0 {
		return config.DefaultAgentToolParallelism
	}
	if cfg.AgentToolParallelism > config.MaxAgentToolParallelism {
		return config.MaxAgentToolParallelism
	}
	return cfg.AgentToolParallelism
}
