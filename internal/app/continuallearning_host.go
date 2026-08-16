package app

import (
	"context"
	"log/slog"

	"denova/internal/agents/trajectory"
	appagentruntime "denova/internal/app/agentruntime"
	continuallearningapp "denova/internal/app/continuallearning"
)

type continualLearningHost struct{ app *App }

func (host continualLearningHost) Runtime() continuallearningapp.Runtime {
	if host.app == nil {
		return continuallearningapp.Runtime{}
	}
	host.app.mu.RLock()
	defer host.app.mu.RUnlock()
	runtime := continuallearningapp.Runtime{Execution: host.app.executionRuntime}
	if host.app.cfg != nil {
		runtime.Config = *host.app.cfg
	}
	return runtime
}

func (host continualLearningHost) AcquireRootOperation(ctx context.Context) (continuallearningapp.Operation, error) {
	if host.app == nil {
		return nil, appagentruntime.ErrNoWorkspace
	}
	return host.app.acquireRootOperation(ctx)
}

func (host continualLearningHost) TrajectorySources(ctx context.Context) ([]trajectory.Source, error) {
	if host.app == nil {
		return nil, appagentruntime.ErrNoWorkspace
	}
	sources, issues, err := host.app.globalTrajectorySources(ctx)
	for _, issue := range issues {
		slog.WarnContext(ctx, "[trajectory] skip unavailable Project source", "project_id", issue.ProjectID, "error", issue.Message)
	}
	return sources, err
}
