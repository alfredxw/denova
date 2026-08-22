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

// HandleVersionStatus returns local version state for the scoped Book Project.
func (h *Handlers) HandleVersionStatus(ctx context.Context, c *app.RequestContext) {
	scope, ok := requireProjectScope(c)
	if !ok {
		return
	}
	status, err := h.app.ProjectVersionStatus(ctx, scope.ProjectID)
	if err != nil {
		writeVersionError(c, err)
		return
	}
	writeJSON(c, consts.StatusOK, status)
}

// HandleVersionHistory returns version history for the scoped Book Project.
func (h *Handlers) HandleVersionHistory(ctx context.Context, c *app.RequestContext) {
	scope, ok := requireProjectScope(c)
	if !ok {
		return
	}
	limit := 30
	if raw := c.Query("limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			limit = parsed
		}
	}
	versions, err := h.app.ProjectVersionHistory(ctx, scope.ProjectID, limit)
	if err != nil {
		writeVersionError(c, err)
		return
	}
	writeJSON(c, consts.StatusOK, map[string]any{"versions": versions})
}

// HandleVersionCreate creates a manual version for the scoped Book Project.
func (h *Handlers) HandleVersionCreate(ctx context.Context, c *app.RequestContext) {
	scope, ok := requireProjectScope(c)
	if !ok {
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
	result, err := h.app.CreateProjectVersion(ctx, scope.ProjectID, req.Message)
	if err != nil {
		writeVersionError(c, err)
		return
	}
	writeJSON(c, consts.StatusOK, result)
}

// HandleVersionDiff returns a Project version diff, optionally for one path.
func (h *Handlers) HandleVersionDiff(ctx context.Context, c *app.RequestContext) {
	scope, ok := requireProjectScope(c)
	if !ok {
		return
	}
	id := c.Param("id")
	if id == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.versions.idRequired")
		return
	}
	comparison := book.VersionDiffComparison(c.Query("comparison"))
	if comparison == "" {
		comparison = book.VersionDiffComparisonWorkspace
	}
	if comparison != book.VersionDiffComparisonWorkspace && comparison != book.VersionDiffComparisonParent {
		writeErrorKey(c, consts.StatusBadRequest, "api.versions.invalidDiffComparison")
		return
	}
	diff, err := h.app.ProjectVersionDiff(ctx, scope.ProjectID, id, c.Query("path"), comparison)
	if err != nil {
		writeVersionError(c, err)
		return
	}
	writeJSON(c, consts.StatusOK, diff)
}

// HandleVersionRestorePlan previews a restore against the scoped Book Project.
func (h *Handlers) HandleVersionRestorePlan(ctx context.Context, c *app.RequestContext) {
	scope, ok := requireProjectScope(c)
	if !ok {
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
	plan, err := h.app.ProjectVersionRestorePlan(ctx, scope.ProjectID, id, req.Paths)
	if err != nil {
		writeVersionError(c, err)
		return
	}
	writeJSON(c, consts.StatusOK, plan)
}

// HandleVersionRestore restores the scoped Book Project or selected paths.
func (h *Handlers) HandleVersionRestore(ctx context.Context, c *app.RequestContext) {
	scope, ok := requireProjectScope(c)
	if !ok {
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
	result, err := h.app.RestoreProjectVersion(ctx, scope.ProjectID, id, req.Paths)
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
