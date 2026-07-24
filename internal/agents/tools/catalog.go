package tools

import (
	"context"
	"fmt"
	"strings"

	agent "github.com/alfredxw/denova/agent"
	agenttools "github.com/alfredxw/denova/agent/tools"

	"denova/config"
	novaskills "denova/internal/agents/skills"
	"denova/internal/workspacechange"
)

// Catalog is the single construction boundary for Denova's concrete tools.
// The agents package chooses a surface and owns execution policy; this package
// owns tool schemas, implementations, descriptors, and filesystem adapters.
type Catalog struct {
	cfg               *config.Config
	workspaceMetadata WorkspaceMetadataProvider
}

func NewCatalog(cfg *config.Config, workspaceMetadata WorkspaceMetadataProvider) *Catalog {
	return &Catalog{cfg: cfg, workspaceMetadata: workspaceMetadata}
}

type Factory func(config.ResolvedAgentToolSettings) ([]agent.BaseTool, error)

func (catalog *Catalog) Lore(forceReadOnly bool) Factory {
	return loreToolsFactory(catalog.cfg, forceReadOnly)
}

func (catalog *Catalog) IDE() Factory {
	return ideToolsFactory(catalog.cfg)
}

func (catalog *Catalog) Image() Factory {
	return imageToolsFactory(catalog.cfg)
}

func (catalog *Catalog) InteractiveStory(toolContext InteractiveContext) Factory {
	return interactiveStoryToolsFactory(catalog.cfg, toolContext)
}

func (catalog *Catalog) InteractiveDirector(toolContext InteractiveContext) Factory {
	return interactiveDirectorToolsFactory(catalog.cfg, toolContext)
}

func (catalog *Catalog) ConfigManager() Factory {
	return configManagerToolsFactory(catalog.cfg)
}

func (catalog *Catalog) Filesystem(settings config.ResolvedAgentToolSettings) ([]agent.BaseTool, error) {
	workspace := ""
	if catalog != nil && catalog.cfg != nil {
		workspace = catalog.cfg.Workspace
	}
	var metadata WorkspaceMetadataProvider
	if catalog != nil {
		metadata = catalog.workspaceMetadata
	}
	return filesystemToolsFactory(workspace, metadata)(settings)
}

func (catalog *Catalog) WebAccess() ([]agent.BaseTool, error) {
	return newWebAccessTools(catalog.cfg)
}

func (catalog *Catalog) WebAccessEnabled(agentKind string, settings config.ResolvedAgentToolSettings) bool {
	return stableWebSearchSchemaAllowed(agentKind)(settings)
}

func (catalog *Catalog) Skill(ctx context.Context, backend *novaskills.Backend, maxBytes int) (agent.BaseTool, error) {
	return newSkillTool(ctx, backend, maxBytes)
}

func (catalog *Catalog) WriteTodos() (agent.BaseTool, error) {
	return newWriteTodosTool()
}

func (catalog *Catalog) Task(ctx context.Context, subAgents []agent.Runnable) (agent.BaseTool, error) {
	return newTaskTool(ctx, subAgents)
}

// Tool factories are kept apart from Agent construction so adding a product
// tool surface does not make the model/middleware assembly module a catch-all.
func loreToolsFactory(cfg *config.Config, forceReadOnly bool) func(config.ResolvedAgentToolSettings) ([]agent.BaseTool, error) {
	return func(settings config.ResolvedAgentToolSettings) ([]agent.BaseTool, error) {
		if cfg == nil || (!settings.LoreRead && !settings.LoreWrite) {
			return nil, nil
		}
		allowWrite := !forceReadOnly && settings.LoreWrite
		return newLoreTools(cfg.Workspace, allowWrite)
	}
}

func ideToolsFactory(cfg *config.Config) func(config.ResolvedAgentToolSettings) ([]agent.BaseTool, error) {
	return func(_ config.ResolvedAgentToolSettings) ([]agent.BaseTool, error) {
		if cfg == nil {
			return nil, nil
		}
		loreTools, err := newLoreTools(cfg.Workspace, true)
		if err != nil {
			return nil, err
		}
		imageTools, err := newIllustrationTools(cfg)
		if err != nil {
			return nil, err
		}
		tools := append([]agent.BaseTool{}, loreTools...)
		tools = append(tools, imageTools...)
		return tools, nil
	}
}

