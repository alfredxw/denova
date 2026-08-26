package agents

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	agent "github.com/alfredxw/denova/agent"
	"github.com/alfredxw/denova/agent/providers"
	agenttoolresult "github.com/alfredxw/denova/agent/toolresult"
	publictools "github.com/alfredxw/denova/agent/tools"

	"denova/config"
	agentchat "denova/internal/agents/chat"
	agentcompaction "denova/internal/agents/context/compaction"
	agentconversation "denova/internal/agents/conversation"
	agentdelegation "denova/internal/agents/delegation"
	"denova/internal/agents/harnessstate"
	agentinteractive "denova/internal/agents/interactive"
	agentlifecycle "denova/internal/agents/lifecycle"
	"denova/internal/agents/modelio"
	"denova/internal/agents/prompts"
	agentrun "denova/internal/agents/run"
	"denova/internal/agents/scripttools"
	"denova/internal/agents/skillassembly"
	"denova/internal/agents/toolresult"
	agenttoolruntime "denova/internal/agents/toolruntime"
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
}

// BuildDefinitionWithCompositionForHost returns the complete public Agent
// composition for a writing/IDE Project Session.
func BuildDefinitionWithCompositionForHost(ctx context.Context, cfg *config.Config, state *book.State, teller prompts.IDEStoryTeller, host AgentHostCapabilities) (agent.Definition, prompts.SystemPromptComposition, error) {
	composition, err := prompts.ComposeInstruction(cfg, state, teller)
	if err != nil {
		return agent.Definition{}, prompts.SystemPromptComposition{}, err
	}
	assembly, err := buildAgentDefinitionWithComposition(ctx, cfg, agentBuildSpec{
		Kind:              config.AgentKindIDE,
		Name:              "DenovaAgent",
		Description:       "AI novel-writing assistant",
		Composition:       composition,
		ProjectState:      state,
		EnableSkills:      true,
		InteractiveHost:   host.Interactive,
		ExtraTools:        host.RootTools,
		ExtraToolsFactory: agenttoolruntime.NewCatalog(cfg).IDE(),
		ReadAdapters:      host.ReadAdapters,
	})
	return assembly.Definition, assembly.Composition, err
}

// BuildGeneralDefinitionWithCompositionForHost returns the complete public
// Agent composition for a general Project Session.
func BuildGeneralDefinitionWithCompositionForHost(ctx context.Context, cfg *config.Config, state *book.State, host AgentHostCapabilities) (agent.Definition, prompts.SystemPromptComposition, error) {
	composition, err := prompts.ComposeGeneralInstruction(cfg)
	if err != nil {
		return agent.Definition{}, prompts.SystemPromptComposition{}, err
	}
	assembly, err := buildAgentDefinitionWithComposition(ctx, cfg, agentBuildSpec{
		Kind:            config.AgentKindGeneral,
		Name:            "DenovaGeneralAgent",
		Description:     "General-purpose project Agent",
		Composition:     composition,
		ProjectState:    state,
		EnableSkills:    true,
		InteractiveHost: host.Interactive,
		ExtraTools:      host.RootTools,
		ReadAdapters:    host.ReadAdapters,
	})
	return assembly.Definition, assembly.Composition, err
}

// BuildHarnessDefinitionWithCompositionForHost assembles the system-managed
// Harness Project Agent with ordinary workspace tools and trajectory adapters.
func BuildHarnessDefinitionWithCompositionForHost(ctx context.Context, cfg *config.Config, host AgentHostCapabilities) (agent.Definition, prompts.SystemPromptComposition, error) {
	composition, err := prompts.ComposeHarnessInstruction(cfg)
	if err != nil {
		return agent.Definition{}, prompts.SystemPromptComposition{}, err
	}
	assembly, err := buildAgentDefinitionWithComposition(ctx, cfg, agentBuildSpec{
		Kind:            config.AgentKindHarness,
		Name:            "DenovaHarnessAgent",
		Description:     "Maintains user-level Harness State from trajectory evidence",
		Composition:     composition,
		EnableSkills:    true,
		InteractiveHost: host.Interactive,
		ExtraTools:      host.RootTools,
		ReadAdapters:    host.ReadAdapters,
	})
	return assembly.Definition, assembly.Composition, err
}

