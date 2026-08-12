package agents

import (
	"context"
	"crypto/sha256"
	agentchat "denova/internal/agents/chat"
	agentinteractive "denova/internal/agents/interactive"
	agentlifecycle "denova/internal/agents/lifecycle"
	agenttoolruntime "denova/internal/agents/toolruntime"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	agent "github.com/alfredxw/denova/agent"
	"github.com/alfredxw/denova/agent/providers"
	agenttoolresult "github.com/alfredxw/denova/agent/toolresult"
	publictools "github.com/alfredxw/denova/agent/tools"

	"denova/config"
	agentcompaction "denova/internal/agents/context/compaction"
	agentconversation "denova/internal/agents/conversation"
	agentdelegation "denova/internal/agents/delegation"
	"denova/internal/agents/harnessstate"
	"denova/internal/agents/modelio"
	"denova/internal/agents/prompts"
	agentrun "denova/internal/agents/run"
	novaskills "denova/internal/agents/skills"
	"denova/internal/agents/toolresult"
	producttools "denova/internal/agents/tools"
	"denova/internal/book"
)

// ToolDefinition keeps application packages on Denova's Agent boundary rather
// than importing the provider runtime directly.
type ToolDefinition = agent.ToolDefinition
type Definition = agent.Definition

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

// buildAgentDefinition is Denova's public Agent composition root. Root and
// delegated children are Definitions; execution always enters through a
// durable Agent Session/Run owned by the execution adapter.
func buildAgentDefinition(ctx context.Context, cfg *config.Config, spec agentBuildSpec) (agent.Definition, error) {
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
	var taskAgents []agentdelegation.Child
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
			general, err := buildChildDefinition(cfg, childDefinitionSpec{
				ParentKind:  spec.Kind,
				Name:        producttools.GeneralSubAgentName,
				Description: "通用子 Agent，用于研究复杂问题、搜索代码和执行独立的多步骤任务。",
				Composition: childComposition,
				Model:       chatModel, ModelIdentity: modelIdentity,
				ModelContextWindow: config.ResolveAgentModel(cfg, spec.Kind).ContextWindowTokens,
				Tools:              generalAssembly.Tools, Middlewares: generalAssembly.Middlewares,
			})
			if err != nil {
				return agent.Definition{}, fmt.Errorf("创建通用子 Agent 失败: %w", err)
			}
			taskAgents = append([]agentdelegation.Child{general}, taskAgents...)
		}
	}

	tools := append([]agent.ToolDefinition(nil), assembly.Tools...)
	var builtinToolsets []agent.CapabilityIdentity
	if !spec.DisableWriteTodos && toolSettings.Allows(config.AgentToolTodo) {
		todoToolset, err := publictools.Todo()
		if err != nil {
			return agent.Definition{}, fmt.Errorf("创建 todo 工具失败: %w", err)
		}
		prepared, err := todoToolset.PrepareTools(ctx, agent.ToolRequest{})
		if err != nil {
			return agent.Definition{}, fmt.Errorf("准备 todo 工具失败: %w", err)
		}
		tools = append(tools, prepared...)
		builtinToolsets = append(builtinToolsets, todoToolset.Identity())
	}
	if spec.InteractiveHost && toolSettings.Allows(config.AgentToolAsk) && (spec.Kind == config.AgentKindGeneral || spec.Kind == config.AgentKindIDE || spec.Kind == config.AgentKindConfigManager || spec.Kind == config.AgentKindHarnessOptimizer) {
		askToolset, err := publictools.Ask()
		if err != nil {
			return agent.Definition{}, fmt.Errorf("创建 ask 工具失败: %w", err)
		}
		prepared, err := askToolset.PrepareTools(ctx, agent.ToolRequest{})
		if err != nil {
			return agent.Definition{}, fmt.Errorf("准备 ask 工具失败: %w", err)
		}
		tools = append(tools, prepared...)
		builtinToolsets = append(builtinToolsets, askToolset.Identity())
	}
	if toolSettings.Allows(config.AgentToolDelegation) {
		if len(taskAgents) == 0 {
			return agent.Definition{}, fmt.Errorf("delegation is enabled for %s but no child Agent is configured", spec.Kind)
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
	permissionMode := config.AgentApprovalAsk
	var permissionRules []config.AgentApprovalRule
	if cfg != nil {
		permissionMode = config.NormalizeAgentApprovalMode(cfg.AgentApprovalMode)
		permissionRules = config.NormalizeAgentApprovalRules(cfg.AgentApprovalRules)
	}
	permission, err := agentlifecycle.NewPermissionPolicy(agentlifecycle.PermissionConfig{
		Mode: permissionMode, ProjectID: configProjectID(cfg), Workspace: configWorkspace(cfg),
		Rules: permissionRules,
	})
	if err != nil {
		return agent.Definition{}, fmt.Errorf("create Agent Permission policy kind=%s: %w", spec.Kind, err)
	}
	var goalManager agent.GoalManager
	if toolSettings.Allows(config.AgentToolGoal) {
		goalManager = agentlifecycle.NewGoalManager()
	}
	rootTools, err := agent.StaticToolsIdentified(denovaCapabilityIdentity("denova.tools", struct {
		Kind      string
		ProjectID string
		Workspace string
		Settings  config.ResolvedAgentToolSettings
		Builtins  []agent.CapabilityIdentity
	}{spec.Kind, configProjectID(cfg), configWorkspace(cfg), toolSettings, builtinToolsets}), tools...)
	if err != nil {
		return agent.Definition{}, fmt.Errorf("construct root Agent Toolset kind=%s: %w", spec.Kind, err)
	}
	var definitionTools agent.Toolset = rootTools
	if len(taskAgents) > 0 {
		taskDescription := harness.ToolDescriptions()["task"]
		catalog, err := agentdelegation.NewCatalog(rootTools, agentdelegation.Config{
			Capability:     config.AgentToolDelegation,
			Description:    taskDescription,
			MaxResultBytes: toolresult.LimitBytes(cfg),
		}, taskAgents...)
		if err != nil {
			return agent.Definition{}, fmt.Errorf("create durable task Toolset: %w", err)
		}
		definitionTools = catalog
	}
	return agent.Definition{
		Key:           "denova." + spec.Kind,
		Name:          spec.Name,
		Description:   spec.Description,
		Model:         chatModel,
		ModelIdentity: modelIdentity,
		Instructions:  composition.Instruction(),
		Context:       harness.ContextSource(cfg, spec.Kind),
		Tools:         definitionTools,
		Middlewares:   middlewares,
		ResultProcessor: agenttoolresult.Standard(agenttoolresult.Policy{
			MaxBytes:            toolresult.LimitBytes(cfg),
			EagerMinTokens:      config.DefaultToolResultEagerMinTokens,
			ContextWindowTokens: config.ResolveAgentModel(cfg, spec.Kind).ContextWindowTokens,
		}),
		Cleanup:    agentconversation.NewAgentCleanupManager(cfg, spec.Kind),
		Compaction: compaction,
		Goal:       goalManager,
		Permission: permission,
		Execution: agent.ExecutionPolicy{
			Retry: retry,
			RetryIdentity: denovaCapabilityIdentity("denova.retry", struct {
				MaxRetries  int
				OutputGuard bool
			}{configModelMaxRetries(cfg), spec.ModelOutputGuard != nil}),
			MaxIterations: configMaxIteration(cfg), ToolParallelism: configToolParallelism(cfg), IdleTimeout: configIdleTimeout(cfg),
			MaxAutomaticCompactionFailures: config.DefaultContextCompactionMaxConsecutiveFailures,
		},
	}, nil
}

func identifyDenovaMiddlewares(kind string, cfg *config.Config, middlewares []agent.Middleware) []agent.Middleware {
	identified := make([]agent.Middleware, len(middlewares))
	for index, middleware := range middlewares {
		if _, ok := middleware.(agent.IdentifiedMiddleware); ok {
			identified[index] = middleware
			continue
		}
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
	if cfg != nil {
		workspace = cfg.Workspace
	}
	toolCatalog := agenttoolruntime.NewCatalogWithContext(ctx, cfg)
	settings := spec.ToolSettings
	middlewares := append([]agent.Middleware(nil), spec.ExtraMiddlewares...)
	middlewares = append(middlewares,
		agenttoolruntime.NewOrchestratorMiddleware(agenttoolruntime.OrchestratorConfig{
			AgentKind: spec.Kind, PolicyKind: firstNonEmpty(spec.ToolPolicyKind, spec.Kind),
			ToolSettings: spec.ToolSettings, EnforceToolSettings: true,
			Workspace: workspace, ToolResultMaxBytes: toolresult.LimitBytes(cfg),
		}),
		agentrun.NewModelInputLoggingMiddleware(
			spec.Kind, spec.ModelCfg, spec.ContextWindowTokens, spec.ProviderInputMaxBytes,
		),
	)
	// Context maintenance must observe the final model call after every
	// mode-specific option and tool decision has been applied.
	middlewares = append(middlewares, agentchat.NewModelContextMiddlewares(
		toolresult.ResolveContextPolicy(cfg, firstNonEmpty(spec.ToolPolicyKind, spec.Kind)),
	)...)
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

func buildConfiguredSubAgents(ctx context.Context, cfg *config.Config, parent agentBuildSpec, parentTools config.ResolvedAgentToolSettings, subConfigs []config.SubAgentConfig) ([]agentdelegation.Child, error) {
	if cfg == nil || !config.IsSubAgentParentKind(parent.Kind) {
		return nil, nil
	}
	subConfigs = config.SanitizeSubAgents(subConfigs)
	if len(subConfigs) == 0 {
		return nil, nil
	}
	subAgents := make([]agentdelegation.Child, 0, len(subConfigs))
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

func buildConfiguredSubAgent(ctx context.Context, cfg *config.Config, parent agentBuildSpec, parentTools config.ResolvedAgentToolSettings, sub config.SubAgentConfig) (agentdelegation.Child, error) {
	composition, err := composeSubAgentInstruction(cfg, parent, sub)
	if err != nil {
		return agentdelegation.Child{}, fmt.Errorf("assemble sub Agent system prompt id=%s: %w", sub.ID, err)
	}
	resolvedModel := config.ResolveSubAgentModel(cfg, parent.Kind, sub)
	modelCfg, err := modelio.ConfigFromResolved(resolvedModel)
	if err != nil {
		return agentdelegation.Child{}, fmt.Errorf("resolve sub Agent model configuration id=%s: %w", sub.ID, err)
	}
	subChatModel, err := modelio.NewChatModel(ctx, modelCfg)
	if err != nil {
		return agentdelegation.Child{}, fmt.Errorf("创建子 Agent 模型失败 id=%s: %w", sub.ID, err)
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
		return agentdelegation.Child{}, err
	}
	modelIdentity, err := providers.ModelIdentity(modelCfg)
	if err != nil {
		return agentdelegation.Child{}, fmt.Errorf("resolve sub Agent model identity id=%s: %w", sub.ID, err)
	}
	return buildChildDefinition(cfg, childDefinitionSpec{
		ParentKind: parent.Kind, Name: sub.ID, Description: sub.Description,
		Composition: composition, Model: subChatModel, ModelIdentity: modelIdentity,
		ModelContextWindow: resolvedModel.ContextWindowTokens,
		Tools:              assembly.Tools, Middlewares: assembly.Middlewares,
	})
}

type childDefinitionSpec struct {
	ParentKind         string
	Name               string
	Description        string
	Composition        prompts.SystemPromptComposition
	Model              agent.BaseChatModel
	ModelIdentity      agent.CapabilityIdentity
	ModelContextWindow int
	Tools              []agent.ToolDefinition
	Middlewares        []agent.Middleware
}

func buildChildDefinition(cfg *config.Config, spec childDefinitionSpec) (agentdelegation.Child, error) {
	compaction, err := agentcompaction.NewAgentManagerForModel(
		cfg, spec.ParentKind, spec.ModelContextWindow, spec.Model, spec.ModelIdentity,
	)
	if err != nil {
		return agentdelegation.Child{}, err
	}
	permissionMode := config.AgentApprovalAsk
	var permissionRules []config.AgentApprovalRule
	if cfg != nil {
		permissionMode = config.NormalizeAgentApprovalMode(cfg.AgentApprovalMode)
		permissionRules = config.NormalizeAgentApprovalRules(cfg.AgentApprovalRules)
	}
	permission, err := agentlifecycle.NewPermissionPolicy(agentlifecycle.PermissionConfig{
		Mode: permissionMode, ProjectID: configProjectID(cfg), Workspace: configWorkspace(cfg), Rules: permissionRules,
	})
	if err != nil {
		return agentdelegation.Child{}, err
	}
	tools, err := agent.StaticToolsIdentified(denovaCapabilityIdentity("denova.child.tools", struct {
		Parent string
		Name   string
	}{spec.ParentKind, spec.Name}), spec.Tools...)
	if err != nil {
		return agentdelegation.Child{}, fmt.Errorf("construct delegated Agent Toolset %q: %w", spec.Name, err)
	}
	definition := agent.Definition{
		Key:  "denova." + spec.ParentKind + ".child." + spec.Name,
		Name: spec.Name, Description: spec.Description,
		Model: spec.Model, ModelIdentity: spec.ModelIdentity,
		Instructions: spec.Composition.Instruction(), Tools: tools,
		Middlewares: identifyDenovaMiddlewares(spec.ParentKind+".child."+spec.Name, cfg, spec.Middlewares),
		ResultProcessor: agenttoolresult.Standard(agenttoolresult.Policy{
			MaxBytes: toolresult.LimitBytes(cfg), EagerMinTokens: config.DefaultToolResultEagerMinTokens,
			ContextWindowTokens: spec.ModelContextWindow,
		}),
		Cleanup: agentconversation.NewAgentCleanupManager(cfg, spec.ParentKind),
		// Goals are a root product workflow. Delegated Agents keep isolated
		// task transcripts and must not create or continue a parent Goal.
		Compaction: compaction, Permission: permission,
		Execution: agent.ExecutionPolicy{
			Retry: modelRetryConfig(cfg, nil),
			RetryIdentity: denovaCapabilityIdentity("denova.child.retry", struct {
				Parent, Name string
				MaxRetries   int
			}{
				spec.ParentKind, spec.Name, configModelMaxRetries(cfg),
			}),
			MaxIterations: configMaxIteration(cfg), ToolParallelism: configToolParallelism(cfg), IdleTimeout: configIdleTimeout(cfg),
			MaxAutomaticCompactionFailures: config.DefaultContextCompactionMaxConsecutiveFailures,
		},
	}
	behavior, err := agent.DefinitionBehaviorIdentity(definition)
	if err != nil {
		return agentdelegation.Child{}, fmt.Errorf("fingerprint delegated Agent %q: %w", spec.Name, err)
	}
	identity := agent.CapabilityIdentity{Kind: "denova.child", Version: 1, ConfigHash: behavior}
	return agentdelegation.Child{
		Name: spec.Name, Description: spec.Description, Definition: definition, Identity: identity,
	}, nil
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

func configIdleTimeout(cfg *config.Config) time.Duration {
	if cfg == nil || cfg.AgentIdleTimeoutSeconds <= 0 {
		return 0
	}
	return time.Duration(cfg.AgentIdleTimeoutSeconds) * time.Second
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
