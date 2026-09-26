package handlers

import (
	"context"
	"errors"
	"io"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	imageapp "denova/internal/app/image"
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
		var toolErr *imageapp.ImageToolError
		switch {
		case errors.Is(err, imageapp.ErrImageToolNotCalled):
			writeErrorKey(c, consts.StatusBadRequest, "api.lore.imageAgentNotCalled")
		case errors.Is(err, imageapp.ErrImageOutputMissing):
			writeErrorKey(c, consts.StatusBadRequest, "api.lore.imageAgentOutputMissing")
		case errors.As(err, &toolErr):
			if toolErr.Detail == "" {
				writeErrorKey(c, consts.StatusBadRequest, "api.lore.imageAgentToolFailed")
			} else {
				writeErrorKey(c, consts.StatusBadRequest, "api.lore.imageAgentToolFailedWithDetail", "detail", toolErr.Detail)
			}
		default:
			writeProjectBookError(c, err, "api.projectBook.loreFailed")
		}
		return
	}
	writeJSON(c, consts.StatusOK, item)
}

func (h *Handlers) HandleLoreItemMaterialUpload(ctx context.Context, c *app.RequestContext) {
	scope, ok := requireProjectScope(c)
	if !ok {
		return
	}
	fileHeader, err := c.FormFile("file")
	if err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.lore.materialUploadRequired")
		return
	}
	if fileHeader.Size > lore.MaxMaterialUploadBytes {
		writeErrorKey(c, consts.StatusBadRequest, "api.lore.materialTooLarge")
		return
	}
	file, err := fileHeader.Open()
	if err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.lore.materialReadFailed", "detail", err.Error())
		return
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, lore.MaxMaterialUploadBytes+1))
	if err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.lore.materialReadFailed", "detail", err.Error())
		return
	}
	if len(data) > lore.MaxMaterialUploadBytes {
		writeErrorKey(c, consts.StatusBadRequest, "api.lore.materialTooLarge")
		return
	}
	item, err := h.app.Lore().UploadItemMaterial(ctx, scope.ProjectID, c.Param("id"), fileHeader.Filename, data)
	if err != nil {
		switch {
		case errors.Is(err, lore.ErrMaterialInvalid):
			writeErrorKey(c, consts.StatusBadRequest, "api.lore.materialInvalid")
			return
		case errors.Is(err, lore.ErrMaterialTooLarge):
			writeErrorKey(c, consts.StatusBadRequest, "api.lore.materialTooLarge")
			return
		}
		writeProjectBookError(c, err, "api.projectBook.loreFailed")
		return
	}
	writeJSON(c, consts.StatusOK, item)
}

func (h *Handlers) HandleLoreMaterialAssets(ctx context.Context, c *app.RequestContext) {
	scope, ok := requireProjectScope(c)
	if !ok {
		return
	}
	assets, err := h.app.Lore().MaterialAssets(ctx, scope.ProjectID)
	if err != nil {
		writeProjectBookError(c, err, "api.projectBook.loreFailed")
		return
	}
	writeJSON(c, consts.StatusOK, assets)
}
func (h *Handlers) HandleLoreMaterialMutation(ctx context.Context, c *app.RequestContext) {
	scope, ok := requireProjectScope(c)
	if !ok {
		return
	}
	var body lore.MaterialMutation
	if err := c.BindJSON(&body); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequest")
		return
	}
	item, err := h.app.Lore().MutateMaterial(ctx, scope.ProjectID, c.Param("id"), body)
	if err != nil {
		writeProjectBookError(c, err, "api.projectBook.loreFailed")
		return
	}
	writeJSON(c, consts.StatusOK, item)
}