func BuildInteractiveStoryDefinitionWithCompositionForHost(
	ctx context.Context,
	cfg *config.Config,
	state *book.State,
	teller prompts.InteractiveStorySystemInstructionInput,
	host AgentHostCapabilities,
	toolContexts ...agentinteractive.InteractiveStoryToolContext,
) (agent.Definition, prompts.SystemPromptComposition, error) {
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
	assembly, err := buildAgentDefinitionWithComposition(ctx, cfg, agentBuildSpec{
		Kind:              config.AgentKindInteractiveStory,
		Name:              "DenovaInteractiveStoryAgent",
		Description:       "AI interactive-story narrator",
		Composition:       composition,
		ProjectState:      state,
		EnableSkills:      true,
		InteractiveHost:   host.Interactive,
		DisableWriteTodos: true,
		ExtraTools:        host.RootTools,
		ReadAdapters:      host.ReadAdapters,
		ExtraMiddlewares:  handlers,
		ExtraToolsFactory: agenttoolruntime.NewCatalog(cfg).InteractiveStory(agenttoolruntime.ProjectInteractiveContext(toolContexts...)),
		ModelOutputGuard:  outputGuard,
	})
	return assembly.Definition, assembly.Composition, err
}

func BuildInteractiveDirectorDefinitionWithComposition(ctx context.Context, cfg *config.Config, state *book.State, toolContexts ...agentinteractive.InteractiveStoryToolContext) (agent.Definition, prompts.SystemPromptComposition, error) {
	composition, err := prompts.ComposeInteractiveDirectorInstruction(cfg, state)
	if err != nil {
		return agent.Definition{}, prompts.SystemPromptComposition{}, err
	}
	toolContext := agenttoolruntime.ProjectInteractiveContext(toolContexts...)
	assembly, err := buildAgentDefinitionWithComposition(ctx, cfg, agentBuildSpec{
		Kind:                config.AgentKindInteractiveDirector,
		Name:                "DenovaInteractiveDirectorAgent",
		Description:         "AI background director for interactive stories",
		Composition:         composition,
		ProjectState:        state,
		EnableSkills:        false,
		DisableWriteTodos:   true,
		ExtraMiddlewares:    []agent.Middleware{agenttoolruntime.NewInteractiveDirectorPlanFileMiddleware()},
		ReadAdaptersFactory: agenttoolruntime.NewCatalog(cfg).InteractiveDirectorRead(toolContext),
		ExtraToolsFactory:   agenttoolruntime.NewCatalog(cfg).InteractiveDirector(toolContext),
	})
	return assembly.Definition, assembly.Composition, err
}

func BuildConfigManagerDefinitionWithCompositionForHost(ctx context.Context, cfg *config.Config, state *book.State, host AgentHostCapabilities, resourceSkills ...prompts.ConfigManagerResourceSkill) (agent.Definition, prompts.SystemPromptComposition, error) {
	composition, err := prompts.ComposeConfigManagerInstruction(cfg, state, resourceSkills...)
	if err != nil {
		return agent.Definition{}, prompts.SystemPromptComposition{}, err
	}
	assembly, err := buildAgentDefinitionWithComposition(ctx, cfg, agentBuildSpec{
		Kind:              config.AgentKindConfigManager,
		Name:              "DenovaConfigManagerAgent",
		Description:       "AI configuration and resource-management assistant",
		Composition:       composition,
		ProjectState:      state,
		EnableSkills:      true,
		InteractiveHost:   host.Interactive,
		ExtraToolsFactory: agenttoolruntime.NewCatalog(cfg).ConfigManager(),
	})
	return assembly.Definition, assembly.Composition, err
}

