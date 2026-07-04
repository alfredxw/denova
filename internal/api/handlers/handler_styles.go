package handlers

import (
	"context"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	"denova/internal/styleref"
)

func (h *Handlers) HandleStyleReferences(ctx context.Context, c *app.RequestContext) {
	refs, err := h.app.StyleReferences()
	if err != nil {
		writeError(c, consts.StatusConflict, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, map[string]any{"styles": refs})
}

func (h *Handlers) HandleStyleReferenceSave(ctx context.Context, c *app.RequestContext) {
	var body styleref.WriteRequest
	if err := c.BindJSON(&body); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
		return
	}
	ref, err := h.app.SaveStyleReference(body)
	if err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, ref)
}

func (h *Handlers) HandleStyleReferenceDelete(ctx context.Context, c *app.RequestContext) {
	path := strings.TrimSpace(c.Query("path"))
	if path == "" {
		path = strings.TrimSpace(c.Param("path"))
	}
	if err := h.app.DeleteStyleReference(path); err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, map[string]string{"status": "ok"})
}
