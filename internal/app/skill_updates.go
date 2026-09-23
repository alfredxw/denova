package app

import (
	"context"
	"log/slog"
	"path/filepath"
	"time"

	"denova/internal/agents/skills"
	"denova/internal/project"
)

// startSkillUpdates is owned by the App root scope, including cancellation and
// shutdown waiting. It scans registered Projects without changing foreground
// navigation or retaining machine-specific paths as durable identities.
func (a *App) startSkillUpdates(ctx context.Context) {
	workerCtx, lease, err := a.rootScope.AcquireContext(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "Start Skill updates failed", "error", err)
		return
	}
	go func() {
		defer lease.Release()
		defer func() {
			if recovered := recover(); recovered != nil {
				slog.ErrorContext(ctx, "Skill update worker panic recovered", "error", recovered)
			}
		}()
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		a.updateDueSkills(workerCtx, time.Now().UTC())
		for {
			select {
			case <-workerCtx.Done():
				return
			case now := <-ticker.C:
				a.updateDueSkills(workerCtx, now.UTC())
			}
		}
	}()
}

func (a *App) updateDueSkills(ctx context.Context, now time.Time) {
	a.mu.RLock()
	dataDir, registry := a.cfg.DataDir(), a.projectRegistry
	a.mu.RUnlock()
	skills.UpdateDue(ctx, []skills.Directory{{Scope: skills.ScopeUser, Path: filepath.Join(dataDir, "skills"), Writable: true}}, now)
	if ctx.Err() != nil || registry == nil {
		return
	}
	projects, err := registry.List(false)
	if err != nil {
		slog.ErrorContext(ctx, "List Projects for Skill updates failed", "error", err)
		return
	}
	for _, record := range projects {
		if ctx.Err() != nil {
			return
		}
		if record.Status != project.StatusAvailable {
			continue
		}
		layout, err := registry.Layout(record)
		if err != nil {
			continue
		}
		// Background checks never create stores or migrate legacy workspaces.
		skills.UpdateDue(ctx, []skills.Directory{{Scope: skills.ScopeWorkspace, Path: filepath.Join(layout.ContentRoot, "skills"), Writable: true}}, now)
	}
}
