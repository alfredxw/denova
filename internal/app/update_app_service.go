package app

import (
	"context"
	"io"
	"log/slog"

	agentrun "denova/internal/agents/run"
	apptask "denova/internal/app/task"
	"denova/internal/i18n"
	"denova/internal/update"
)

func (a *App) CheckUpdate(ctx context.Context) (update.CheckResult, error) {
	return update.NewService().Check(ctx)
}

func (a *App) InstallUpdate(ctx context.Context) (update.InstallResult, error) {
	return update.NewService().Install(ctx)
}

// InstallLocalUpdate validates and stages an uploaded archive for explicit apply.
func (a *App) InstallLocalUpdate(ctx context.Context, name string, archive io.Reader) (update.InstallResult, error) {
	return update.NewService().InstallLocal(ctx, name, archive)
}

func (a *App) ApplyUpdate(ctx context.Context) (update.ApplyResult, error) {
	return update.NewService().Apply(ctx)
}

// StartInstallUpdateTask retains the initiating request's language throughout
// the detached download. Internal diagnostics stay in the server log.
func (a *App) StartInstallUpdateTask(locale string) *apptask.Task {
	return apptask.New(func(ctx context.Context, task *apptask.Task, emit func(agentrun.Event)) {
		result, err := update.NewService().InstallWithProgress(ctx, func(progress update.InstallProgress) {
			emit(agentrun.Event{Type: "update_progress", Data: progress})
		})
		if err != nil {
			slog.ErrorContext(ctx, "[app/update_app_service.go] update installation failed", "error", err)
			emit(agentrun.Event{Type: "error", Data: map[string]string{"message": i18n.New(locale).T("api.update.installFailed")}})
			return
		}
		emit(agentrun.Event{Type: "update_result", Data: result})
	})
}
