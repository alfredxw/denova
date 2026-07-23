package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/adk/filesystem"
	filesystemmw "github.com/cloudwego/eino/adk/middlewares/filesystem"
	"github.com/cloudwego/eino/components/tool"

	"denova/config"
	"denova/internal/workspacechange"
)

// Tool factories are kept apart from Agent construction so adding a product
// tool surface does not make the model/middleware assembly module a catch-all.
func loreToolsFactory(cfg *config.Config, forceReadOnly bool) func(config.ResolvedAgentToolSettings) ([]tool.BaseTool, error) {
	return func(settings config.ResolvedAgentToolSettings) ([]tool.BaseTool, error) {
		if cfg == nil || (!settings.LoreRead && !settings.LoreWrite) {
			return nil, nil
		}
		allowWrite := !forceReadOnly && settings.LoreWrite
		return newLoreTools(cfg.Workspace, allowWrite)
	}
}

func ideToolsFactory(cfg *config.Config) func(config.ResolvedAgentToolSettings) ([]tool.BaseTool, error) {
	return func(_ config.ResolvedAgentToolSettings) ([]tool.BaseTool, error) {
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
		tools := append([]tool.BaseTool{}, loreTools...)
		tools = append(tools, imageTools...)
		return tools, nil
	}
}

func imageToolsFactory(cfg *config.Config) func(config.ResolvedAgentToolSettings) ([]tool.BaseTool, error) {
	return func(_ config.ResolvedAgentToolSettings) ([]tool.BaseTool, error) {
		if cfg == nil {
			return nil, nil
		}
		return newIllustrationTools(cfg)
	}
}

func interactiveStoryToolsFactory(cfg *config.Config, toolContexts ...InteractiveStoryToolContext) func(config.ResolvedAgentToolSettings) ([]tool.BaseTool, error) {
	return func(_ config.ResolvedAgentToolSettings) ([]tool.BaseTool, error) {
		var tools []tool.BaseTool
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

func interactiveDirectorToolsFactory(cfg *config.Config, toolContexts ...InteractiveStoryToolContext) func(config.ResolvedAgentToolSettings) ([]tool.BaseTool, error) {
	return func(settings config.ResolvedAgentToolSettings) ([]tool.BaseTool, error) {
		var tools []tool.BaseTool
		var storyToolContext InteractiveStoryToolContext
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

func configManagerToolsFactory(cfg *config.Config) func(config.ResolvedAgentToolSettings) ([]tool.BaseTool, error) {
	return func(settings config.ResolvedAgentToolSettings) ([]tool.BaseTool, error) {
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

var filesystemMiddlewareToolNames = []string{"ls", "read_file", "glob", "grep", "write_file", "edit_file", "execute"}

func newFilesystemMiddleware(ctx context.Context, backend filesystem.Backend, streamingShell filesystem.StreamingShell, settings config.ResolvedAgentToolSettings, workspaces ...string) (adk.ChatModelAgentMiddleware, error) {
	if backend == nil || (!settings.FileRead && !settings.FileWrite && !settings.ShellExecute) {
		return nil, nil
	}
	workspace := ""
	if len(workspaces) > 0 {
		workspace = strings.TrimSpace(workspaces[0])
	}
	readTool, err := newWorkspaceReadFileTool(backend, workspace)
	if err != nil {
		return nil, fmt.Errorf("创建 read_file 工具失败: %w", err)
	}
	readToolConfig := &filesystemmw.ToolConfig{CustomTool: readTool}
	writeToolConfig := &filesystemmw.ToolConfig{}
	editToolConfig := &filesystemmw.ToolConfig{}
	if workspace != "" && settings.FileWrite {
		changes, err := workspacechange.ForWorkspace(workspace)
		if err != nil {
			return nil, fmt.Errorf("创建 workspace change service 失败: %w", err)
		}
		writeTool, err := newWorkspaceWriteFileTool(changes)
		if err != nil {
			return nil, fmt.Errorf("创建 write_file 工具失败: %w", err)
		}
		editTool, err := newWorkspaceEditFileTool(changes)
		if err != nil {
			return nil, fmt.Errorf("创建 edit_file 工具失败: %w", err)
		}
		writeToolConfig.CustomTool = writeTool
		editToolConfig.CustomTool = editTool
	}
	mwConfig := &filesystemmw.MiddlewareConfig{
		Backend:             backend,
		LsToolConfig:        &filesystemmw.ToolConfig{},
		ReadFileToolConfig:  readToolConfig,
		GlobToolConfig:      &filesystemmw.ToolConfig{},
		GrepToolConfig:      &filesystemmw.ToolConfig{},
		WriteFileToolConfig: writeToolConfig,
		EditFileToolConfig:  editToolConfig,
	}
	if streamingShell != nil {
		mwConfig.StreamingShell = streamingShell
	}
	return filesystemmw.New(ctx, mwConfig)
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
