package handlers

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	"nova/internal/automation"
)

func (h *Handlers) HandleAutomations(ctx context.Context, c *app.RequestContext) {
	tasks, err := h.app.Automations()
	if err != nil {
		writeError(c, consts.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, automation.ListResult{Tasks: tasks})
}

func (h *Handlers) HandleAutomationCreate(ctx context.Context, c *app.RequestContext) {
	var req automation.Task
	if err := c.BindJSON(&req); err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	task, err := h.app.CreateAutomation(req)
	if err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, task)
}

func (h *Handlers) HandleAutomationUpdate(ctx context.Context, c *app.RequestContext) {
	id := c.Param("id")
	var req automation.Task
	if err := c.BindJSON(&req); err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	task, err := h.app.UpdateAutomation(id, req)
	if err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, task)
}

func (h *Handlers) HandleAutomationDelete(ctx context.Context, c *app.RequestContext) {
	if err := h.app.DeleteAutomation(c.Param("id")); err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, map[string]string{"message": "deleted"})
}

func (h *Handlers) HandleAutomationRun(ctx context.Context, c *app.RequestContext) {
	result, err := h.app.RunAutomation(ctx, c.Param("id"), automation.TriggerManual)
	if err != nil {
		writeError(c, consts.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, result)
}
