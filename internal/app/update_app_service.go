package app

import (
	"context"
	agentrun "denova/internal/agents/run"
	apptask "denova/internal/app/task"

	"denova/internal/update"
)

func (a *App) CheckUpdate(ctx context.Context) (update.CheckResult, error) {
	return update.NewService().Check(ctx)
}

func (a *App) InstallUpdate(ctx context.Context) (update.InstallResult, error) {
	return update.NewService().Install(ctx)
}

func (a *App) ApplyUpdate(ctx context.Context) (update.ApplyResult, error) {
	return update.NewService().Apply(ctx)
}

func (a *App) StartInstallUpdateTask() *apptask.Task {
	return apptask.New(func(ctx context.Context, task *apptask.Task, emit func(agentrun.Event)) {
		result, err := update.NewService().InstallWithProgress(ctx, func(progress update.InstallProgress) {
			emit(agentrun.Event{Type: "update_progress", Data: progress})
		})
		if err != nil {
			emit(agentrun.Event{Type: "error", Data: map[string]string{"message": err.Error()}})
			return
		}
		emit(agentrun.Event{Type: "update_result", Data: result})
	})
}
