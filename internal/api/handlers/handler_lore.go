package handlers

import (
	"context"
	"errors"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	"denova/internal/api/sse"
	novaApp "denova/internal/app"
	"denova/internal/book"
)

func (h *Handlers) HandleLoreItems(ctx context.Context, c *app.RequestContext) {
	if !h.requireWorkspace(c) {
		return
	}
	var items []book.LoreItem
	_, ok := h.withLoreStore(c, func(store *book.LoreStore) error {
		var err error
		items, err = store.ListAll()
		return err
	})
	if !ok {
		return
	}
	writeJSON(c, consts.StatusOK, map[string]any{"items": items})
}

func (h *Handlers) HandleLoreItemCreate(ctx context.Context, c *app.RequestContext) {
	if !h.requireWorkspace(c) {
		return
	}
	var body book.LoreItemInput
	if err := c.BindJSON(&body); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
		return
	}
	var item book.LoreItem
	_, ok := h.withLoreStore(c, func(store *book.LoreStore) error {
		var err error
		item, err = store.Create(body)
		return err
	})
	if !ok {
		return
	}
	writeJSON(c, consts.StatusOK, item)
}

func (h *Handlers) HandleLoreItemUpdate(ctx context.Context, c *app.RequestContext) {
	if !h.requireWorkspace(c) {
		return
	}
	var body book.LoreItemInput
	if err := c.BindJSON(&body); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
		return
	}
	var item book.LoreItem
	_, err := h.app.WithLoreStore(workspaceChangeExpectedWorkspace(c), func(store *book.LoreStore) error {
		var updateErr error
		item, updateErr = store.Update(c.Param("id"), body)
		return updateErr
	})
	if err != nil {
		if errors.Is(err, novaApp.ErrWorkspaceChanged) || errors.Is(err, novaApp.ErrNoWorkspace) {
			h.writeWorkspaceChangeLeaseError(c, workspaceChangeExpectedWorkspace(c), err)
			return
		}
		if errors.Is(err, book.ErrLoreRevisionConflict) {
			writeErrorKey(c, consts.StatusConflict, "api.resource.revisionConflict")
			return
		}
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, item)
}

func (h *Handlers) HandleLoreItemDelete(ctx context.Context, c *app.RequestContext) {
	if !h.requireWorkspace(c) {
		return
	}
	_, ok := h.withLoreStore(c, func(store *book.LoreStore) error { return store.Delete(c.Param("id")) })
	if !ok {
		return
	}
	writeJSON(c, consts.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handlers) HandleLoreClassificationPreview(ctx context.Context, c *app.RequestContext) {
	if !h.requireWorkspace(c) {
		return
	}
	var body novaApp.LoreClassificationPreviewRequest
	if err := c.BindJSON(&body); err != nil && len(c.Request.Body()) > 0 {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
		return
	}
	expectedWorkspace := workspaceChangeExpectedWorkspace(c)
	preview, err := h.app.PreviewLoreClassificationForWorkspace(ctx, expectedWorkspace, body)
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
	var body novaApp.LoreClassificationApplyRequest
	if err := c.BindJSON(&body); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
		return
	}
	var result book.LoreTypeApplyResult
	_, err := h.app.WithLoreStore(workspaceChangeExpectedWorkspace(c), func(store *book.LoreStore) error {
		var applyErr error
		result, applyErr = store.ApplyTypeChanges(body.Revision, body.Changes)
		return applyErr
	})
	if err != nil {
		if errors.Is(err, novaApp.ErrWorkspaceChanged) || errors.Is(err, novaApp.ErrNoWorkspace) {
			h.writeWorkspaceChangeLeaseError(c, workspaceChangeExpectedWorkspace(c), err)
			return
		}
		if errors.Is(err, book.ErrLoreRevisionConflict) {
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
	var body novaApp.LoreItemImageGenerateRequest
	if err := c.BindJSON(&body); err != nil && len(c.Request.Body()) > 0 {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
		return
	}
	expectedWorkspace := workspaceChangeExpectedWorkspace(c)
	item, err := h.app.GenerateLoreItemImageForWorkspace(ctx, expectedWorkspace, c.Param("id"), body)
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
	var body novaApp.LoreImagesGenerateRequest
	if err := c.BindJSON(&body); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
		return
	}
	expectedWorkspace := workspaceChangeExpectedWorkspace(c)
	task, err := h.app.StartLoreImagesGenerateTaskForWorkspace(expectedWorkspace, body)
	if err != nil {
		if errors.Is(err, novaApp.ErrWorkspaceChanged) || errors.Is(err, novaApp.ErrNoWorkspace) || errors.Is(err, novaApp.ErrWorkspaceTransition) {
			h.writeWorkspaceChangeLeaseError(c, expectedWorkspace, err)
			return
		}
		if errors.Is(err, novaApp.ErrLoreImageTaskRunning) {
			writeError(c, consts.StatusConflict, err.Error())
			return
		}
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	sse.StreamTask(c, task)
}

func (h *Handlers) HandleLoreImagesGenerateAbort(ctx context.Context, c *app.RequestContext) {
	expectedWorkspace := workspaceChangeExpectedWorkspace(c)
	if err := h.app.AbortLoreImagesGenerateTaskForWorkspace(expectedWorkspace); err != nil {
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
	var item book.LoreItem
	_, ok := h.withLoreStore(c, func(store *book.LoreStore) error {
		var err error
		item, err = store.SetImage(c.Param("id"), nil)
		return err
	})
	if !ok {
		return
	}
	writeJSON(c, consts.StatusOK, item)
}

func (h *Handlers) withLoreStore(c *app.RequestContext, action func(*book.LoreStore) error) (string, bool) {
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
