package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	appsvc "denova/internal/app"
	automationapp "denova/internal/app/automation"
	"denova/internal/automation"
)

func (h *Handlers) HandleAutomations(ctx context.Context, c *app.RequestContext) {
	projectID := strings.TrimSpace(c.Query("project_id"))
	workspace := strings.TrimSpace(c.Query("workspace"))
	if projectID == "" && workspace == "" {
		writeError(c, consts.StatusBadRequest, "自动化目录需要项目范围 / Automation catalog requires a Project target")
		return
	}
	tasks, err := h.app.Automation().ListForProject(projectID, workspace)
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
	projectID := strings.TrimSpace(c.Query("project_id"))
	workspace := strings.TrimSpace(c.Query("workspace"))
	if projectID == "" && workspace == "" {
		writeError(c, consts.StatusBadRequest, "自动化收件箱需要项目范围 / Automation inbox requires a Project target")
		return
	}
	items, err := h.app.Automation().InboxForProject(projectID, workspace)
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
	var req automation.TaskDefinition
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

type automationRunRequest struct {
	CommandID       string                       `json:"command_id"`
	TriggerEvidence []automation.TriggerEvidence `json:"trigger_evidence"`
}

func (h *Handlers) startAutomationRun(ctx context.Context, c *app.RequestContext) (automation.RunRecord, bool) {
	var req automationRunRequest
	if body := c.Request.Body(); len(body) > 0 {
		if err := json.Unmarshal(body, &req); err != nil {
			writeError(c, consts.StatusBadRequest, err.Error())
			return automation.RunRecord{}, false
		}
	}
	if strings.TrimSpace(req.CommandID) == "" {
		writeAgentRuntimeError(c, consts.StatusBadRequest, "agent_runtime.invalid_command", "缺少 command_id，无法安全重试自动化运行 / command_id is required for safe automation retries", nil)
		return automation.RunRecord{}, false
	}
	_, run, err := h.app.Automation().StartTaskCommand(ctx, c.Param("id"), req.CommandID, req.TriggerEvidence)
	if err != nil {
		if errors.Is(err, appsvc.ErrInvalidAgentCommand) {
			writeAgentRuntimeError(c, consts.StatusBadRequest, "agent_runtime.invalid_command", err.Error(), nil)
			return automation.RunRecord{}, false
		}
		if errors.Is(err, automation.ErrRunIdentityConflict) || errors.Is(err, automationapp.ErrCommandConflict) {
			writeAgentRuntimeError(c, consts.StatusConflict, "agent_runtime.command_conflict", "command_id 已用于不同的自动化请求 / command_id was already used for a different automation request", nil)
			return automation.RunRecord{}, false
		}
		writeError(c, consts.StatusInternalServerError, err.Error())
		return automation.RunRecord{}, false
	}
	return run, true
}

// HandleAutomationRun starts the project-Agent conversation in the background
// and returns its durable navigation identity. Execution is observed and
// controlled through AgentChat, not through an Automation-only chat surface.
func (h *Handlers) HandleAutomationRun(ctx context.Context, c *app.RequestContext) {
	run, ok := h.startAutomationRun(ctx, c)
	if !ok {
		return
	}
	writeJSON(c, consts.StatusAccepted, map[string]automation.RunRecord{"run": run})
}
