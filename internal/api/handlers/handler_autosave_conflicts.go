package handlers

import (
	"context"
	"errors"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	"denova/internal/autosaveconflict"
)

// HandleAutosaveConflictCreate stores an immutable recovery record. Request
// size is governed by the server-wide Hertz body limit so manuscript conflicts
// are not truncated by an unrelated small endpoint cap.
func (h *Handlers) HandleAutosaveConflictCreate(ctx context.Context, c *app.RequestContext) {
	var input autosaveconflict.Input
	if err := c.BindJSON(&input); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
		return
	}
	result, err := h.app.RecordAutosaveConflict(ctx, input)
	if err != nil {
		if errors.Is(err, autosaveconflict.ErrInvalidInput) {
			writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
			return
		}
		writeError(c, consts.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(c, consts.StatusCreated, map[string]string{
		"id":   result.Record.ID,
		"path": result.Path,
	})
}
