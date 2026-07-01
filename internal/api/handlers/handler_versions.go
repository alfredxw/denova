package handlers

import (
	"context"
	"errors"
	"strconv"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	"denova/internal/book"
)

type versionRestoreRequest struct {
	Paths []string `json:"paths"`
}

// handleVersionStatus GET /api/versions/status — 返回当前书籍本地版本状态。
func (h *Handlers) HandleVersionStatus(ctx context.Context, c *app.RequestContext) {
	if !h.requireWorkspace(c) {
		return
	}
	status, err := h.app.VersionStatus(ctx)
	if err != nil {
		writeVersionError(c, err)
		return
	}
	writeJSON(c, consts.StatusOK, status)
}

// handleVersionHistory GET /api/versions?limit=30 — 返回版本历史。
func (h *Handlers) HandleVersionHistory(ctx context.Context, c *app.RequestContext) {
	if !h.requireWorkspace(c) {
		return
	}
	limit := 30
	if raw := c.Query("limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			limit = parsed
		}
	}
	versions, err := h.app.VersionHistory(ctx, limit)
	if err != nil {
		writeVersionError(c, err)
		return
	}
	writeJSON(c, consts.StatusOK, map[string]any{"versions": versions})
}

// handleVersionCreate POST /api/versions — 创建手动版本。
func (h *Handlers) HandleVersionCreate(ctx context.Context, c *app.RequestContext) {
	if !h.requireWorkspace(c) {
		return
	}
	var req struct {
		Message string `json:"message"`
	}
	if len(c.Request.Body()) > 0 {
		if err := c.BindJSON(&req); err != nil {
			writeErrorKey(c, consts.StatusBadRequest, "api.versions.invalidCreateRequest")
			return
		}
	}
	result, err := h.app.CreateVersion(ctx, req.Message)
	if err != nil {
		writeVersionError(c, err)
		return
	}
	writeJSON(c, consts.StatusOK, result)
}

// handleVersionDiff GET /api/versions/:id/diff?path=optional — 返回版本差异。
func (h *Handlers) HandleVersionDiff(ctx context.Context, c *app.RequestContext) {
	if !h.requireWorkspace(c) {
		return
	}
	id := c.Param("id")
	if id == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.versions.idRequired")
		return
	}
	diff, err := h.app.VersionDiff(ctx, id, c.Query("path"))
	if err != nil {
		writeVersionError(c, err)
		return
	}
	writeJSON(c, consts.StatusOK, diff)
}

// HandleVersionRestorePlan POST /api/versions/:id/restore-plan — 返回版本恢复影响预览。
func (h *Handlers) HandleVersionRestorePlan(ctx context.Context, c *app.RequestContext) {
	if !h.requireWorkspace(c) {
		return
	}
	id := c.Param("id")
	if id == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.versions.idRequired")
		return
	}
	req, ok := bindVersionRestoreRequest(c)
	if !ok {
		return
	}
	plan, err := h.app.VersionRestorePlan(ctx, id, req.Paths)
	if err != nil {
		writeVersionError(c, err)
		return
	}
	writeJSON(c, consts.StatusOK, plan)
}

// handleVersionRestore POST /api/versions/:id/restore — 恢复整本书或指定文件到目标版本。
func (h *Handlers) HandleVersionRestore(ctx context.Context, c *app.RequestContext) {
	if !h.requireWorkspace(c) {
		return
	}
	id := c.Param("id")
	if id == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.versions.idRequired")
		return
	}
	req, ok := bindVersionRestoreRequest(c)
	if !ok {
		return
	}
	result, err := h.app.RestoreVersion(ctx, id, req.Paths)
	if err != nil {
		writeVersionError(c, err)
		return
	}
	writeJSON(c, consts.StatusOK, result)
}

func bindVersionRestoreRequest(c *app.RequestContext) (versionRestoreRequest, bool) {
	var req versionRestoreRequest
	if len(c.Request.Body()) == 0 {
		return req, true
	}
	if err := c.BindJSON(&req); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.versions.invalidRestoreRequest")
		return versionRestoreRequest{}, false
	}
	return req, true
}

func writeVersionError(c *app.RequestContext, err error) {
	switch {
	case errors.Is(err, book.ErrVersionNotFound):
		writeError(c, consts.StatusNotFound, err.Error())
	case errors.Is(err, book.ErrVersionClean):
		writeError(c, consts.StatusBadRequest, err.Error())
	default:
		writeError(c, consts.StatusBadRequest, err.Error())
	}
}