func imageToolsFactory(cfg *config.Config) func(config.ResolvedAgentToolSettings) ([]agent.BaseTool, error) {
	return func(_ config.ResolvedAgentToolSettings) ([]agent.BaseTool, error) {
		if cfg == nil {
			return nil, nil
		}
		return newIllustrationTools(cfg)
	}
}

func interactiveStoryToolsFactory(cfg *config.Config, toolContexts ...InteractiveContext) func(config.ResolvedAgentToolSettings) ([]agent.BaseTool, error) {
	return func(_ config.ResolvedAgentToolSettings) ([]agent.BaseTool, error) {
		var tools []agent.BaseTool
		if cfg != nil {
			loreTools, err := newLoreTools(cfg.Workspace, false)
			if err != nil {
				return nil, err
			}
			tools = append(tools, loreTools...)
		}
		if len(toolContexts) > 0 {
			historyTools, err := newInteractiveHistoryTools(toolContexts[0])
			if err != nil {
				return nil, err
			}
			tools = append(tools, historyTools...)
			stateSchemaTools, err := newInteractiveOpeningStateSchemaTools(toolContexts[0])
			if err != nil {
				return nil, err
			}
			tools = append(tools, stateSchemaTools...)
			turnTools, err := newInteractiveTurnTools(toolContexts[0])
			if err != nil {
				return nil, err
			}
			tools = append(tools, turnTools...)
		}
		return tools, nil
	}
}

func interactiveDirectorToolsFactory(cfg *config.Config, toolContexts ...InteractiveContext) func(config.ResolvedAgentToolSettings) ([]agent.BaseTool, error) {
	return func(settings config.ResolvedAgentToolSettings) ([]agent.BaseTool, error) {
		var tools []agent.BaseTool
		var storyToolContext InteractiveContext
		if len(toolContexts) > 0 {
			storyToolContext = toolContexts[0]
		}
		if cfg != nil && settings.LoreRead {
			var options []loreToolsOptions
			switch strings.TrimSpace(storyToolContext.MaintenanceTask) {
			case "director_plan_update", "opening_plan":
				policy := defaultLoreReadPolicy()
				policy.OnRead = storyToolContext.OnLoreItemsRead
				options = append(options, loreToolsOptions{ReadPolicy: policy})
			}
			loreTools, err := newLoreTools(cfg.Workspace, false, options...)
			if err != nil {
				return nil, err
			}
			tools = append(tools, loreTools...)
		}
		if len(toolContexts) == 0 {
			return tools, nil
		}
		ctx := storyToolContext
		switch strings.TrimSpace(ctx.MaintenanceTask) {
		case "director_plan_update", "opening_plan":
			historyTools, err := newInteractiveHistoryTools(ctx)
			if err != nil {
				return nil, err
			}
			eventTools, err := newInteractiveEventTools(ctx)
			if err != nil {
				return nil, err
			}
			planTools, err := newInteractiveDirectorPlanTools(ctx)
			tools = append(tools, historyTools...)
			tools = append(tools, eventTools...)
			return append(tools, planTools...), err
		default:
			return tools, nil
		}
	}
}

func configManagerToolsFactory(cfg *config.Config) func(config.ResolvedAgentToolSettings) ([]agent.BaseTool, error) {
	return func(settings config.ResolvedAgentToolSettings) ([]agent.BaseTool, error) {
		if cfg == nil || !configManagerFactoryAllowed(settings) {
			return nil, nil
		}
		return newConfigManagerTools(cfg, settings)
	}
}

func configManagerFactoryAllowed(settings config.ResolvedAgentToolSettings) bool {
	return settings.LoreRead ||
		settings.LoreWrite ||
		settings.Todo ||
		settings.Skills ||
		settings.AgentConfigRead ||
		settings.AgentConfigWrite
}

