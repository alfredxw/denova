package agents

import (
	"context"
	"os"
	"strings"

	agent "github.com/alfredxw/denova/agent"

	"denova/config"
	producttools "denova/internal/agents/tools"
	"denova/internal/runtimetools"
	"denova/internal/shellenv"
	"denova/internal/workspacechange"
)

const submitDirectorPlanUpdateToolName = producttools.SubmitDirectorPlanUpdateToolName

type submitDirectorPlanUpdateInput = producttools.SubmitDirectorPlanUpdateInput

// newToolCatalog is the only bridge from Agent orchestration into concrete
// tool construction. Runtime metadata is projected into a narrow callback so
// the tools package never imports the agents package.
func newToolCatalog(cfg *config.Config) *producttools.Catalog {
	return newToolCatalogWithContext(context.Background(), cfg)
}

func newToolCatalogWithContext(ctx context.Context, cfg *config.Config) *producttools.Catalog {
	if ctx == nil {
		ctx = context.Background()
	}
	executablePath, _ := os.Executable()
	discovered := runtimetools.DiscoverForExecutable(executablePath)
	return producttools.NewCatalog(cfg, agentWorkspaceChangeMetadata, producttools.RuntimeExecutables{
		Ripgrep: discovered.Ripgrep,
		Bash:    discovered.Bash,
		Pwsh:    discovered.Pwsh,
		ShellRuntime: func() (producttools.ShellRuntime, error) {
			environment := os.Environ()
			bashOverride := ""
			mode := config.ShellEnvironmentProcess
			shell := ""
			if cfg != nil {
				mode = cfg.ShellEnvironmentMode
				if mode == "" {
					// Hand-built Config values in focused tests predate this
					// setting. Loaded application configs always carry Auto.
					mode = config.ShellEnvironmentProcess
				}
				shell = cfg.ShellEnvironmentShell
				bashOverride = cfg.AgentBashPath
			}
			snapshot, err := shellenv.Resolve(ctx, shellenv.Options{Mode: mode, Shell: shell})
			if err != nil {
				return producttools.ShellRuntime{}, err
			}
			environment = snapshot.Environment
			resolved := runtimetools.DiscoverForExecutableWithEnvironment(executablePath, environment, bashOverride)
			return producttools.ShellRuntime{
				Bash: resolved.Bash, Pwsh: resolved.Pwsh,
				Environment: append([]string(nil), environment...),
			}, nil
		},
	})
}

func projectInteractiveToolContext(contexts ...InteractiveStoryToolContext) producttools.InteractiveContext {
	if len(contexts) == 0 {
		return producttools.InteractiveContext{}
	}
	source := contexts[0]
	return producttools.InteractiveContext{
		Store:                     source.Store,
		StoryID:                   source.StoryID,
		BranchID:                  source.BranchID,
		TurnID:                    source.TurnID,
		MaintenanceTask:           source.MaintenanceTask,
		OnLoreItemsRead:           source.OnLoreItemsRead,
		SubmitStateSchemaBatch:    source.SubmitStateSchemaBatch,
		SubmitDirectorPlanUpdate:  source.SubmitDirectorPlanUpdate,
		RequestDirectorCompletion: requestInteractiveDirectorPlanCompletion,
		RequestTurnCompletion:     requestInteractiveTurnCompletion,
		PrepareTurn:               source.PrepareTurn,
		SubmitTurnResult:          source.SubmitTurnResult,
	}
}

func agentWorkspaceChangeMetadata(ctx context.Context) workspacechange.ChangeMetadata {
	providerCallID := strings.TrimSpace(agent.ToolCallID(ctx))
	executionID := agent.ToolExecutionID(ctx, providerCallID)
	runID := ""
	sessionID := ""
	reviewThreadID := ""
	if observer := RunObserverFromContext(ctx); observer != nil {
		runID = strings.TrimSpace(observer.RunID())
		sessionID = strings.TrimSpace(observer.SessionID())
		reviewThreadID = strings.TrimSpace(observer.ReviewThreadID())
	}
	groupID := runID
	if groupID == "" {
		groupID = executionID
	}
	return workspacechange.ChangeMetadata{
		Origin:         workspacechange.OriginAgent,
		ChangeGroupID:  groupID,
		RunID:          runID,
		SessionID:      sessionID,
		ReviewThreadID: reviewThreadID,
		ToolCallID:     executionID,
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
