package app

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"denova/config"
	"denova/internal/buildinfo"
	"denova/internal/update"
)

// Update scheduling belongs to the App, never to a browser or Agent runtime.
// Both workers share root cancellation and are joined by App.Close.
func (a *App) startSoftwareUpdates(ctx context.Context) {
	if buildinfo.Distribution == "desktop" {
		return
	}
	for _, work := range []func(context.Context){
		func(ctx context.Context) { a.updates.RunAutomatic(ctx, a.updatePolicy) },
		a.waitForUpdateRestart,
	} {
		ctx, lease, err := a.rootScope.AcquireContext(ctx)
		if err != nil {
			slog.Error("update_worker_start_failed", "error", err)
			return
		}
		go func() {
			defer lease.Release()
			defer func() {
				if v := recover(); v != nil {
					slog.Error("update_worker_panic", "error", v)
				}
			}()
			work(ctx)
		}()
	}
}

func (a *App) updatePolicy() (update.Policy, error) {
	a.mu.RLock()
	dataDir := a.cfg.DataDir()
	a.mu.RUnlock()
	// Installation policy does not change when the foreground Book changes.
	layers, err := config.LoadLayeredWithStartupConfigAt(dataDir, "", "")
	if err != nil {
		return update.Policy{}, err
	}
	s := layers.Effective
	return update.Policy{Check: s.UpdateCheckEnabled != nil && *s.UpdateCheckEnabled, Download: s.UpdateDownloadEnabled != nil && *s.UpdateDownloadEnabled}, nil
}

func (a *App) UpdateStatus() (update.Status, error) {
	status, err := a.updateService().Status()
	if err != nil || a == nil {
		return status, err
	}
	a.mu.RLock()
	defer a.mu.RUnlock()
	status.RestartWaiting = a.updateWaitID != ""
	if a.updateRestartError != "" && a.updateRestartErrorID == status.ID {
		status.OperationError = a.updateRestartError
	}
	status.Busy, err = a.updateWorkActiveLocked(context.Background())
	return status, err
}

// updateWorkActiveLocked observes admitted operations, including image jobs,
// all product/runtime Tasks, and Native child runs that outlive their parent.
// App.mu serializes operation and Task admission with the final restart fence.
func (a *App) updateWorkActiveLocked(ctx context.Context) (bool, error) {
	if a.workspaceTransition || len(a.projectTransitions) != 0 {
		return true, nil
	}
	for _, scope := range a.workspaceScopes {
		if scope.ActiveLeases() != 0 {
			return true, nil
		}
	}
	for _, scope := range a.projectScopes {
		if scope.ActiveLeases() != 0 {
			return true, nil
		}
	}
	for task := range a.workspaceTasks {
		if !task.Finished() {
			return true, nil
		}
	}
	for task := range a.projectTasks {
		if !task.Finished() {
			return true, nil
		}
	}
	active, err := a.executionRuntime.ActiveSessions(ctx)
	return active != 0, err
}

// ScheduleUpdateRestart pins a user's restart request to one staged transaction.
// Waiting is process-local intent; restarting Denova normally cancels the wait.
func (a *App) ScheduleUpdateRestart(ctx context.Context, when string) (update.ApplyResult, error) {
	if a == nil {
		return a.updateService().Apply(ctx, nil, 0, "")
	}
	a.updateRestartMu.Lock()
	defer a.updateRestartMu.Unlock()
	status, err := a.updateService().Status()
	if err != nil {
		return update.ApplyResult{}, err
	}
	result := update.ApplyResult{ID: status.ID, Version: status.Version, LogPath: status.LogPath}
	switch when {
	case "cancel":
		a.mu.Lock()
		if a.closed || a.updateRestarting {
			a.mu.Unlock()
			return result, update.ErrUpdateBusy
		}
		a.updateWaitID, a.updateRestartError = "", ""
		a.mu.Unlock()
		result.Status = "canceled"
		slog.Info("update_restart_wait_canceled", "id", status.ID)
		return result, nil
	case "when_idle", "now":
	default:
		return result, fmt.Errorf("unsupported update restart mode %q", when)
	}
	if status.Phase != update.PhaseStaged || status.Operation != "" {
		return result, update.ErrUpdateBusy
	}
	if when == "when_idle" {
		a.mu.Lock()
		if a.closed || a.updateRestarting {
			a.mu.Unlock()
			return result, update.ErrUpdateBusy
		}
		a.updateWaitID, a.updateRestartError = status.ID, ""
		a.mu.Unlock()
		result.Status = "waiting_for_idle"
		slog.Info("update_restart_wait_started", "id", status.ID)
		return result, nil
	}
	return a.applyUpdateNow(ctx, status.ID)
}

// Called with updateRestartMu held. The admission fence is reversible until
// the updater has acknowledged the handoff; a failure leaves the app usable.
func (a *App) applyUpdateNow(ctx context.Context, id string) (update.ApplyResult, error) {
	a.mu.Lock()
	if a.closed {
		a.mu.Unlock()
		return update.ApplyResult{}, update.ErrUpdateBusy
	}
	a.updateRestarting, a.updateWaitID, a.updateRestartError = true, "", ""
	port := a.cfg.RuntimeWebPort
	a.mu.Unlock()
	result, err := a.updateService().Apply(ctx, a.Close, port, id)
	if err != nil {
		a.mu.Lock()
		a.updateRestarting, a.updateRestartError = false, err.Error()
		a.updateRestartErrorID = id
		a.mu.Unlock()
		slog.ErrorContext(ctx, "update_restart_failed", "error", err)
	}
	return result, err
}

func (a *App) waitForUpdateRestart(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.tryUpdateRestart(ctx)
		}
	}
}

func (a *App) tryUpdateRestart(ctx context.Context) {
	a.updateRestartMu.Lock()
	defer a.updateRestartMu.Unlock()
	a.mu.Lock()
	if a.updateWaitID == "" || a.closed {
		a.mu.Unlock()
		return
	}
	status, err := a.updates.Status()
	if err == nil && (status.ID != a.updateWaitID || status.Phase != update.PhaseStaged) {
		err = fmt.Errorf("the staged update changed while waiting to restart")
	}
	if err != nil {
		a.updateWaitID, a.updateRestartError = "", err.Error()
		a.updateRestartErrorID = status.ID
		a.mu.Unlock()
		slog.ErrorContext(ctx, "update_restart_wait_failed", "error", err)
		return
	}
	if status.Operation != "" {
		a.mu.Unlock()
		return
	}
	ready, err := a.fenceIdleUpdateLocked(ctx)
	if err != nil || !ready {
		a.mu.Unlock()
		if err != nil {
			slog.ErrorContext(ctx, "update_idle_check_failed", "error", err)
		}
		return
	}
	a.mu.Unlock()
	_, _ = a.applyUpdateNow(ctx, status.ID)
}

// Fence under the same lock as the final activity observation. Existing
// operations can finish; newly admitted work must wait for the new process.
func (a *App) fenceIdleUpdateLocked(ctx context.Context) (bool, error) {
	busy, err := a.updateWorkActiveLocked(ctx)
	if err != nil || busy {
		return false, err
	}
	a.updateRestarting = true
	return true, nil
}
