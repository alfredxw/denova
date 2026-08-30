package handlers

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	agentchat "denova/internal/agents/chat"
	"denova/internal/api/sse"
	novaApp "denova/internal/app"
	workspacechange "denova/internal/workspace/change"
)

// handleChat 处理聊天请求：启动后台 Task，然后以 AI SDK UIMessage stream 订阅事件。
func (h *Handlers) HandleChat(ctx context.Context, c *app.RequestContext) {
	if !h.requireWorkspace(c) {
		return
	}
	var body struct {
		novaApp.AgentChatRequest
		SessionID string `json:"session_id"`
	}
	if err := c.BindJSON(&body); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidBody")
		return
	}
	req := body.AgentChatRequest
	sessionID, ok := requiredWritingSessionID(c, body.SessionID)
	if !ok {
		return
	}
	if strings.TrimSpace(req.Message) == "" && len(req.AttachmentUploads) == 0 {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.messageRequired")
		return
	}
	if strings.TrimSpace(req.CommandID) == "" {
		writeAgentRuntimeError(c, consts.StatusBadRequest, "agent_runtime.invalid_command", "缺少 command_id，无法安全重试请求 / command_id is required for safe request retries", nil)
		return
	}
	if err := h.app.MaterializeWritingAttachments(sessionID, req.CommandID, &req); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
		return
	}
	req.Locale = requestLocale(c)

	task, err := h.app.StartTaskForSessionWithError(ctx, sessionID, req)
	if err != nil {
		h.writeChatPreparationError(c, err)
		return
	}
	slog.InfoContext(ctx, fmt.Sprintf("[agent-ui-sse] attach new chat task_id=%s session_id=%s", task.ID(), sessionID))
	sse.StreamTaskUI(ctx, c, task)
}

// HandleChatContextAnalysis 模拟一次聊天请求，返回真实 SystemPrompt 和上下文组成，不启动 LLM。
func (h *Handlers) HandleChatContextAnalysis(ctx context.Context, c *app.RequestContext) {
	if !h.requireWorkspace(c) {
		return
	}
	var req novaApp.AgentChatRequest
	if err := c.BindJSON(&req); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidBody")
		return
	}
	if strings.TrimSpace(req.Message) == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.messageRequired")
		return
	}
	req.Locale = requestLocale(c)
	analysis, err := h.app.AnalyzeContext(ctx, req)
	if err != nil {
		h.writeChatPreparationError(c, err)
		return
	}
	c.JSON(consts.StatusOK, analysis)
}

func (h *Handlers) writeChatPreparationError(c *app.RequestContext, err error) {
	if errors.Is(err, novaApp.ErrAgentCommandIDRequired) {
		writeAgentRuntimeError(c, consts.StatusBadRequest, "agent_runtime.invalid_command", "缺少 command_id，无法安全重试请求 / command_id is required for safe request retries", nil)
		return
	}
	if errors.Is(err, novaApp.ErrInvalidAgentCommand) {
		writeAgentRuntimeError(c, consts.StatusBadRequest, "agent_runtime.invalid_command", err.Error(), nil)
		return
	}
	if errors.Is(err, novaApp.ErrAgentCommandConflict) {
		writeAgentRuntimeError(c, consts.StatusConflict, "agent_runtime.command_conflict", "command_id 已用于其他请求 / command_id was already used for a different request", nil)
		return
	}
	if errors.Is(err, novaApp.ErrAgentOperationActive) {
		writeAgentRuntimeError(c, consts.StatusConflict, "agent_runtime.busy", "已有 Agent 正在运行，请使用 Follow Up、Steer 或 Stop / An agent is already running; use Follow Up, Steer, or Stop", nil)
		return
	}
	if errors.Is(err, agentchat.ErrInterruptionNotPending) {
		writeAgentRuntimeError(c, consts.StatusConflict, "agent_runtime.interruption_changed", "暂停点已变化，请刷新后重试 / The paused turn changed; refresh and try again", nil)
		return
	}
	if errors.Is(err, novaApp.ErrWorkspaceTransition) || errors.Is(err, novaApp.ErrAgentContextChanged) {
		writeAgentRuntimeError(c, consts.StatusConflict, "agent_runtime.context_changed", "运行上下文已变化，请重试 / The agent context changed; retry the request", nil)
		return
	}
	if errors.Is(err, novaApp.ErrNoWorkspace) {
		writeErrorKey(c, consts.StatusConflict, "api.workspace.noWorkspace")
		return
	}
	if errors.Is(err, novaApp.ErrWorkspaceChanged) {
		h.writeWorkspaceChangeLeaseError(c, "", err)
		return
	}
	var changeErr *workspacechange.Error
	if errors.As(err, &changeErr) {
		writeWorkspaceChangeError(c, err)
		return
	}
	writeError(c, consts.StatusInternalServerError, err.Error())
}

