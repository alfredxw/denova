package api

import (
	"context"
	"errors"
	"strconv"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	"nova/internal/book"
)

// handleVersionStatus GET /api/versions/status — 返回当前书籍原生版本状态。
func (s *Server) handleVersionStatus(ctx context.Context, c *app.RequestContext) {
	if !s.requireWorkspace(c) {
		return
	}
	status, err := s.app.VersionStatus(ctx)
	if err != nil {
		writeVersionError(c, err)
		return
	}
	writeJSON(c, consts.StatusOK, status)
}

// handleVersionHistory GET /api/versions?limit=30 — 返回版本历史。
func (s *Server) handleVersionHistory(ctx context.Context, c *app.RequestContext) {
	if !s.requireWorkspace(c) {
		return
	}
	limit := 30
	if raw := c.Query("limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			limit = parsed
		}
	}
	versions, err := s.app.VersionHistory(ctx, limit)
	if err != nil {
		writeVersionError(c, err)
		return
	}
	writeJSON(c, consts.StatusOK, map[string]any{"versions": versions})
}

// handleVersionCreate POST /api/versions — 创建手动版本。
func (s *Server) handleVersionCreate(ctx context.Context, c *app.RequestContext) {
	if !s.requireWorkspace(c) {
		return
	}
	var req struct {
		Message string `json:"message"`
	}
	if err := c.BindJSON(&req); err != nil || req.Message == "" {
		writeError(c, consts.StatusBadRequest, "请提供版本说明")
		return
	}
	result, err := s.app.CreateVersion(ctx, req.Message)
	if err != nil {
		writeVersionError(c, err)
		return
	}
	writeJSON(c, consts.StatusOK, result)
}

// handleVersionDiff GET /api/versions/:id/diff?path=optional — 返回版本差异。
func (s *Server) handleVersionDiff(ctx context.Context, c *app.RequestContext) {
	if !s.requireWorkspace(c) {
		return
	}
	id := c.Param("id")
	if id == "" {
		writeError(c, consts.StatusBadRequest, "请提供版本 ID")
		return
	}
	diff, err := s.app.VersionDiff(ctx, id, c.Query("path"))
	if err != nil {
		writeVersionError(c, err)
		return
	}
	writeJSON(c, consts.StatusOK, diff)
}

// handleVersionRestore POST /api/versions/:id/restore — 恢复整本书到指定版本。
func (s *Server) handleVersionRestore(ctx context.Context, c *app.RequestContext) {
	if !s.requireWorkspace(c) {
		return
	}
	id := c.Param("id")
	if id == "" {
		writeError(c, consts.StatusBadRequest, "请提供版本 ID")
		return
	}
	result, err := s.app.RestoreVersion(ctx, id)
	if err != nil {
		writeVersionError(c, err)
		return
	}
	writeJSON(c, consts.StatusOK, result)
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
