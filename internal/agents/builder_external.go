package agents

import (
	"context"
	"fmt"

	"denova/config"
	"denova/internal/agents/agentprofile"
	"denova/internal/agents/conversationconfig"
	"denova/internal/agents/lifecycle"
	"denova/internal/agents/prompts"
	"denova/internal/agents/toolruntime"
	producttools "denova/internal/agents/tools"
	"denova/internal/book"
	agent "github.com/alfredxw/denova/agent"
	publictools "github.com/alfredxw/denova/agent/tools"
)

// ExternalAssembly contains reusable product capabilities only. The caller
// owns foreground-entry admission, engine selection, context materialization,
// and durable tool dispatch. No Native Definition or model is constructed.
type ExternalAssembly struct {
	Composition  prompts.SystemPromptComposition
	Tools        []agent.ToolDefinition
	Context      agent.ContextSource
	ToolSettings config.ResolvedAgentToolSettings
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
	if err := composition.ValidateForAgent(kind); err != nil {
		return ExternalAssembly{}, err
	}
	// External engines receive the product's role defaults. Saved Native tool
	// permissions and delegation policy are dormant, never silently translated.
	settings := ExternalConversationToolSettings(kind)
	assembly, err := buildAgentTools(ctx, cfg, agentToolsSpec{
		Kind: kind, SystemPrompt: composition, Settings: settings, EnableSkills: true,
		ExtraToolsFactory: factory, ReadAdapters: host.ReadAdapters,
	})
	if err != nil {
		return ExternalAssembly{}, err
	}
	// Root tools are Native session controls. Ask reuses its public schema only;
	// the external Host resolves it from the product journal instead of Run state.
	asks, err := publictools.Ask().PrepareTools(ctx, agent.ToolRequest{})
	if err != nil {
		return ExternalAssembly{}, err
	}
	definitions := append(assembly.Tools, asks...)
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
	source, err := agent.CombineContextSources(project, agentprofile.ContextSource(cfg, kind))
	if err != nil {
		return ExternalAssembly{}, fmt.Errorf("compose external product context: %w", err)
	}
	return ExternalAssembly{Composition: assembly.SystemPrompt, Tools: definitions, Context: source, ToolSettings: settings}, nil
}

// ExternalConversationToolSettings is shared by execution and the read-only
// Agents manifest. Native permissions and session-dependent tools stay dormant.
func ExternalConversationToolSettings(kind string) config.ResolvedAgentToolSettings {
	settings := config.ResolveAgentTools(&config.Config{AgentTools: config.DefaultAgentToolSettings()}, kind)
	for _, capability := range []string{config.AgentToolTodo, config.AgentToolDelegation, config.AgentToolScript} {
		settings[capability] = false
	}
	return settings
}
