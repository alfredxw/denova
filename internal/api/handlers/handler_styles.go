package handlers

import (
	"context"
	"errors"
	"os"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	novaApp "denova/internal/app"
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

func (h *Handlers) HandleStyleReferenceFile(ctx context.Context, c *app.RequestContext) {
	path := strings.TrimSpace(c.Query("path"))
	if path == "" {
		writeError(c, consts.StatusBadRequest, "文风参考路径不能为空")
		return
	}
	doc, err := h.app.StyleReferenceFile(path)
	if err != nil {
		if errors.Is(err, novaApp.ErrNoWorkspace) {
			writeError(c, consts.StatusConflict, err.Error())
			return
		}
		writeError(c, fileReadStatus(err), err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, doc)
}

func (h *Handlers) HandleStyleReferenceFileUpdate(ctx context.Context, c *app.RequestContext) {
	var body styleref.UpdateRequest
	if err := c.BindJSON(&body); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
		return
	}
	doc, err := h.app.UpdateStyleReferenceFile(body)
	if err != nil {
		if errors.Is(err, novaApp.ErrNoWorkspace) {
			writeError(c, consts.StatusConflict, err.Error())
			return
		}
		if errors.Is(err, styleref.ErrReferenceRevisionConflict) {
			writeError(c, consts.StatusConflict, err.Error())
			return
		}
		writeError(c, styleReferenceWriteStatus(err), err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, doc)
}

func styleReferenceWriteStatus(err error) int {
	if os.IsNotExist(err) {
		return consts.StatusNotFound
	}
	return consts.StatusBadRequest
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
