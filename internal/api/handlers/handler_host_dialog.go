package handlers

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	"denova/internal/hostdialog"
)

// HandleDirectoryPicker opens a native folder chooser on the machine running
// Denova. The API layer registers this handler behind a local-client guard.
func (h *Handlers) HandleDirectoryPicker(ctx context.Context, c *app.RequestContext) {
	var request struct {
		InitialPath string `json:"initial_path"`
	}
	if len(c.Request.Body()) > 0 {
		if err := c.BindJSON(&request); err != nil {
			writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidBody")
			return
		}
	}
	selection, err := h.directoryPicker.SelectDirectory(ctx, hostdialog.DirectoryOptions{
		Title:       messageKey(c, "api.hostDialog.projectDirectoryTitle"),
		InitialPath: request.InitialPath,
	})
	if err != nil {
		slog.ErrorContext(ctx, fmt.Sprintf("[api/handlers/handler_host_dialog.go] native directory picker failed err=%v", err))
		if errors.Is(err, hostdialog.ErrUnavailable) {
			writeErrorKey(c, consts.StatusServiceUnavailable, "api.hostDialog.unavailable")
			return
		}
		writeErrorKey(c, consts.StatusInternalServerError, "api.hostDialog.failed", "detail", err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, selection)
}

// HandleProjectFileReveal opens the native file manager for one scoped project
// item. The route is guarded so only a browser on the Denova host may call it.
func (h *Handlers) HandleProjectFileReveal(ctx context.Context, c *app.RequestContext) {
	scope, ok := requireProjectScope(c)
	if !ok {
		return
	}
	var request struct {
		Path string `json:"path"`
	}
	if err := c.BindJSON(&request); err != nil || request.Path == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.workspace.pathMissing")
		return
	}
	hostPath, err := h.app.ProjectFiles().ResolveHostPath(scope.ProjectID, request.Path)
	if err != nil {
		writeProjectFilesError(c, err, "api.projectFiles.revealFailed")
		return
	}
	if err := h.pathRevealer.RevealPath(ctx, hostPath); err != nil {
		slog.ErrorContext(ctx, fmt.Sprintf("[api/handlers/handler_host_dialog.go] revealing project path in native file manager failed project_id=%s path=%s err=%v", scope.ProjectID, request.Path, err))
		if errors.Is(err, hostdialog.ErrUnavailable) {
			writeErrorKey(c, consts.StatusServiceUnavailable, "api.hostReveal.unavailable")
			return
		}
		writeErrorKey(c, consts.StatusInternalServerError, "api.hostReveal.failed", "detail", err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, map[string]any{
		"project_id": scope.ProjectID,
		"path":       request.Path,
	})
}
