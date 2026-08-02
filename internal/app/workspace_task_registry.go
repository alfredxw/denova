package app

import (
	"context"
	apptask "denova/internal/app/task"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"denova/internal/concurrency"
)

// registerWorkspaceTaskLocked records a background task against the exact
// workspace resources it captured. The caller must hold a.mu. strictCurrent
// is used by UI-scoped agents; library automations may intentionally target a
// different registered workspace and therefore opt out of that equality
// check while retaining transition fencing.
func (a *App) registerWorkspaceTaskLocked(task *apptask.Task, workspace string, strictCurrent bool) error {
	workspace = strings.TrimSpace(workspace)
	if task == nil || workspace == "" {
		return ErrNoWorkspace
	}
	if strictCurrent && a.workspace != workspace {
		return fmt.Errorf("%w: expected=%q actual=%q", ErrWorkspaceChanged, workspace, a.workspace)
	}
	key := lifecycleWorkspaceKey(workspace)
	if _, transitioning := a.workspaceTransitionTargets[key]; a.workspaceTransition && transitioning {
		return ErrWorkspaceTransition
	}
	scope, err := a.workspaceScopeLocked(workspace)
	if err != nil {
		return err
	}
	if err := a.registerOwnedTaskLocked(task, workspace, scope); err != nil {
		if errors.Is(err, concurrency.ErrClosing) || errors.Is(err, concurrency.ErrClosed) {
			return ErrWorkspaceTransition
		}
		return err
	}
	return nil
}

// registerOwnedTaskLocked binds every App-owned Task to both its cancellation
// scope and the process-wide replay budget before publishing it in the common
// registry. The caller holds App.mu. Rollback is completed here because a
// failed apptask.NewDeferred registration does not return the Task to
// its caller, so no outer cleanup can safely recover these resources.
func (a *App) registerOwnedTaskLocked(task *apptask.Task, workspace string, scope *concurrency.Scope) error {
	if task == nil {
		return fmt.Errorf("cannot register a nil Task")
	}
	if scope == nil {
		return concurrency.ErrClosed
	}
	if _, exists := a.workspaceTasks[task]; exists {
		return fmt.Errorf("Task %q is already registered", task.ID())
	}
	lease, err := scope.Acquire()
	if err != nil {
		return err
	}
	replay, err := a.activeTaskReplay.Reserve(task)
	if err != nil {
		// Release while the Task is still private. No App registry entry or
		// cancellation callback has been published at this point.
		lease.Release()
		return err
	}
	if a.workspaceTasks == nil {
		a.workspaceTasks = make(map[*apptask.Task]string)
	}
	if a.workspaceTaskLeases == nil {
		a.workspaceTaskLeases = make(map[*apptask.Task]*concurrency.Lease)
	}
	if a.workspaceTaskStops == nil {
		a.workspaceTaskStops = make(map[*apptask.Task]func() bool)
	}
	if a.workspaceTaskReplayReservations == nil {
		a.workspaceTaskReplayReservations = make(map[*apptask.Task]*apptask.ReplayReservation)
	}
	a.workspaceTasks[task] = workspace
	a.workspaceTaskLeases[task] = lease
	a.workspaceTaskStops[task] = context.AfterFunc(scope.Context(), task.Abort)
	a.workspaceTaskReplayReservations[task] = replay
	return nil
}

