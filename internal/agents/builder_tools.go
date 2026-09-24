package agents

import (
	"context"

	"denova/config"
	"denova/internal/agents/prompts"
	"denova/internal/agents/scripttools"
	"denova/internal/agents/skillassembly"
	"denova/internal/agents/toolruntime"
	producttools "denova/internal/agents/tools"
	agent "github.com/alfredxw/denova/agent"
)

// agentToolsSpec assembles product tools and Skills without a model, permission
// policy or Agent loop. Callers supply the applicable capability set explicitly.
type agentToolsSpec struct {
	Kind                string
	SystemPrompt        prompts.SystemPromptComposition
	Settings            config.ResolvedAgentToolSettings
	EnableSkills        bool
	ExtraTools          []agent.ToolDefinition
	ReadAdapters        []producttools.ReadAdapterBinding
	ReadAdaptersFactory producttools.ReadAdapterFactory
	ExtraToolsFactory   func(config.ResolvedAgentToolSettings) ([]agent.ToolDefinition, error)
}

type agentToolsAssembly struct {
	SystemPrompt prompts.SystemPromptComposition
	Tools        []agent.ToolDefinition
}

func buildAgentTools(ctx context.Context, cfg *config.Config, spec agentToolsSpec) (agentToolsAssembly, error) {
	catalog := toolruntime.NewCatalogWithContext(ctx, cfg)
	settings := spec.Settings
	skills, err := skillassembly.Build(ctx, cfg, spec.Kind, spec.EnableSkills, settings, spec.SystemPrompt)
	if err != nil {
		return agentToolsAssembly{}, err
	}
	tools := append([]agent.ToolDefinition(nil), spec.ExtraTools...)
	readAdapters := append(skills.ReadAdapters, spec.ReadAdapters...)
	if spec.ReadAdaptersFactory != nil {
		extra, err := spec.ReadAdaptersFactory(settings)
		if err != nil {
			return agentToolsAssembly{}, err
		}
		readAdapters = append(readAdapters, extra...)
	}
	workspaceTools, err := catalog.Workspace(settings, readAdapters...)
	if err != nil {
		return agentToolsAssembly{}, err
	}
	tools = append(tools, workspaceTools...)
	tools = append(tools, skills.Tools...)
	if spec.ExtraToolsFactory != nil {
		extra, err := spec.ExtraToolsFactory(settings)
		if err != nil {
			return agentToolsAssembly{}, err
		}
		tools = append(tools, extra...)
	}
	webTools, err := catalog.WebAccess(settings)
	if err != nil {
		return agentToolsAssembly{}, err
	}
	tools = append(tools, webTools...)
	browserTools, err := catalog.Browser(ctx, settings)
	if err != nil {
		return agentToolsAssembly{}, err
	}
	tools = append(tools, browserTools...)
	if settings.Allows(config.AgentToolScript) {
		definition, err := scripttools.Immediate(cfg)
		if err != nil {
			return agentToolsAssembly{}, err
		}
		tools = append(tools, definition)
	}
	if err := producttools.Validate(ctx, tools); err != nil {
		return agentToolsAssembly{}, err
	}
	return agentToolsAssembly{SystemPrompt: skills.SystemPrompt, Tools: tools}, nil
}
