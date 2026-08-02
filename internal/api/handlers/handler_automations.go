package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	"denova/internal/api/agentui"
	"denova/internal/api/sse"
	appsvc "denova/internal/app"
	automationapp "denova/internal/app/automation"
	"denova/internal/automation"
)

func (h *Handlers) HandleAutomations(ctx context.Context, c *app.RequestContext) {
	tasks, err := h.app.Automation().List()
	if err != nil {
		writeError(c, consts.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, automation.ListResult{Tasks: tasks})
}

func (h *Handlers) HandleAutomationTemplates(ctx context.Context, c *app.RequestContext) {
	writeJSON(c, consts.StatusOK, automation.TemplateListResult{
		Templates: h.app.Automation().Templates(c.Query("locale")),
	})
}

func (h *Handlers) HandleAutomationInbox(ctx context.Context, c *app.RequestContext) {
	items, err := h.app.Automation().Inbox()
	if err != nil {
		writeError(c, consts.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, automation.InboxListResult{Items: items})
}

func (h *Handlers) HandleAutomationCheck(ctx context.Context, c *app.RequestContext) {
	items, err := h.app.Automation().CheckTriggers(ctx, c.Param("id"))
	if err != nil {
		writeError(c, consts.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, automation.InboxListResult{Items: items})
}

func (h *Handlers) HandleAutomationInboxConfirm(ctx context.Context, c *app.RequestContext) {
	result, err := h.app.Automation().ConfirmInboxItem(ctx, c.Param("item_id"))
	if err != nil {
		writeError(c, consts.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, result)
}

func (h *Handlers) HandleAutomationInboxDismiss(ctx context.Context, c *app.RequestContext) {
	item, err := h.app.Automation().DismissInboxItem(c.Param("item_id"))
	if err != nil {
		writeError(c, consts.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, item)
}

func (h *Handlers) HandleAutomationInboxRead(ctx context.Context, c *app.RequestContext) {
	item, err := h.app.Automation().MarkInboxItemRead(c.Param("item_id"))
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
	task, err := h.app.Automation().Create(req)
	if err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, task)
}

func (h *Handlers) HandleAutomationUpdate(ctx context.Context, c *app.RequestContext) {
	id := c.Param("id")
	var req automation.Task
	body := c.Request.Body()
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	var metadata struct {
		BaseRevision string `json:"base_revision"`
	}
	if err := json.Unmarshal(body, &metadata); err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	baseRevision := strings.TrimSpace(metadata.BaseRevision)
	if baseRevision == "" {
		writeJSON(c, consts.StatusBadRequest, map[string]any{
			"error": messageKey(c, "api.automation.baseRevisionRequired"),
			"code":  "base_revision_required",
		})
		return
	}
	task, err := h.app.Automation().UpdateIfRevision(id, req, baseRevision)
	if err != nil {
		if errors.Is(err, automation.ErrRevisionConflict) {
			writeJSON(c, consts.StatusConflict, map[string]any{
				"error": messageKey(c, "api.resource.revisionConflict"),
				"code":  "revision_conflict",
			})
			return
		}
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, task)
}

func (h *Handlers) HandleAutomationDelete(ctx context.Context, c *app.RequestContext) {
	if err := h.app.Automation().Delete(c.Param("id")); err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, map[string]string{"message": "deleted"})
}

func (h *Handlers) HandleAutomationRunStream(ctx context.Context, c *app.RequestContext) {
	var req struct {
		CommandID       string                       `json:"command_id"`
		TriggerEvidence []automation.TriggerEvidence `json:"trigger_evidence"`
	}
	if body := c.Request.Body(); len(body) > 0 {
		if err := json.Unmarshal(body, &req); err != nil {
			writeError(c, consts.StatusBadRequest, err.Error())
			return
		}
	}
	if strings.TrimSpace(req.CommandID) == "" {
		writeAgentRuntimeError(c, consts.StatusBadRequest, "agent_runtime.invalid_command", "缺少 command_id，无法安全重试自动化运行 / command_id is required for safe automation retries", nil)
		return
	}
	task, run, err := h.app.Automation().StartTaskCommand(ctx, c.Param("id"), req.CommandID, req.TriggerEvidence)
	if err != nil {
		if errors.Is(err, appsvc.ErrInvalidAgentCommand) {
			writeAgentRuntimeError(c, consts.StatusBadRequest, "agent_runtime.invalid_command", err.Error(), nil)
			return
		}
		if errors.Is(err, automation.ErrRunIdentityConflict) || errors.Is(err, automationapp.ErrCommandConflict) {
			writeAgentRuntimeError(c, consts.StatusConflict, "agent_runtime.command_conflict", "command_id 已用于不同的自动化请求 / command_id was already used for a different automation request", nil)
			return
		}
		writeError(c, consts.StatusInternalServerError, err.Error())
		return
	}
	slog.InfoContext(ctx, fmt.Sprintf("[automation-sse] attach run task_id=%s run_id=%s backend_task_id=%s", run.TaskID, run.ID, task.ID()))
	sse.StreamTaskUI(ctx, c, task)
}

func (h *Handlers) HandleAutomationActiveRuns(ctx context.Context, c *app.RequestContext) {
	writeJSON(c, consts.StatusOK, automation.ActiveRunsResult{Runs: h.app.Automation().ActiveAutomationRuns()})
}

func (h *Handlers) HandleAutomationRunStreamByID(ctx context.Context, c *app.RequestContext) {
	task, run, ok := h.app.Automation().ActiveAutomationTaskByRunID(c.Param("run_id"))
	if !ok {
		writeError(c, consts.StatusNotFound, "automation run is not active")
		return
	}
	slog.InfoContext(ctx, fmt.Sprintf("[automation-sse] attach active run task_id=%s run_id=%s backend_task_id=%s", run.TaskID, run.ID, task.ID()))
	sse.StreamTaskUI(ctx, c, task)
}

func (h *Handlers) HandleAutomationRunChatStream(ctx context.Context, c *app.RequestContext) {
	var req struct {
		CommandID string `json:"command_id"`
		Message   string `json:"message"`
	}
	if err := c.BindJSON(&req); err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	if strings.TrimSpace(req.Message) == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.messageRequired")
		return
	}
	if strings.TrimSpace(req.CommandID) == "" {
		writeAgentRuntimeError(c, consts.StatusBadRequest, "agent_runtime.invalid_command", "缺少 command_id，无法安全重试自动化追问 / command_id is required for safe automation follow-up retries", nil)
		return
	}
	task, run, err := h.app.Automation().ContinueRun(ctx, c.Param("run_id"), req.CommandID, req.Message)
	if err != nil {
		if errors.Is(err, appsvc.ErrInvalidAgentCommand) {
			writeAgentRuntimeError(c, consts.StatusBadRequest, "agent_runtime.invalid_command", err.Error(), nil)
			return
		}
		if errors.Is(err, automationapp.ErrCommandConflict) {
			writeAgentRuntimeError(c, consts.StatusConflict, "agent_runtime.command_conflict", "command_id 已用于不同的自动化追问 / command_id was already used for a different automation follow-up", nil)
			return
		}
		if errors.Is(err, automationapp.ErrOperationActive) {
			writeAgentRuntimeError(c, consts.StatusConflict, "agent_runtime.busy", "自动化运行已有活动操作 / The automation run already has an active operation", nil)
			return
		}
		writeError(c, consts.StatusInternalServerError, err.Error())
		return
	}
	slog.InfoContext(ctx, fmt.Sprintf("[automation-sse] attach run follow-up task_id=%s run_id=%s backend_task_id=%s", run.TaskID, run.ID, task.ID()))
	sse.StreamTaskUI(ctx, c, task)
}

func (h *Handlers) HandleAutomationRunAbort(ctx context.Context, c *app.RequestContext) {
	var req struct {
		CommandID         string `json:"command_id"`
		TargetOperationID string `json:"target_operation_id"`
		Reason            string `json:"reason,omitempty"`
	}
	if err := c.BindJSON(&req); err != nil {
		writeAgentRuntimeError(c, consts.StatusBadRequest, "agent_runtime.invalid_command", "命令格式无效 / Invalid agent command", nil)
		return
	}
	if strings.TrimSpace(req.CommandID) == "" || strings.TrimSpace(req.TargetOperationID) == "" {
		writeAgentRuntimeError(c, consts.StatusBadRequest, "agent_runtime.invalid_command", "command_id 和 target_operation_id 为必填项 / command_id and target_operation_id are required", nil)
		return
	}
	receipt, err := h.app.Automation().AbortRunCommand(
		ctx, c.Param("run_id"), req.CommandID,
		appsvc.AgentOperationID(strings.TrimSpace(req.TargetOperationID)), req.Reason,
	)
	if err != nil {
		h.writeAgentCommandError(c, err, req.TargetOperationID)
		return
	}
	c.JSON(consts.StatusAccepted, agentCommandReceiptResponse{
		CommandID: string(receipt.CommandID), OperationID: string(receipt.OperationID), Cursor: uint64(receipt.Cursor),
	})
}

func (h *Handlers) HandleAutomationRunMessages(ctx context.Context, c *app.RequestContext) {
	entries, err := h.app.Automation().AutomationRunMessages(c.Param("run_id"))
	if err != nil {
		writeError(c, consts.StatusNotFound, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, agentui.MessagesFromHistory(entries))
}
