package handlers

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	"denova/internal/api/sse"
	"denova/internal/app/activity"
	"denova/internal/update"
)

func (h *Handlers) HandleUpdateNotes(ctx context.Context, c *app.RequestContext) {
	result, err := h.app.Activity().UpdateNotes(ctx, c.Query("from"), c.Query("to"), requestLocale(c))
	if err != nil {
		slog.WarnContext(ctx, "update_notes_failed", "from_version", c.Query("from"), "to_version", c.Query("to"), "error", err)
		if errors.Is(err, activity.ErrUpdateNotesRange) {
			writeErrorKey(c, consts.StatusBadRequest, "api.update.notesRangeInvalid")
		} else {
			writeErrorKey(c, consts.StatusBadGateway, "api.update.notesFailed")
		}
		return
	}
	writeJSON(c, consts.StatusOK, result)
}

func (h *Handlers) HandleUpdateCheck(ctx context.Context, c *app.RequestContext) {
	result, err := h.app.CheckUpdate(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "[handlers/handler_update.go] update check failed", "error", err)
		writeErrorKey(c, consts.StatusBadGateway, "api.update.checkFailed", "detail", err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, result)
}

func (h *Handlers) HandleUpdateInstall(ctx context.Context, c *app.RequestContext) {
	result, err := h.app.InstallUpdate(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "[handlers/handler_update.go] update install failed", "error", err)
		writeErrorKey(c, consts.StatusBadGateway, "api.update.installFailed", "detail", err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, result)
}

func (h *Handlers) HandleUpdateApply(ctx context.Context, c *app.RequestContext) {
	request := struct {
		When string `json:"when"`
	}{When: "now"}
	if len(c.Request.Body()) != 0 {
		if err := c.BindJSON(&request); err != nil {
			writeErrorKey(c, consts.StatusBadRequest, "api.update.applyFailed")
			return
		}
	}
	result, err := h.app.ScheduleUpdateRestart(ctx, request.When)
	if err != nil {
		slog.ErrorContext(ctx, "[handlers/handler_update.go] update apply failed", "error", err)
		writeErrorKey(c, consts.StatusBadGateway, "api.update.applyFailed", "detail", err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, result)
}

func (h *Handlers) HandleUpdateInstallStream(ctx context.Context, c *app.RequestContext) {
	task := h.app.StartInstallUpdateTask(requestLocale(c))
	slog.InfoContext(ctx, fmt.Sprintf("[update-sse] attach install task_id=%s", task.ID()))
	sse.StreamTask(ctx, c, task)
}

func (h *Handlers) HandleUpdateUpload(ctx context.Context, c *app.RequestContext) {
	header, err := c.FormFile("file")
	if err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.update.uploadRequired")
		return
	}
	if header.Size > update.MaxLocalArchiveBytes {
		writeErrorKey(c, consts.StatusRequestEntityTooLarge, "api.update.tooLarge")
		return
	}
	file, err := header.Open()
	if err != nil {
		slog.ErrorContext(ctx, "Could not open local update upload", "error", err)
		writeErrorKey(c, consts.StatusBadRequest, "api.update.invalidPackage", "detail", err.Error())
		return
	}
	defer file.Close()
	result, err := h.app.InstallLocalUpdate(ctx, header.Filename, file)
	if err != nil {
		slog.ErrorContext(ctx, "Local update installation failed", "asset", header.Filename, "error", err)
		status, key := consts.StatusBadRequest, "api.update.invalidPackage"
		switch {
		case errors.Is(err, update.ErrPackageVersion):
			key = "api.update.newerRequired"
		case errors.Is(err, update.ErrPackagePlatform):
			key = "api.update.wrongPlatform"
		case errors.Is(err, update.ErrPackageTooLarge):
			status, key = consts.StatusRequestEntityTooLarge, "api.update.tooLarge"
		case errors.Is(err, update.ErrUpdateBusy):
			status, key = consts.StatusConflict, "api.update.busy"
		case errors.Is(err, update.ErrInvalidPackage):
		default:
			status, key = consts.StatusInternalServerError, "api.update.installFailed"
		}
		writeErrorKey(c, status, key, "detail", err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, result)
}

func (h *Handlers) HandleUpdateStatus(ctx context.Context, c *app.RequestContext) {
	result, err := h.app.UpdateStatus()
	if err != nil {
		slog.ErrorContext(ctx, "update_status_failed", "error", err)
		writeErrorKey(c, consts.StatusInternalServerError, "api.update.checkFailed", "detail", err.Error())
		return
	}
	c.Header("Cache-Control", "no-store")
	writeJSON(c, consts.StatusOK, result)
}
