package handlers

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	"denova/internal/api/sse"
)

func (h *Handlers) HandleUpdateCheck(ctx context.Context, c *app.RequestContext) {
	result, err := h.app.CheckUpdate(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "[handlers/handler_update.go] update check failed", "error", err)
		writeErrorKey(c, consts.StatusBadGateway, "api.update.checkFailed")
		return
	}
	writeJSON(c, consts.StatusOK, result)
}

func (h *Handlers) HandleUpdateInstall(ctx context.Context, c *app.RequestContext) {
	result, err := h.app.InstallUpdate(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "[handlers/handler_update.go] update install failed", "error", err)
		writeErrorKey(c, consts.StatusBadGateway, "api.update.installFailed")
		return
	}
	writeJSON(c, consts.StatusOK, result)
}

func (h *Handlers) HandleUpdateApply(ctx context.Context, c *app.RequestContext) {
	result, err := h.app.ApplyUpdate(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "[handlers/handler_update.go] update apply failed", "error", err)
		writeErrorKey(c, consts.StatusBadGateway, "api.update.applyFailed")
		return
	}
	writeJSON(c, consts.StatusOK, result)
}

func (h *Handlers) HandleUpdateInstallStream(ctx context.Context, c *app.RequestContext) {
	task := h.app.StartInstallUpdateTask(requestLocale(c))
	slog.InfoContext(ctx, fmt.Sprintf("[update-sse] attach install task_id=%s", task.ID()))
	sse.StreamTask(ctx, c, task)
}
