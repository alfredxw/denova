package app

import (
	"context"

	"denova/internal/update"
)

// DesktopActivity is a live projection for the owning desktop process, never
// persisted recovery state. Scheduling and task admission remain App-owned.
type DesktopActivity struct {
	ActiveTasks    bool          `json:"active_tasks"`
	ScheduledTasks bool          `json:"scheduled_tasks"`
	Policy         update.Policy `json:"policy"`
}

func (a *App) DesktopActivity(ctx context.Context) (DesktopActivity, error) {
	a.mu.Lock()
	active, err := a.updateWorkActiveLocked(ctx)
	a.mu.Unlock()
	if err != nil {
		return DesktopActivity{}, err
	}
	policy, err := a.updatePolicy()
	if err != nil {
		return DesktopActivity{}, err
	}
	tasks, err := a.Automation().List()
	if err != nil {
		return DesktopActivity{}, err
	}
	result := DesktopActivity{ActiveTasks: active, Policy: policy}
	for _, task := range tasks {
		result.ScheduledTasks = result.ScheduledTasks || task.Enabled
	}
	return result, nil
}

// FenceDesktopUpdate atomically checks activity and closes admission. The
// caller must shut down this backend after a successful fence.
func (a *App) FenceDesktopUpdate(ctx context.Context) (bool, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.fenceIdleUpdateLocked(ctx)
}
