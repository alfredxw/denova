package handlers

import (
	"context"
	"errors"
	"log"

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
		log.Printf("[api/handlers/handler_host_dialog.go] native directory picker failed err=%v", err)
		if errors.Is(err, hostdialog.ErrUnavailable) {
			writeErrorKey(c, consts.StatusServiceUnavailable, "api.hostDialog.unavailable")
			return
		}
		writeErrorKey(c, consts.StatusInternalServerError, "api.hostDialog.failed", "detail", err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, selection)
}
