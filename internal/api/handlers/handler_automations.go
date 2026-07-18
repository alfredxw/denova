package handlers

import (
	"context"
	"encoding/json"
	"log"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	"denova/internal/api/agentui"
	"denova/internal/api/sse"
	"denova/internal/automation"
)

func (h *Handlers) HandleAutomations(ctx context.Context, c *app.RequestContext) {
	tasks, err := h.app.Automations()
	if err != nil {
		writeError(c, consts.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, automation.ListResult{Tasks: tasks})
}

func (h *Handlers) HandleAutomationTemplates(ctx context.Context, c *app.RequestContext) {
	writeJSON(c, consts.StatusOK, automation.TemplateListResult{
		Templates: h.app.AutomationTemplates(c.Query("locale")),
	})
}

func (h *Handlers) HandleAutomationInbox(ctx context.Context, c *app.RequestContext) {
	items, err := h.app.AutomationInbox()
	if err != nil {
		writeError(c, consts.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, automation.InboxListResult{Items: items})
}

func (h *Handlers) HandleAutomationCheck(ctx context.Context, c *app.RequestContext) {
	items, err := h.app.CheckAutomationTriggers(ctx, c.Param("id"))
	if err != nil {
		writeError(c, consts.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, automation.InboxListResult{Items: items})
}

func (h *Handlers) HandleAutomationInboxConfirm(ctx context.Context, c *app.RequestContext) {
	result, err := h.app.ConfirmAutomationInboxItem(ctx, c.Param("item_id"))
	if err != nil {
		writeError(c, consts.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, result)
}

func (h *Handlers) HandleAutomationInboxDismiss(ctx context.Context, c *app.RequestContext) {
	item, err := h.app.DismissAutomationInboxItem(c.Param("item_id"))
	if err != nil {
		writeError(c, consts.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, item)
}

func (h *Handlers) HandleAutomationInboxRead(ctx context.Context, c *app.RequestContext) {
	item, err := h.app.MarkAutomationInboxItemRead(c.Param("item_id"))
	if err != nil {
		writeError(c, consts.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, item)
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

func (h *Handlers) HandleAutomationRunStream(ctx context.Context, c *app.RequestContext) {
	var req struct {
		TriggerEvidence []automation.TriggerEvidence `json:"trigger_evidence"`
	}
	if body := c.Request.Body(); len(body) > 0 {
		if err := json.Unmarshal(body, &req); err != nil {
			writeError(c, consts.StatusBadRequest, err.Error())
			return
		}
	}
	task, run, err := h.app.StartAutomationTaskWithEvidence(ctx, c.Param("id"), automation.TriggerManual, req.TriggerEvidence)
	if err != nil {
		writeError(c, consts.StatusInternalServerError, err.Error())
		return
	}
	log.Printf("[automation-sse] attach run task_id=%s run_id=%s backend_task_id=%s", run.TaskID, run.ID, task.ID())
	sse.StreamTaskUI(c, task)
}

func (h *Handlers) HandleAutomationActiveRuns(ctx context.Context, c *app.RequestContext) {
	writeJSON(c, consts.StatusOK, automation.ActiveRunsResult{Runs: h.app.ActiveAutomationRuns()})
}

func (h *Handlers) HandleAutomationRunStreamByID(ctx context.Context, c *app.RequestContext) {
	task, run, ok := h.app.ActiveAutomationTaskByRunID(c.Param("run_id"))
	if !ok {
		writeError(c, consts.StatusNotFound, "automation run is not active")
		return
	}
	log.Printf("[automation-sse] attach active run task_id=%s run_id=%s backend_task_id=%s", run.TaskID, run.ID, task.ID())
	sse.StreamTaskUI(c, task)
}

func (h *Handlers) HandleAutomationRunChatStream(ctx context.Context, c *app.RequestContext) {
	var req struct {
		Message string `json:"message"`
	}
	if err := c.BindJSON(&req); err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	if strings.TrimSpace(req.Message) == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.messageRequired")
		return
	}
	task, run, err := h.app.ContinueAutomationRun(ctx, c.Param("run_id"), req.Message)
	if err != nil {
		writeError(c, consts.StatusInternalServerError, err.Error())
		return
	}
	log.Printf("[automation-sse] attach run follow-up task_id=%s run_id=%s backend_task_id=%s", run.TaskID, run.ID, task.ID())
	sse.StreamTaskUI(c, task)
}

func (h *Handlers) HandleAutomationRunAbort(ctx context.Context, c *app.RequestContext) {
	if !h.app.AbortAutomationRun(c.Param("run_id")) {
		writeError(c, consts.StatusNotFound, "automation run is not active")
		return
	}
	writeJSON(c, consts.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handlers) HandleAutomationRunMessages(ctx context.Context, c *app.RequestContext) {
	entries, err := h.app.AutomationRunMessages(c.Param("run_id"))
	if err != nil {
		writeError(c, consts.StatusNotFound, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, agentui.MessagesFromHistory(entries))
}
