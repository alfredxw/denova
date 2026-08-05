package handlers

import (
	"context"
	"errors"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	"denova/internal/api/sse"
	loreapp "denova/internal/app/lore"
	"denova/internal/book/lore"
)

func (h *Handlers) HandleLoreClassificationPreview(ctx context.Context, c *app.RequestContext) {
	scope, ok := requireProjectScope(c)
	if !ok {
		return
	}
	var body loreapp.ClassificationPreviewRequest
	if err := c.BindJSON(&body); err != nil && len(c.Request.Body()) > 0 {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
		return
	}
	preview, err := h.app.Lore().PreviewClassification(ctx, scope.ProjectID, body)
	if err != nil {
		writeProjectBookError(c, err, "api.projectBook.loreFailed")
		return
	}
	writeJSON(c, consts.StatusOK, preview)
}

func (h *Handlers) HandleLoreClassificationApply(ctx context.Context, c *app.RequestContext) {
	scope, ok := requireProjectScope(c)
	if !ok {
		return
	}
	var body loreapp.ClassificationApplyRequest
	if err := c.BindJSON(&body); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
		return
	}
	result, err := h.app.Lore().ApplyClassification(ctx, scope.ProjectID, body)
	if err != nil {
		if errors.Is(err, lore.ErrRevisionConflict) {
			writeErrorKey(c, consts.StatusConflict, "api.resource.revisionConflict")
			return
		}
		writeProjectBookError(c, err, "api.projectBook.loreFailed")
		return
	}
	writeJSON(c, consts.StatusOK, result)
}

func (h *Handlers) HandleLoreItemImageGenerate(ctx context.Context, c *app.RequestContext) {
	scope, ok := requireProjectScope(c)
	if !ok {
		return
	}
	var body loreapp.ItemImageGenerateRequest
	if err := c.BindJSON(&body); err != nil && len(c.Request.Body()) > 0 {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
		return
	}
	item, err := h.app.Lore().GenerateItemImage(ctx, scope.ProjectID, c.Param("id"), body)
	if err != nil {
		writeProjectBookError(c, err, "api.projectBook.loreFailed")
		return
	}
	writeJSON(c, consts.StatusOK, item)
}

func (h *Handlers) HandleLoreImagesGenerateStream(ctx context.Context, c *app.RequestContext) {
	scope, ok := requireProjectScope(c)
	if !ok {
		return
	}
	var body loreapp.ImagesGenerateRequest
	if err := c.BindJSON(&body); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
		return
	}
	task, err := h.app.Lore().StartImagesGenerateTask(ctx, scope.ProjectID, body)
	if err != nil {
		if errors.Is(err, loreapp.ErrImageTaskRunning) {
			writeError(c, consts.StatusConflict, err.Error())
			return
		}
		writeProjectBookError(c, err, "api.projectBook.loreFailed")
		return
	}
	sse.StreamTask(ctx, c, task)
}

func (h *Handlers) HandleLoreImagesGenerateAbort(ctx context.Context, c *app.RequestContext) {
	scope, ok := requireProjectScope(c)
	if !ok {
		return
	}
	if err := h.app.Lore().AbortImagesGenerateTask(scope.ProjectID); err != nil {
		writeProjectBookError(c, err, "api.projectBook.loreFailed")
		return
	}
	writeJSON(c, consts.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handlers) HandleLoreItemImageDelete(ctx context.Context, c *app.RequestContext) {
	scope, ok := requireProjectScope(c)
	if !ok {
		return
	}
	item, err := h.app.Lore().ClearItemImage(ctx, scope.ProjectID, c.Param("id"))
	if err != nil {
		writeProjectBookError(c, err, "api.projectBook.loreFailed")
		return
	}
	writeJSON(c, consts.StatusOK, item)
}
