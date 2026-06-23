package handlers

import (
	"context"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	"nova/internal/api/sse"
	appsvc "nova/internal/app"
)

func (h *Handlers) HandleConfigManagerStream(ctx context.Context, c *app.RequestContext) {
	if !h.requireWorkspace(c) {
		return
	}
	var req appsvc.ConfigManagerRequest
	if err := c.BindJSON(&req); err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	if strings.TrimSpace(req.Instruction) == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.messageRequired")
		return
	}
	task := h.app.StartConfigManagerTask(req)
	if task == nil {
		writeError(c, consts.StatusInternalServerError, "config manager agent is unavailable")
		return
	}
	sse.StreamTask(c, task)
}

func (h *Handlers) HandleConfigManagerMessages(ctx context.Context, c *app.RequestContext) {
	if !h.app.HasWorkspace() {
		writeJSON(c, consts.StatusOK, []messageDTO{})
		return
	}
	entries, err := h.app.ConfigManagerMessages(configManagerRequestFromQuery(c))
	if err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, historyEntriesToMessageDTOs(entries))
}

func (h *Handlers) HandleConfigManagerClear(ctx context.Context, c *app.RequestContext) {
	if !h.requireWorkspace(c) {
		return
	}
	if err := h.app.ClearConfigManagerSession(configManagerRequestFromQuery(c)); err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, map[string]string{"status": "ok"})
}

func configManagerRequestFromQuery(c *app.RequestContext) appsvc.ConfigManagerRequest {
	return appsvc.ConfigManagerRequest{
		Origin:     strings.TrimSpace(c.Query("origin")),
		ResourceID: strings.TrimSpace(c.Query("resource_id")),
		StoryID:    strings.TrimSpace(c.Query("story_id")),
		BranchID:   strings.TrimSpace(c.Query("branch_id")),
	}
}
