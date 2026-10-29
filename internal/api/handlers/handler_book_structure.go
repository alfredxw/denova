package handlers

import (
	"context"
	"errors"
	"log/slog"

	"denova/internal/book"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
)

func (h *Handlers) HandleBookStructure(ctx context.Context, c *app.RequestContext) {
	scope, ok := requireProjectScope(c)
	if !ok {
		return
	}
	var input struct {
		Structure book.ChapterStructure `json:"structure"`
		Revision  string                `json:"revision"`
	}
	if err := c.BindJSON(&input); err != nil || !input.Structure.Valid() {
		writeErrorKey(c, consts.StatusBadRequest, "api.bookStructure.invalid")
		return
	}
	if string(c.Method()) == "PUT" && input.Revision == "" {
		writeErrorKey(c, consts.StatusConflict, "api.bookStructure.conflict")
		return
	}
	if string(c.Method()) == "POST" {
		input.Revision = ""
	}
	preview, err := h.app.BookStructure(ctx, scope.ProjectID, input.Structure, input.Revision, book.StructureTitles{Volume: messageKey(c, "api.bookStructure.firstVolume"), Section: messageKey(c, "api.bookStructure.firstSection")}, messageKey(c, "api.bookStructure.backup"))
	if err != nil {
		slog.WarnContext(ctx, "Book structure operation failed", "project_id", scope.ProjectID, "target", input.Structure, "error", err)
		key, status := "api.bookStructure.failed", consts.StatusInternalServerError
		if errors.Is(err, book.ErrStructureConflict) {
			key, status = "api.bookStructure.conflict", consts.StatusConflict
		}
		if errors.Is(err, book.ErrUnsupportedStructure) {
			key, status = "api.bookStructure.unsupported", consts.StatusBadRequest
		}
		writeErrorKey(c, status, key)
		return
	}
	writeJSON(c, consts.StatusOK, preview)
}
