package handlers

import (
	"context"
	"errors"
	"io"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	loreapp "denova/internal/app/lore"
	"denova/internal/book/lore"
	imageasset "denova/internal/image/asset"
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

func (h *Handlers) HandleLoreItemImageUpload(ctx context.Context, c *app.RequestContext) {
	scope, ok := requireProjectScope(c)
	if !ok {
		return
	}
	fileHeader, err := c.FormFile("file")
	if err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.lore.imageUploadRequired")
		return
	}
	if fileHeader.Size > imageasset.MaxLoreImageUploadBytes {
		writeErrorKey(c, consts.StatusBadRequest, "api.lore.imageTooLarge")
		return
	}
	file, err := fileHeader.Open()
	if err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.lore.imageReadFailed", "detail", err.Error())
		return
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, imageasset.MaxLoreImageUploadBytes+1))
	if err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.lore.imageReadFailed", "detail", err.Error())
		return
	}
	if len(data) > imageasset.MaxLoreImageUploadBytes {
		writeErrorKey(c, consts.StatusBadRequest, "api.lore.imageTooLarge")
		return
	}
	item, err := h.app.Lore().UploadItemImage(ctx, scope.ProjectID, c.Param("id"), fileHeader.Filename, data)
	if err != nil {
		switch {
		case errors.Is(err, imageasset.ErrLoreImageUploadEmpty), errors.Is(err, imageasset.ErrLoreImageUploadInvalid):
			writeErrorKey(c, consts.StatusBadRequest, "api.lore.imageInvalid")
			return
		case errors.Is(err, imageasset.ErrLoreImageUploadTooLarge):
			writeErrorKey(c, consts.StatusBadRequest, "api.lore.imageTooLarge")
			return
		}
		writeProjectBookError(c, err, "api.projectBook.loreFailed")
		return
	}
	writeJSON(c, consts.StatusOK, item)
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
