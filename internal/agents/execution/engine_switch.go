package execution

import (
	"context"

	agentrun "denova/internal/agents/run"
	agent "github.com/alfredxw/denova/agent"
)

// RequireIdleForEngineSwitch includes detached children and unfinished Goals.
// The caller excludes new product admissions until the selection is committed.
func (runtime *Runtime) RequireIdleForEngineSwitch(ctx context.Context, options agentrun.Options) error {
	if runtime == nil || runtime.public == nil {
		return ErrRuntimeProjectionUnavailable
	}
	root, _, err := runtime.public.openSession(ctx, options)
	if err != nil {
		return err
	}
	family, err := runtime.public.taskSessions(ctx, root)
	if err != nil {
		return err
	}
	for _, current := range family {
		snapshot, err := current.Snapshot(ctx)
		if err != nil {
			return err
		}
		if snapshot.ActiveRunID != "" || len(snapshot.QueuedRuns) > 0 || len(snapshot.OpenTools) > 0 || len(snapshot.PendingInteractions) > 0 {
			return agent.ErrSessionBusy
		}
		goal, found, err := current.Goal(ctx)
		if err != nil {
			return err
		}
		if found && goal.Status != agent.GoalCompleted && goal.Status != agent.GoalCleared {
			return agent.ErrSessionBusy
		}
	}
	return nil
}
