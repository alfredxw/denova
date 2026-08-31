package handlers

import (
	"context"
	"errors"
	"strconv"
	"strings"

	appsvc "denova/internal/app"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
)

// HandleGlobalAgentRunTraces returns a user-level Run catalog across every
// registered Project without consulting or changing the foreground Book.
func (h *Handlers) HandleGlobalAgentRunTraces(ctx context.Context, c *app.RequestContext) {
	limit, _ := strconv.Atoi(c.Query("limit"))
	target := appsvc.GlobalAgentRunTraceTarget{
		ProjectID: strings.TrimSpace(c.Query("project_id")),
		RunID:     strings.TrimSpace(c.Query("run_id")),
	}
	if (target.ProjectID == "") != (target.RunID == "") {
		target = appsvc.GlobalAgentRunTraceTarget{}
	}
	catalog, err := h.app.GlobalAgentRunTraces(ctx, limit, target)
	if err != nil {
		if errors.Is(err, appsvc.ErrDeveloperModeDisabled) {
			writeErrorKey(c, consts.StatusNotFound, "api.trajectory.disabled")
			return
		}
		writeError(c, consts.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, catalog)
}

// HandleAgentRunTraces returns recent traces for the scoped Project.
func (h *Handlers) HandleAgentRunTraces(ctx context.Context, c *app.RequestContext) {
	_ = ctx
	scope, ok := requireProjectScope(c)
	if !ok {
		return
	}
	limit, _ := strconv.Atoi(c.Query("limit"))
	traces, err := h.app.ProjectAgentRunTraces(scope.ProjectID, limit)
	if err != nil {
		writeError(c, consts.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, map[string]any{"runs": traces})
}

// HandleAgentRunTrace returns one trace from the scoped Project.
func (h *Handlers) HandleAgentRunTrace(ctx context.Context, c *app.RequestContext) {
	_ = ctx
	scope, ok := requireProjectScope(c)
	if !ok {
		return
	}
	id := c.Param("id")
	trace, err := h.app.ProjectAgentRunTrace(scope.ProjectID, id)
	if err != nil {
		writeError(c, consts.StatusNotFound, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, trace)
}

// HandleAgentRunTraceExport downloads one Project trace as JSONL.
func (h *Handlers) HandleAgentRunTraceExport(ctx context.Context, c *app.RequestContext) {
	_ = ctx
	scope, ok := requireProjectScope(c)
	if !ok {
		return
	}
	trace, err := h.app.ExportProjectAgentRunTrace(scope.ProjectID, c.Param("id"))
	if err != nil {
		writeError(c, consts.StatusNotFound, err.Error())
		return
	}
	c.Response.Header.Set("Content-Disposition", attachmentContentDisposition(trace.Filename))
	c.Response.Header.Set("Cache-Control", "no-store")
	c.Data(consts.StatusOK, "application/x-ndjson; charset=utf-8", trace.Data)
}
