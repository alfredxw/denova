package app

import (
	"context"
	"log/slog"
	"time"
)

// startResourceUpdates is owned by the App root scope, including cancellation
// and shutdown waiting. All source policy is owned by installation records.
func (a *App) startResourceUpdates(ctx context.Context) {
	workerCtx, lease, err := a.rootScope.AcquireContext(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "Start resource updates failed", "error", err)
		return
	}
	go func() {
		defer lease.Release()
		defer func() {
			if recovered := recover(); recovered != nil {
				slog.ErrorContext(ctx, "Resource update worker panic recovered", "error", recovered)
			}
		}()
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		a.ResourceExchange().UpdateDue(workerCtx, time.Now().UTC(), a.ApplyResourcePlan)
		for {
			select {
			case <-workerCtx.Done():
				return
			case now := <-ticker.C:
				a.ResourceExchange().UpdateDue(workerCtx, now.UTC(), a.ApplyResourcePlan)
			}
		}
	}()
}
