package handlers

import (
	"context"
	"errors"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	"denova/internal/api/sse"
	novaApp "denova/internal/app"
	loreapp "denova/internal/app/lore"
	"denova/internal/book/lore"
)

func (h *Handlers) HandleLoreClassificationPreview(ctx context.Context, c *app.RequestContext) {
	if !h.requireWorkspace(c) {
		return
	}
	var body loreapp.ClassificationPreviewRequest
	if err := c.BindJSON(&body); err != nil && len(c.Request.Body()) > 0 {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
		return
	}
	expectedWorkspace := workspaceChangeExpectedWorkspace(c)
	preview, err := h.app.Lore().PreviewClassificationForWorkspace(ctx, expectedWorkspace, body)
	if err != nil {
		if errors.Is(err, novaApp.ErrWorkspaceChanged) || errors.Is(err, novaApp.ErrNoWorkspace) || errors.Is(err, novaApp.ErrWorkspaceTransition) {
			h.writeWorkspaceChangeLeaseError(c, expectedWorkspace, err)
			return
		}
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, preview)
}

func (h *Handlers) HandleLoreClassificationApply(ctx context.Context, c *app.RequestContext) {
	if !h.requireWorkspace(c) {
		return
	}
	var body loreapp.ClassificationApplyRequest
	if err := c.BindJSON(&body); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
		return
	}
	var result lore.TypeApplyResult
	_, err := h.app.WithLoreStore(workspaceChangeExpectedWorkspace(c), func(store *lore.Store) error {
		var applyErr error
		result, applyErr = store.ApplyTypeChanges(body.Revision, body.Changes)
		return applyErr
	})
	if err != nil {
		if errors.Is(err, novaApp.ErrWorkspaceChanged) || errors.Is(err, novaApp.ErrNoWorkspace) {
			h.writeWorkspaceChangeLeaseError(c, workspaceChangeExpectedWorkspace(c), err)
			return
		}
		if errors.Is(err, lore.ErrRevisionConflict) {
			writeErrorKey(c, consts.StatusConflict, "api.resource.revisionConflict")
			return
		}
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, result)
}

func (h *Handlers) HandleLoreItemImageGenerate(ctx context.Context, c *app.RequestContext) {
	if !h.requireWorkspace(c) {
		return
	}
	var body loreapp.ItemImageGenerateRequest
	if err := c.BindJSON(&body); err != nil && len(c.Request.Body()) > 0 {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
		return
	}
	expectedWorkspace := workspaceChangeExpectedWorkspace(c)
	item, err := h.app.Lore().GenerateItemImageForWorkspace(ctx, expectedWorkspace, c.Param("id"), body)
	if err != nil {
		if errors.Is(err, novaApp.ErrWorkspaceChanged) || errors.Is(err, novaApp.ErrNoWorkspace) || errors.Is(err, novaApp.ErrWorkspaceTransition) {
			h.writeWorkspaceChangeLeaseError(c, expectedWorkspace, err)
			return
		}
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, item)
}

func (h *Handlers) HandleLoreImagesGenerateStream(ctx context.Context, c *app.RequestContext) {
	if !h.requireWorkspace(c) {
		return
	}
	var body loreapp.ImagesGenerateRequest
	if err := c.BindJSON(&body); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
		return
	}
	expectedWorkspace := workspaceChangeExpectedWorkspace(c)
	task, err := h.app.Lore().StartImagesGenerateTaskForWorkspace(ctx, expectedWorkspace, body)
	if err != nil {
		if errors.Is(err, novaApp.ErrWorkspaceChanged) || errors.Is(err, novaApp.ErrNoWorkspace) || errors.Is(err, novaApp.ErrWorkspaceTransition) {
			h.writeWorkspaceChangeLeaseError(c, expectedWorkspace, err)
			return
		}
		if errors.Is(err, loreapp.ErrImageTaskRunning) {
			writeError(c, consts.StatusConflict, err.Error())
			return
		}
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	sse.StreamTask(ctx, c, task)
}

func (h *Handlers) HandleLoreImagesGenerateAbort(ctx context.Context, c *app.RequestContext) {
	expectedWorkspace := workspaceChangeExpectedWorkspace(c)
	if err := h.app.Lore().AbortImagesGenerateTask(expectedWorkspace); err != nil {
		if errors.Is(err, novaApp.ErrWorkspaceChanged) || errors.Is(err, novaApp.ErrNoWorkspace) {
			h.writeWorkspaceChangeLeaseError(c, expectedWorkspace, err)
			return
		}
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handlers) HandleLoreItemImageDelete(ctx context.Context, c *app.RequestContext) {
	if !h.requireWorkspace(c) {
		return
	}
	var item lore.Item
	_, ok := h.withLoreStore(c, func(store *lore.Store) error {
		var err error
		item, err = store.SetImage(c.Param("id"), nil)
		return err
	})
	if !ok {
		return
	}
	writeJSON(c, consts.StatusOK, item)
}

func (h *Handlers) withLoreStore(c *app.RequestContext, action func(*lore.Store) error) (string, bool) {
	expectedWorkspace := workspaceChangeExpectedWorkspace(c)
	workspace, err := h.app.WithLoreStore(expectedWorkspace, action)
	if err != nil {
		if errors.Is(err, novaApp.ErrWorkspaceChanged) || errors.Is(err, novaApp.ErrNoWorkspace) {
			h.writeWorkspaceChangeLeaseError(c, expectedWorkspace, err)
		} else {
			writeError(c, consts.StatusBadRequest, err.Error())
		}
		return "", false
	}
	return workspace, true
}
