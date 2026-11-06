package agents

import (
	"context"
	"fmt"

	"denova/config"
	"denova/internal/agents/agentprofile"
	"denova/internal/agents/conversationconfig"
	agentinteractive "denova/internal/agents/interactive"
	"denova/internal/agents/lifecycle"
	"denova/internal/agents/prompts"
	"denova/internal/agents/toolruntime"
	producttools "denova/internal/agents/tools"
	"denova/internal/book"

	agentcontext "github.com/alfredxw/denova/agent/context"
	agenttool "github.com/alfredxw/denova/agent/tool"
	publictools "github.com/alfredxw/denova/agent/tool/builtin"
)

// ExternalAssembly contains reusable product capabilities only. The caller
// owns foreground-entry admission, engine selection, context materialization,
// and durable tool dispatch. No Native Definition or model is constructed.
type ExternalAssembly struct {
	Composition  prompts.SystemPromptComposition
	Tools        []agenttool.ToolDefinition
	Context      agentcontext.ContextSource
	ToolSettings config.ResolvedAgentToolSettings
	pluginTools  agenttool.Toolset
}

// PrepareTools binds plugin process lifetimes to the execution context, rather
// than the short-lived HTTP context used to assemble admission capabilities.
func (assembly ExternalAssembly) PrepareTools(ctx context.Context) ([]agenttool.ToolDefinition, error) {
	definitions := append([]agenttool.ToolDefinition(nil), assembly.Tools...)
	if assembly.pluginTools != nil {
		plugins, err := assembly.pluginTools.PrepareTools(ctx, agenttool.ToolRequest{})
		if err != nil {
			return nil, fmt.Errorf("prepare external plugin tools: %w", err)
		}
		definitions = append(definitions, plugins...)
	}
	return definitions, producttools.Validate(ctx, definitions)
}

func BuildExternalConversationAssembly(ctx context.Context, cfg *config.Config, state *book.State, teller prompts.IDEStoryTeller, kind string, host AgentHostCapabilities) (ExternalAssembly, error) {
	if cfg == nil || !host.Interactive {
		return ExternalAssembly{}, conversationconfig.ErrRuntimeCapabilityUnsupported
	}
	var composition prompts.SystemPromptComposition
	var factory producttools.Factory
	var err error
	catalog := toolruntime.NewCatalogWithContext(ctx, cfg)
	switch kind {
	case config.AgentKindIDE:
		composition, err = prompts.ComposeInstruction(cfg, state, teller)
		factory = catalog.IDE()
	case config.AgentKindGeneral:
		composition, err = prompts.ComposeGeneralInstruction(cfg)
		factory = catalog.Configuration()
	default:
		return ExternalAssembly{}, conversationconfig.ErrRuntimeCapabilityUnsupported
	}
	if err != nil {
		return ExternalAssembly{}, err
	}
	return buildExternalAssembly(ctx, cfg, state, kind, composition, factory, host)
}

// BuildExternalGameAssembly exposes Game's product contract to an external
// engine. It constructs no Native Definition, model, session or control tools.
func BuildExternalGameAssembly(ctx context.Context, cfg *config.Config, state *book.State, teller prompts.InteractiveStorySystemInstructionInput, host AgentHostCapabilities, tools agentinteractive.InteractiveStoryToolContext) (ExternalAssembly, error) {
	composition, err := prompts.ComposeInteractiveStoryInstruction(cfg, state, teller)
	if err != nil {
		return ExternalAssembly{}, err
	}
	return buildExternalAssembly(ctx, cfg, state, config.AgentKindInteractiveStory, composition,
		toolruntime.NewCatalogWithContext(ctx, cfg).InteractiveStory(toolruntime.ProjectInteractiveContext(tools)), host)
}

func buildExternalAssembly(ctx context.Context, cfg *config.Config, state *book.State, kind string, composition prompts.SystemPromptComposition, factory producttools.Factory, host AgentHostCapabilities) (ExternalAssembly, error) {
	if err := composition.ValidateForAgent(kind); err != nil {
		return ExternalAssembly{}, err
	}
	// External engines receive the product's role defaults. Saved Native tool
	// permissions and delegation policy are dormant, never silently translated.
	settings := ExternalConversationToolSettings(kind)
	assembly, err := buildAgentTools(ctx, cfg, agentToolsSpec{
		Kind: kind, SystemPrompt: composition, Settings: settings, EnableSkills: true,
		ExtraToolsFactory: factory, ReadAdapters: host.ReadAdapters, ExtraTools: host.RootTools,
	})
	if err != nil {
		return ExternalAssembly{}, err
	}
	// Root tools are application-owned capabilities. Ask reuses its schema only;
	// the external Host resolves it from the product journal instead of Run state.
	var asks []agenttool.ToolDefinition
	if kind != config.AgentKindInteractiveStory {
		asks, err = publictools.Ask().PrepareTools(ctx, agenttool.ToolRequest{})
		if err != nil {
			return ExternalAssembly{}, err
		}
	}
	definitions := make([]agenttool.ToolDefinition, 0, len(assembly.Tools)+len(asks))
	for _, definition := range assembly.Tools {
		info, err := definition.Tool.Info(ctx)
		if err != nil {
			return ExternalAssembly{}, err
		}
		// Native discovery replaces duplicate filesystem search tools. Keep
		// product read because it also resolves virtual URIs and durable artifacts.
		if info.Name != "glob" && info.Name != "grep" {
			definitions = append(definitions, definition)
		}
	}
	definitions = append(definitions, asks...)
	definitions, err = agentprofile.ApplyToolGuidance(ctx, cfg, kind, definitions)
	if err != nil {
		return ExternalAssembly{}, err
	}
	if err := producttools.Validate(ctx, definitions); err != nil {
		return ExternalAssembly{}, err
	}
	project, err := lifecycle.NewProjectInstructionsContextSource(cfg, kind, state)
	if err != nil {
		return ExternalAssembly{}, err
	}
	source, err := agentcontext.CombineContextSources(project, agentprofile.ContextSource(cfg, kind))
	if err != nil {
		return ExternalAssembly{}, fmt.Errorf("compose external product context: %w", err)
	}
	return ExternalAssembly{Composition: assembly.SystemPrompt, Tools: definitions, Context: source, ToolSettings: settings, pluginTools: host.PluginTools}, nil
}

// ExternalConversationToolSettings is shared by execution and the read-only
// Agents manifest. Native permissions and session-dependent tools stay dormant.
func ExternalConversationToolSettings(kind string) config.ResolvedAgentToolSettings {
	settings := config.ResolveAgentTools(&config.Config{AgentTools: config.DefaultAgentToolSettings()}, kind)
	// External engines own their plan tools independently of the Native Todo default.
	settings[config.AgentToolTodo] = true
	for _, capability := range []string{config.AgentToolDelegation, config.AgentToolScript, config.AgentToolShell, config.AgentToolWebSearch, config.AgentToolWebFetch, config.AgentToolBrowser} {
		settings[capability] = false
	}
	return settings
}
