package handlers

import (
	"context"
	"log"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	"denova/internal/agentruntime"
	"denova/internal/api/agentui"
	"denova/internal/api/sse"
	appsvc "denova/internal/app"
)

func (h *Handlers) HandleConfigManagerStream(ctx context.Context, c *app.RequestContext) {
	var req appsvc.ConfigManagerRequest
	if err := c.BindJSON(&req); err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	if strings.TrimSpace(req.Instruction) == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.messageRequired")
		return
	}
	if strings.TrimSpace(req.CommandID) == "" {
		writeAgentRuntimeError(c, consts.StatusBadRequest, "agent_runtime.invalid_command", "缺少 command_id，无法安全重试请求 / command_id is required for safe request retries", nil)
		return
	}
	if err := agentruntime.ValidateCommandID(req.CommandID, agentruntime.DefaultInputLimits()); err != nil {
		writeAgentRuntimeError(c, consts.StatusBadRequest, "agent_runtime.invalid_command", "command_id 请求标识无效 / invalid request identifier command_id", nil)
		return
	}
	if !h.requireWorkspace(c) {
		return
	}
	req.Locale = requestLocale(c)
	task, err := h.app.StartConfigManagerTaskWithError(ctx, req)
	if err != nil {
		h.writeChatPreparationError(c, err)
		return
	}
	sse.StreamTaskUI(c, task)
}

// HandleConfigManagerTaskStream reattaches only to the exact scoped display
// Task selected by /active; a stale task ID can never cross Config scopes.
func (h *Handlers) HandleConfigManagerTaskStream(ctx context.Context, c *app.RequestContext) {
	if !h.requireWorkspace(c) {
		return
	}
	taskID := strings.TrimSpace(c.Query("task_id"))
	if taskID == "" {
		writeAgentRuntimeError(c, consts.StatusBadRequest, "agent_runtime.invalid_command", "缺少 task_id，无法精确恢复 Agent 流 / task_id is required for exact Agent stream recovery", nil)
		return
	}
	task := h.app.ConfigManagerDisplayTask(ctx, configManagerRequestFromQuery(c), taskID)
	if task == nil {
		writeAgentRuntimeError(c, consts.StatusConflict, "agent_runtime.rehydrate_required", "旧的任务流已失效，请从 active projection 重新挂接 / The old task stream is stale; rehydrate from the active projection", map[string]any{"task_id": taskID})
		return
	}
	log.Printf("[config-manager-sse] attach task_id=%s status=%s", taskID, task.Status())
	sse.StreamTaskUI(c, task)
}

func (h *Handlers) HandleConfigManagerActive(ctx context.Context, c *app.RequestContext) {
	if !h.requireWorkspace(c) {
		return
	}
	view := h.app.ConfigManagerAgentActiveView(ctx, configManagerRequestFromQuery(c))
	response := map[string]any{"active": false}
	if view.Task != nil {
		response["active"] = !view.Task.Finished
		response["status"] = view.Task.Status
		response["task_id"] = view.Task.ID
		response["command_id"] = view.CommandID
		response["stream_cursor"] = view.Task.Cursor
	}
	addAgentRuntimeProjection(response, view.Runtime, agentRuntimeProjectionOptions{
		Available: view.RuntimeProjectionOK, StreamAttached: view.StreamAttached,
	})
	writeJSON(c, consts.StatusOK, response)
}

func (h *Handlers) HandleConfigManagerRecovery(ctx context.Context, c *app.RequestContext) {
	if !h.requireWorkspace(c) {
		return
	}
	request, ok := bindAgentRecoveryRequest(c, false)
	if !ok {
		return
	}
	result, err := h.app.RecoverConfigManagerAgent(ctx, configManagerRequestFromQuery(c), request)
	if err != nil {
		h.writeAgentRecoveryError(c, err, request.Action)
		return
	}
	writeAgentRecoveryResponse(c, result)
}

func (h *Handlers) HandleConfigManagerMessages(ctx context.Context, c *app.RequestContext) {
	if !h.app.HasWorkspace() {
		writeJSON(c, consts.StatusOK, []agentui.Message{})
		return
	}
	entries, err := h.app.ConfigManagerMessages(configManagerRequestFromQuery(c))
	if err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, agentui.MessagesFromHistory(entries))
}

func (h *Handlers) HandleConfigManagerClear(ctx context.Context, c *app.RequestContext) {
	if !h.requireWorkspace(c) {
		return
	}
	if err := h.app.ClearConfigManagerSessionContext(ctx, configManagerRequestFromQuery(c)); err != nil {
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