func BuildImageDefinitionWithComposition(ctx context.Context, cfg *config.Config, state *book.State, systemPrompt string) (agent.Definition, prompts.SystemPromptComposition, error) {
	composition, err := prompts.ComposeImageInstruction(cfg, state, systemPrompt)
	if err != nil {
		return agent.Definition{}, prompts.SystemPromptComposition{}, err
	}
	assembly, err := buildAgentDefinitionWithComposition(ctx, cfg, agentBuildSpec{
		Kind:              config.AgentKindImage,
		Name:              "DenovaImageAgent",
		Description:       "AI image-generation assistant",
		Composition:       composition,
		ProjectState:      state,
		EnableSkills:      true,
		DisableWriteTodos: true,
		ExtraToolsFactory: agenttoolruntime.NewCatalog(cfg).Image(),
	})
	return assembly.Definition, assembly.Composition, err
}

type agentBuildSpec struct {
	Kind                string
	Name                string
	Description         string
	Composition         prompts.SystemPromptComposition
	ProjectState        *book.State
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

type agentDefinitionAssembly struct {
	Definition  agent.Definition
	Composition prompts.SystemPromptComposition
}

func buildAgentDefinition(ctx context.Context, cfg *config.Config, spec agentBuildSpec) (agent.Definition, error) {
	assembly, err := buildAgentDefinitionWithComposition(ctx, cfg, spec)
	return assembly.Definition, err
}

// buildAgentDefinitionWithComposition is Denova's public Agent composition
// root. Root and delegated children are Definitions; execution always enters
// through a durable Agent Session/Run owned by the execution adapter. The
// returned composition is the exact post-capability prompt consumed by the
// model and shared with runtime inspection and logging.
func buildAgentDefinitionWithComposition(ctx context.Context, cfg *config.Config, spec agentBuildSpec) (agentDefinitionAssembly, error) {
	composition, err := resolveAgentSystemPrompt(cfg, spec)
	if err != nil {
		return agentDefinitionAssembly{}, err
	}
	harness, err := harnessstate.Load(ctx, cfg)
	if err != nil {
		return agentDefinitionAssembly{}, fmt.Errorf("load Harness State for Agent %s: %w", spec.Kind, err)
	}
	projectContext, err := agentlifecycle.NewProjectInstructionsContextSource(cfg, spec.Kind, spec.ProjectState)
	if err != nil {
		return agentDefinitionAssembly{}, fmt.Errorf("create project instructions context for Agent %s: %w", spec.Kind, err)
	}
	definitionContext, err := agent.CombineContextSources(projectContext, harness.ContextSource(cfg, spec.Kind))
	if err != nil {
		return agentDefinitionAssembly{}, fmt.Errorf("compose ContextSource for Agent %s: %w", spec.Kind, err)
	}
	childComposition, err := prompts.AppendUserStatePrompt(cfg, composition, harness.Prompt(spec.Kind))
	if err != nil {
		return agentDefinitionAssembly{}, fmt.Errorf("compose Harness State prompt for child Agents of %s: %w", spec.Kind, err)
	}
	childSpec := spec
	childSpec.Composition = childComposition
	modelCfg, err := modelio.ConfigForAgent(cfg, spec.Kind)
	if err != nil {
		return agentDefinitionAssembly{}, fmt.Errorf("resolve model configuration: %w", err)
	}
	toolSettings := config.ResolveAgentTools(cfg, spec.Kind)
	chatModel, err := modelio.NewChatModel(ctx, modelCfg)
	if err != nil {
		return agentDefinitionAssembly{}, fmt.Errorf("create model: %w", err)
	}
	modelIdentity, err := providers.ModelIdentity(modelCfg)
	if err != nil {
		return agentDefinitionAssembly{}, fmt.Errorf("resolve model capability identity: %w", err)
	}

	assembly, err := buildChatModelAgentAssembly(ctx, cfg, chatModelAgentAssemblySpec{
		Kind:                spec.Kind,
		SystemPrompt:        composition,
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
		return agentDefinitionAssembly{}, err
	}
	savedScriptTools, err := scripttools.Saved(cfg, harness, spec.Kind)
	if err != nil {
		return agentDefinitionAssembly{}, err
	}
	var taskAgents []agentdelegation.Child
	if toolSettings.Allows(config.AgentToolDelegation) {
		configuredSubAgents, err := buildConfiguredSubAgents(
			ctx, cfg, childSpec, toolSettings, projectContext, harness.SubAgents(), savedScriptTools,
		)
		if err != nil {
			return agentDefinitionAssembly{}, err
		}
		taskAgents = append(taskAgents, configuredSubAgents...)
		if config.GeneralSubAgentEnabled(cfg, spec.Kind) {
			generalAssembly, err := buildChatModelAgentAssembly(ctx, cfg, chatModelAgentAssemblySpec{
				Kind:                  producttools.GeneralSubAgentName,
				SystemPrompt:          childComposition,
				ToolPolicyKind:        spec.Kind,
				ModelCfg:              modelCfg,
				ToolSettings:          toolSettings,
				EnableSkills:          spec.EnableSkills,
				ExtraToolsFactory:     spec.ExtraToolsFactory,
				ContextWindowTokens:   config.ResolveAgentModel(cfg, spec.Kind).ContextWindowTokens,
				ProviderInputMaxBytes: config.ResolveAgentContext(cfg, spec.Kind).MaxProviderInputBytes,
			})
			if err != nil {
				return agentDefinitionAssembly{}, fmt.Errorf("assemble general-purpose child Agent tools: %w", err)
			}
			general, err := buildChildDefinition(cfg, childDefinitionSpec{
				ParentKind:  spec.Kind,
				Name:        producttools.GeneralSubAgentName,
				Description: "Use for an independently scoped research, code-investigation, or multi-step execution task that returns findings to the parent Agent.",
				Composition: generalAssembly.SystemPrompt,
				Context:     projectContext,
				Model:       chatModel, ModelIdentity: modelIdentity,
				ModelContextWindow: config.ResolveAgentModel(cfg, spec.Kind).ContextWindowTokens,
				Tools:              generalAssembly.Tools, Middlewares: generalAssembly.Middlewares,
			})
			if err != nil {
				return agentDefinitionAssembly{}, fmt.Errorf("create general-purpose child Agent: %w", err)
			}
			taskAgents = append([]agentdelegation.Child{general}, taskAgents...)
		}
	}

	tools := append([]agent.ToolDefinition(nil), assembly.Tools...)
	tools = append(tools, savedScriptTools...)
	var builtinToolsets []agent.CapabilityIdentity
	if !spec.DisableWriteTodos && toolSettings.Allows(config.AgentToolTodo) {
		todoToolset := publictools.Todo()
		prepared, err := todoToolset.PrepareTools(ctx, agent.ToolRequest{})
		if err != nil {
			return agentDefinitionAssembly{}, fmt.Errorf("prepare todo tool: %w", err)
		}
		tools = append(tools, prepared...)
		builtinToolsets = append(builtinToolsets, todoToolset.Identity())
	}
	if spec.InteractiveHost && toolSettings.Allows(config.AgentToolAsk) && (spec.Kind == config.AgentKindGeneral || spec.Kind == config.AgentKindIDE || spec.Kind == config.AgentKindConfigManager || spec.Kind == config.AgentKindHarness) {
		askToolset := publictools.Ask()
		prepared, err := askToolset.PrepareTools(ctx, agent.ToolRequest{})
		if err != nil {
			return agentDefinitionAssembly{}, fmt.Errorf("prepare ask tool: %w", err)
		}
		tools = append(tools, prepared...)
		builtinToolsets = append(builtinToolsets, askToolset.Identity())
	}
	if toolSettings.Allows(config.AgentToolDelegation) {
		if len(taskAgents) == 0 {
			return agentDefinitionAssembly{}, fmt.Errorf("delegation is enabled for %s but no child Agent is configured", spec.Kind)
		}
	}
	tools = harness.ApplyToolDescriptions(tools)
	manifest := config.ResolveAgentToolManifestForGOOS(toolSettings, spec.Kind, "", toolresult.LimitBytes(cfg))
	if err := producttools.ValidateAgainstManifest(ctx, tools, manifest); err != nil {
		return agentDefinitionAssembly{}, err
	}

	middlewares := identifyDenovaMiddlewares(spec.Kind, cfg, assembly.Middlewares)
	retry := modelRetryConfig(cfg, spec.ModelOutputGuard)
	compaction, err := agentcompaction.NewAgentManager(cfg, spec.Kind, chatModel, modelIdentity)
	if err != nil {
		return agentDefinitionAssembly{}, fmt.Errorf("create Agent Compaction manager kind=%s: %w", spec.Kind, err)
	}
	permissionMode := config.AgentApprovalAsk
	var permissionRules []config.AgentApprovalRule
	if cfg != nil {
		permissionMode = config.NormalizeAgentApprovalMode(cfg.AgentApprovalMode)
		permissionRules = config.NormalizeAgentApprovalRules(cfg.AgentApprovalRules)
	}
	permission, err := agentlifecycle.NewPermissionPolicy(agentlifecycle.PermissionConfig{
		Mode: permissionMode, AgentKind: spec.Kind,
		ProjectID: configProjectID(cfg), Workspace: configWorkspace(cfg),
		Rules: permissionRules,
	})
	if err != nil {
		return agentDefinitionAssembly{}, fmt.Errorf("create Agent Permission policy kind=%s: %w", spec.Kind, err)
	}
	var goalManager agent.GoalManager
	switch spec.Kind {
	case config.AgentKindGeneral, config.AgentKindHarness, config.AgentKindIDE, config.AgentKindInteractiveStory:
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
		return agentDefinitionAssembly{}, fmt.Errorf("construct root Agent Toolset kind=%s: %w", spec.Kind, err)
	}
	var definitionTools agent.Toolset = rootTools
	if len(taskAgents) > 0 {
		validationIdentity, validateManifest, validationErr := producttools.ManifestValidator(manifest)
		if validationErr != nil {
			return agentDefinitionAssembly{}, fmt.Errorf("identify Agent tool manifest kind=%s: %w", spec.Kind, validationErr)
		}
		taskDescription := harness.ToolDescriptions()["task"]
		catalog, err := agentdelegation.NewCatalog(rootTools, agentdelegation.Config{
			Capability:         config.AgentToolDelegation,
			Description:        taskDescription,
			MaxResultBytes:     toolresult.LimitBytes(cfg),
			ValidationIdentity: validationIdentity,
			Validate:           validateManifest,
		}, taskAgents...)
		if err != nil {
			return agentDefinitionAssembly{}, fmt.Errorf("create durable task Toolset: %w", err)
		}
		definitionTools = catalog
	}
	return agentDefinitionAssembly{Definition: agent.Definition{
		Key:           "denova." + spec.Kind,
		Name:          spec.Name,
		Description:   spec.Description,
		Model:         chatModel,
		ModelIdentity: modelIdentity,
		Instructions:  assembly.SystemPrompt.Instruction(),
		Context:       definitionContext,
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
	}, Composition: assembly.SystemPrompt}, nil
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
	SystemPrompt          prompts.SystemPromptComposition
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
	SystemPrompt prompts.SystemPromptComposition
	Tools        []agent.ToolDefinition
	Middlewares  []agent.Middleware
}

func buildChatModelAgentAssembly(ctx context.Context, cfg *config.Config, spec chatModelAgentAssemblySpec) (chatModelAgentAssembly, error) {
	workspace := ""
	if cfg != nil {
		workspace = cfg.Workspace
	}
	toolCatalog := agenttoolruntime.NewCatalogWithContext(ctx, cfg)
	settings := spec.ToolSettings
	skills, err := skillassembly.Build(ctx, cfg, spec.Kind, spec.EnableSkills, settings, spec.SystemPrompt)
	if err != nil {
		return chatModelAgentAssembly{}, err
	}
	systemPrompt := skills.SystemPrompt
	middlewares := append([]agent.Middleware(nil), spec.ExtraMiddlewares...)
	middlewares = append(middlewares,
		agenttoolruntime.NewOrchestratorMiddleware(agenttoolruntime.OrchestratorConfig{
			AgentKind: spec.Kind, PolicyKind: firstNonEmpty(spec.ToolPolicyKind, spec.Kind),
			ToolSettings: spec.ToolSettings, EnforceToolSettings: true,
			Workspace: workspace, ToolResultMaxBytes: toolresult.LimitBytes(cfg),
		}),
		agentrun.NewModelInputLoggingMiddleware(
			spec.Kind, spec.ModelCfg, spec.ContextWindowTokens, spec.ProviderInputMaxBytes, systemPrompt,
		),
	)
	// Context maintenance must observe the final model call after every
	// mode-specific option and tool decision has been applied.
	middlewares = append(middlewares, agentchat.NewModelContextMiddlewares(
		toolresult.ResolveContextPolicy(cfg, firstNonEmpty(spec.ToolPolicyKind, spec.Kind)),
	)...)
	tools := append([]agent.ToolDefinition(nil), spec.ExtraTools...)
	skillTools := skills.Tools
	readAdapters := skills.ReadAdapters
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
	if settings.Allows(config.AgentToolScript) {
		scriptDefinition, err := scripttools.Immediate(cfg)
		if err != nil {
			return chatModelAgentAssembly{}, err
		}
		tools = append(tools, scriptDefinition)
	}
	if err := producttools.Validate(ctx, tools); err != nil {
		return chatModelAgentAssembly{}, err
	}
	return chatModelAgentAssembly{SystemPrompt: systemPrompt, Tools: tools, Middlewares: middlewares}, nil
}

func buildConfiguredSubAgents(
	ctx context.Context,
	cfg *config.Config,
	parent agentBuildSpec,
	parentTools config.ResolvedAgentToolSettings,
	projectContext agent.ContextSource,
	subConfigs []config.SubAgentConfig,
	savedScriptTools []agent.ToolDefinition,
) ([]agentdelegation.Child, error) {
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
		subAgent, err := buildConfiguredSubAgent(ctx, cfg, parent, parentTools, projectContext, sub, savedScriptTools)
		if err != nil {
			return nil, err
		}
		subAgents = append(subAgents, subAgent)
	}
	return subAgents, nil
}

func buildConfiguredSubAgent(
	ctx context.Context,
	cfg *config.Config,
	parent agentBuildSpec,
	parentTools config.ResolvedAgentToolSettings,
	projectContext agent.ContextSource,
	sub config.SubAgentConfig,
	savedScriptTools []agent.ToolDefinition,
) (agentdelegation.Child, error) {
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
		SystemPrompt:          composition,
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
	selectedScriptTools, err := scripttools.ForSubAgent(ctx, savedScriptTools, sub.Tools)
	if err != nil {
		return agentdelegation.Child{}, fmt.Errorf("select Script Tools for sub Agent %s: %w", sub.ID, err)
	}
	assembly.Tools = append(assembly.Tools, selectedScriptTools...)
	modelIdentity, err := providers.ModelIdentity(modelCfg)
	if err != nil {
		return agentdelegation.Child{}, fmt.Errorf("resolve sub Agent model identity id=%s: %w", sub.ID, err)
	}
	return buildChildDefinition(cfg, childDefinitionSpec{
		ParentKind: parent.Kind, Name: sub.ID, Description: sub.Description,
		Composition: assembly.SystemPrompt, Model: subChatModel, ModelIdentity: modelIdentity,
		Context:            projectContext,
		ModelContextWindow: resolvedModel.ContextWindowTokens,
		Tools:              assembly.Tools, Middlewares: assembly.Middlewares,
	})
}

type childDefinitionSpec struct {
	ParentKind         string
	Name               string
	Description        string
	Composition        prompts.SystemPromptComposition
	Context            agent.ContextSource
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
		Instructions: spec.Composition.Instruction(), Context: spec.Context, Tools: tools,
		Middlewares: identifyDenovaMiddlewares(spec.ParentKind+".child."+spec.Name, cfg, spec.Middlewares),
		ResultProcessor: agenttoolresult.Standard(agenttoolresult.Policy{
			MaxBytes: toolresult.LimitBytes(cfg), EagerMinTokens: config.DefaultToolResultEagerMinTokens,
			ContextWindowTokens: spec.ModelContextWindow,
		}),
		Cleanup: agentconversation.NewAgentCleanupManagerForModel(cfg, spec.ParentKind, spec.ModelContextWindow),
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