func (h *Handlers) HandleChatContextCompaction(ctx context.Context, c *app.RequestContext) {
	if !h.requireWorkspace(c) {
		return
	}
	var body struct {
		CommandID string `json:"command_id"`
	}
	if err := c.BindJSON(&body); err != nil {
		writeAgentRuntimeError(c, consts.StatusBadRequest, "agent_runtime.invalid_command", "命令格式无效 / Invalid compaction command", nil)
		return
	}
	body.CommandID = strings.TrimSpace(body.CommandID)
	if err := novaApp.ValidateAgentCommandID(body.CommandID); err != nil {
		writeAgentRuntimeError(c, consts.StatusBadRequest, "agent_runtime.invalid_command", "缺少或无效的 command_id，无法安全重试 / command_id is required and must be valid for safe retries", nil)
		return
	}
	result, err := h.app.CompactContextCommand(ctx, body.CommandID)
	if err != nil {
		writeError(c, consts.StatusConflict, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, result)
}

func (h *Handlers) HandleChatContextCompactionRemove(ctx context.Context, c *app.RequestContext) {
	if !h.requireWorkspace(c) {
		return
	}
	commandID := strings.TrimSpace(c.Query("command_id"))
	if err := novaApp.ValidateAgentCommandID(commandID); err != nil {
		writeAgentRuntimeError(c, consts.StatusBadRequest, "agent_runtime.invalid_command", "缺少或无效的 command_id，无法安全重试 / command_id is required and must be valid for safe retries", nil)
		return
	}
	removed, err := h.app.RemoveContextCompactionCommand(ctx, commandID)
	if err != nil {
		writeError(c, consts.StatusConflict, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, map[string]bool{"removed": removed})
}

// handleChatStream 重连到当前活跃任务的 UIMessage 事件流（回放已有事件 + 继续接收新事件）。
func (h *Handlers) HandleChatStream(ctx context.Context, c *app.RequestContext) {
	sessionID, ok := requiredWritingSessionID(c, c.Query("session_id"))
	if !ok {
		return
	}
	taskID := strings.TrimSpace(c.Query("task_id"))
	if taskID == "" {
		writeError(c, consts.StatusBadRequest, "缺少 task_id，无法精确恢复 Agent 流 / task_id is required for exact Agent stream recovery")
		return
	}
	task, err := h.app.ActiveTaskForSession(sessionID)
	if err != nil {
		h.writeChatPreparationError(c, err)
		return
	}
	if task == nil || task.ID() != taskID {
		writeAgentRuntimeError(c, consts.StatusConflict, "agent_runtime.rehydrate_required", "旧的任务流已失效，请从 active projection 重新挂接 / The old task stream is stale; rehydrate from the active projection", map[string]any{"task_id": taskID})
		return
	}
	slog.InfoContext(ctx, fmt.Sprintf("[agent-ui-sse] attach active chat task_id=%s session_id=%s status=%s", task.ID(), sessionID, task.Status()))
	sse.StreamTaskUI(ctx, c, task)
}

// handleChatActive 查询当前是否有活跃任务。
func (h *Handlers) HandleChatActive(ctx context.Context, c *app.RequestContext) {
	sessionID, ok := requiredWritingSessionID(c, c.Query("session_id"))
	if !ok {
		return
	}
	view := h.app.WritingAgentActiveView(ctx)
	if strings.TrimSpace(view.SessionID) != sessionID {
		h.writeChatPreparationError(c, novaApp.ErrAgentContextChanged)
		return
	}
	if view.Task == nil {
		response := map[string]interface{}{
			"active": false,
		}
		if view.PendingAsk != nil {
			response["pending_ask"] = view.PendingAsk
		}
		if view.PendingInterruptionID != "" {
			response["pending_interruption_id"] = view.PendingInterruptionID
		}
		addAgentRuntimeProjection(response, view.Runtime, agentRuntimeProjectionOptions{
			Available: view.RuntimeProjectionOK, RecoveryActions: view.RecoveryActions,
		})
		c.JSON(consts.StatusOK, response)
		return
	}
	response := map[string]interface{}{
		"active":        !view.Task.Finished,
		"status":        view.Task.Status,
		"task_id":       view.Task.ID,
		"stream_cursor": view.Task.Cursor,
	}
	if view.PendingAsk != nil {
		response["pending_ask"] = view.PendingAsk
	}
	if view.PendingInterruptionID != "" {
		response["pending_interruption_id"] = view.PendingInterruptionID
	}
	addAgentRuntimeProjection(response, view.Runtime, agentRuntimeProjectionOptions{
		Available: view.RuntimeProjectionOK, StreamAttached: true, RecoveryActions: view.RecoveryActions,
	})
	c.JSON(consts.StatusOK, response)
}

func requiredWritingSessionID(c *app.RequestContext, value string) (string, bool) {
	sessionID := strings.TrimSpace(value)
	if sessionID == "" {
		writeAgentRuntimeError(c, consts.StatusBadRequest, "agent_runtime.invalid_binding", "缺少 session_id，无法绑定创作会话 / session_id is required to bind the Writing session", nil)
		return "", false
	}
	return sessionID, true
}
