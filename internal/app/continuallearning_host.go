package app

import (
	"context"
	"fmt"
	"strings"

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
	if host.app == nil || host.app.projectRegistry == nil {
		return nil, fmt.Errorf("Project registry is unavailable")
	}
	records, err := host.app.projectRegistry.List(true)
	if err != nil {
		return nil, err
	}
	sources := make([]trajectory.Source, 0, len(records))
	for _, record := range records {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		// Trajectory discovery is a read path. Project opening/migration owns
		// State creation; learning must not create or migrate dormant Projects as
		// a side effect of listing evidence.
		layout, layoutErr := host.app.projectRegistry.Layout(record)
		if layoutErr != nil {
			return nil, layoutErr
		}
		name := strings.TrimSpace(record.Name)
		if name == "" {
			name = record.ID
		}
		sources = append(sources, trajectory.Source{
			ProjectID: record.ID, Name: name, Workspace: layout.ContentRoot, StateRoot: layout.StateRoot,
		})
	}
	return sources, nil
}