// filesystemToolsFactory assembles native workspace tools as ordinary Agent
// tools, keeping the concrete surface visible to construction-time validation.
func filesystemToolsFactory(workspace string, metadata WorkspaceMetadataProvider) func(config.ResolvedAgentToolSettings) ([]agent.BaseTool, error) {
	return func(settings config.ResolvedAgentToolSettings) ([]agent.BaseTool, error) {
		if !settings.FileRead && !settings.FileWrite && !settings.ShellExecute {
			return nil, nil
		}
		backend, err := agenttools.OpenWorkspace(workspace)
		if err != nil {
			return nil, fmt.Errorf("create workspace filesystem backend: %w", err)
		}

		definitionOptions := []agenttools.DefinitionOption{agenttools.WithCapability(config.AgentToolFileRead)}
		listDefinition, err := agenttools.List(backend, definitionOptions...)
		if err != nil {
			return nil, fmt.Errorf("create ls tool: %w", err)
		}
		readDefinition, err := agenttools.ReadFile(backend, definitionOptions...)
		if err != nil {
			return nil, fmt.Errorf("create read_file tool: %w", err)
		}
		globDefinition, err := agenttools.Glob(backend, definitionOptions...)
		if err != nil {
			return nil, fmt.Errorf("create glob tool: %w", err)
		}
		grepDefinition, err := agenttools.Grep(backend, definitionOptions...)
		if err != nil {
			return nil, fmt.Errorf("create grep tool: %w", err)
		}

		var changes workspaceChangeService = disabledWorkspaceChangeService{workspace: backend.Root()}
		if settings.FileWrite {
			changes, err = workspacechange.ForWorkspace(backend.Root())
			if err != nil {
				return nil, fmt.Errorf("create workspace change service: %w", err)
			}
		}
		writeTool, err := newWorkspaceWriteFileTool(changes, metadata)
		if err != nil {
			return nil, fmt.Errorf("create write_file tool: %w", err)
		}
		editTool, err := newWorkspaceEditFileTool(changes, metadata)
		if err != nil {
			return nil, fmt.Errorf("create edit_file tool: %w", err)
		}

		var shell *agentStreamingShell
		if settings.ShellExecute {
			shell, err = newAgentStreamingShell(backend.Root())
			if err != nil {
				return nil, fmt.Errorf("create execute shell: %w", err)
			}
		}
		executeDefinition, err := agenttools.Execute(shell, agenttools.WithCapability(config.AgentToolShellExecute))
		if err != nil {
			return nil, fmt.Errorf("create execute tool: %w", err)
		}
		writeTool, err = defineTool(writeTool, workspaceWriteDescriptor(agenttools.SourceWrite, config.AgentToolFileWrite, agenttools.RecoveryReconcilable))
		if err != nil {
			return nil, err
		}
		editTool, err = defineTool(editTool, workspaceWriteDescriptor(agenttools.SourceWrite, config.AgentToolFileWrite, agenttools.RecoveryReconcilable))
		if err != nil {
			return nil, err
		}
		definitions := []agenttools.Definition{listDefinition, readDefinition, globDefinition, grepDefinition}
		tools := make([]agent.BaseTool, 0, len(definitions)+3)
		for _, definition := range definitions {
			tool, bindErr := bindToolDefinition(definition)
			if bindErr != nil {
				return nil, bindErr
			}
			tools = append(tools, tool)
		}
		executeTool, err := bindToolDefinition(executeDefinition)
		if err != nil {
			return nil, err
		}
		return append(tools, writeTool, editTool, executeTool), nil
	}
}

func stableWebSearchSchemaAllowed(agentKind string) func(config.ResolvedAgentToolSettings) bool {
	return func(settings config.ResolvedAgentToolSettings) bool {
		if settings.WebSearch {
			return true
		}
		switch agentKind {
		case config.AgentKindIDE, config.AgentKindInteractiveStory, config.AgentKindConfigManager, config.AgentKindAutomation:
			return true
		default:
			return false
		}
	}
}
