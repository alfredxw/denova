package app

import (
	"context"

	"nova/internal/agent"
	"nova/internal/update"
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

func (a *App) StartInstallUpdateTask() *Task {
	return NewTask(func(ctx context.Context, task *Task, emit func(agent.Event)) {
		result, err := update.NewService().InstallWithProgress(ctx, func(progress update.InstallProgress) {
			emit(agent.Event{Type: "update_progress", Data: progress})
		})
		if err != nil {
			emit(agent.Event{Type: "error", Data: map[string]string{"message": err.Error()}})
			return
		}
		emit(agent.Event{Type: "update_result", Data: result})
	})
}