func (a *App) unregisterWorkspaceTask(task *apptask.Task) {
	if a == nil || task == nil {
		return
	}
	a.mu.Lock()
	if _, registered := a.workspaceTasks[task]; !registered {
		a.mu.Unlock()
		return
	}
	lease := a.workspaceTaskLeases[task]
	stop := a.workspaceTaskStops[task]
	replay := a.workspaceTaskReplayReservations[task]
	delete(a.workspaceTasks, task)
	delete(a.workspaceTaskLeases, task)
	delete(a.workspaceTaskStops, task)
	delete(a.workspaceTaskReplayReservations, task)
	a.mu.Unlock()
	// Resource release intentionally happens after App.mu is dropped. Scope
	// closing and replay accounting have their own locks and never need to
	// participate in the App registry's critical section.
	if stop != nil {
		stop()
	}
	lease.Release()
	replay.Release()
	// Runtime admits a durable HostEffect immediately after the exact output
	// receipt, while the owning display Task may still be registered as active.
	// Wake reconciliation again at the common settlement boundary so Writing,
	// Game, Config, Lore, and Automation effects never depend on the periodic
	// scheduler after an earlier active-operation check deferred delivery.
	a.Automation().SignalReconciliation()
}

// beginWorkspaceTransition fences new current-workspace tasks and snapshots
// every admitted task before cancellation. The returned tasks are safe to
// await without holding App.mu.
func (a *App) beginWorkspaceTransition() ([]*apptask.Task, string, error) {
	tasks, _, workspace, err := a.beginWorkspaceTransitionTo()
	return tasks, workspace, err
}

// beginWorkspaceTransitionTo fences both the currently installed generation
// and every target generation before callers release App.mu. This prevents an
// inactive-workspace automation from racing buildRuntime for the same path.
func (a *App) beginWorkspaceTransitionTo(targets ...string) ([]*apptask.Task, []*concurrency.Scope, string, error) {
	if a == nil {
		return nil, nil, "", ErrNoWorkspace
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.workspaceTransition {
		return nil, nil, a.workspace, ErrWorkspaceTransition
	}
	if err := a.initializeLifecycleLocked(); err != nil {
		return nil, nil, a.workspace, err
	}
	a.workspaceTransition = true
	workspace := a.workspace
	a.workspaceTransitionTargets = make(map[string]struct{}, len(targets)+1)
	transitionWorkspaces := append([]string{workspace}, targets...)
	for _, candidate := range transitionWorkspaces {
		key := lifecycleWorkspaceKey(candidate)
		if key == "" {
			continue
		}
		a.workspaceTransitionTargets[key] = struct{}{}
		if _, err := a.workspaceScopeLocked(key); err != nil {
			a.workspaceTransition = false
			a.workspaceTransitionTargets = nil
			return nil, nil, workspace, err
		}
	}
	scopes := a.fenceWorkspaceScopesLocked(transitionWorkspaces...)
	unique := make(map[*apptask.Task]struct{})
	appendTask := func(task *apptask.Task) {
		if task != nil && !task.Finished() {
			unique[task] = struct{}{}
		}
	}
	for task, owner := range a.workspaceTasks {
		if owner == workspace {
			appendTask(task)
		}
	}
	// Include tasks created before the workspace registry existed so an
	// in-memory App upgraded during tests cannot escape the transition fence.
	appendTask(a.activeTask)
	if a.activeInteractiveRun != nil {
		appendTask(a.activeInteractiveRun.task)
	}
	tasks := make([]*apptask.Task, 0, len(unique))
	for task := range unique {
		tasks = append(tasks, task)
	}
	return tasks, scopes, workspace, nil
}

func (a *App) endWorkspaceTransition() {
	if a == nil {
		return
	}
	a.mu.Lock()
	a.workspaceTransition = false
	a.workspaceTransitionTargets = nil
	a.mu.Unlock()
}

func abortAndWaitTasks(ctx context.Context, tasks []*apptask.Task, workspace string) error {
	for _, task := range tasks {
		if task == nil || task.Finished() {
			continue
		}
		slog.InfoContext(ctx, fmt.Sprintf("[workspace-runtime] abort task before transition workspace=%q task_id=%s status=%s", workspace, task.ID(), task.Status()))
		task.Abort()
	}
	for _, task := range tasks {
		if task == nil || task.Finished() {
			continue
		}
		select {
		case <-task.Done():
		case <-ctx.Done():
			return fmt.Errorf("wait for workspace task %s: %w", task.ID(), ctx.Err())
		}
	}
	return nil
}
