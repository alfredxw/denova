package handlers

import (
	"context"
	"log"
	"strconv"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	"denova/internal/api/agentui"
	"denova/internal/api/sse"
	appsvc "denova/internal/app"
)

type agentChatSessionCreateRequest struct {
	Workspace string `json:"workspace"`
	Title     string `json:"title"`
}

type agentChatSessionRequest struct {
	Workspace string `json:"workspace"`
	SessionID string `json:"session_id"`
}

type agentChatSessionRenameRequest struct {
	Workspace string `json:"workspace"`
	SessionID string `json:"session_id"`
	Title     string `json:"title"`
}

type agentChatTurnRequest struct {
	Workspace string `json:"workspace"`
	SessionID string `json:"session_id"`
	appsvc.AgentChatRequest
}

func agentChatBinding(workspace, sessionID string) appsvc.AgentChatBinding {
	return appsvc.AgentChatBinding{
		Workspace: strings.TrimSpace(workspace), SessionID: strings.TrimSpace(sessionID),
	}
}

func agentChatBindingFromQuery(c *app.RequestContext) appsvc.AgentChatBinding {
	return agentChatBinding(c.Query("workspace"), c.Query("session_id"))
}

// HandleAgentChatProjects lists every project and never switches the open book.
func (h *Handlers) HandleAgentChatProjects(_ context.Context, c *app.RequestContext) {
	writeJSON(c, consts.StatusOK, map[string]any{"projects": h.app.AgentChatProjects()})
}

func (h *Handlers) HandleAgentChatSessionCreate(_ context.Context, c *app.RequestContext) {
	var req agentChatSessionCreateRequest
	if err := c.BindJSON(&req); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidBody")
		return
	}
	sess, err := h.app.CreateProjectSession(req.Workspace, strings.TrimSpace(req.Title))
	if err != nil {
		log.Printf("[api/handlers/handler_agent_chat.go] creating session failed workspace=%q err=%v", req.Workspace, err)
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, sess)
}

func (h *Handlers) HandleAgentChatSessionRename(_ context.Context, c *app.RequestContext) {
	var req agentChatSessionRenameRequest
	if err := c.BindJSON(&req); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidBody")
		return
	}
	if strings.TrimSpace(req.SessionID) == "" || strings.TrimSpace(req.Title) == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidBody")
		return
	}
	if err := h.app.RenameProjectSession(req.Workspace, req.SessionID, req.Title); err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, map[string]any{"session_id": req.SessionID, "title": req.Title})
}

func (h *Handlers) HandleAgentChatSessionDelete(_ context.Context, c *app.RequestContext) {
	var req agentChatSessionRequest
	if err := c.BindJSON(&req); err != nil || strings.TrimSpace(req.SessionID) == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidBody")
		return
	}
	if err := h.app.DeleteProjectSession(req.Workspace, req.SessionID); err != nil {
		writeError(c, consts.StatusConflict, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, map[string]any{"session_id": req.SessionID, "deleted": true})
}

// HandleAgentChat starts a project-scoped task. Neither workspace nor session
// is copied into the foreground Writing runtime.
func (h *Handlers) HandleAgentChat(ctx context.Context, c *app.RequestContext) {
	var req agentChatTurnRequest
	if err := c.BindJSON(&req); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidBody")
		return
	}
	if strings.TrimSpace(req.Message) == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.messageRequired")
		return
	}
	if strings.TrimSpace(req.CommandID) == "" {
		writeAgentRuntimeError(c, consts.StatusBadRequest, "agent_runtime.invalid_command", "缺少 command_id，无法安全重试请求 / command_id is required for safe request retries", nil)
		return
	}
	req.Locale = requestLocale(c)
	task, err := h.app.StartAgentChatTask(ctx, agentChatBinding(req.Workspace, req.SessionID), req.AgentChatRequest)
	if err != nil {
		h.writeChatPreparationError(c, err)
		return
	}
	log.Printf("[agent-chat-sse] attach new task_id=%s workspace=%q session_id=%s", task.ID(), req.Workspace, req.SessionID)
	sse.StreamTaskUI(c, task, h.chatSSEStreamOptions()...)
}

func (h *Handlers) HandleAgentChatStream(_ context.Context, c *app.RequestContext) {
	taskID := strings.TrimSpace(c.Query("task_id"))
	if taskID == "" {
		writeAgentRuntimeError(c, consts.StatusBadRequest, "agent_runtime.invalid_command", "缺少 task_id，无法精确恢复 Agent 流 / task_id is required for exact Agent stream recovery", nil)
		return
	}
	task := h.app.AgentChatDisplayTask(agentChatBindingFromQuery(c), taskID)
	if task == nil {
		writeAgentRuntimeError(c, consts.StatusConflict, "agent_runtime.rehydrate_required", "旧的任务流已失效，请从 active projection 重新挂接 / The old task stream is stale; rehydrate from the active projection", map[string]any{"task_id": taskID})
		return
	}
	sse.StreamTaskUI(c, task, h.chatSSEStreamOptions()...)
}

func (h *Handlers) HandleAgentChatActive(ctx context.Context, c *app.RequestContext) {
	view := h.app.AgentChatActiveView(ctx, agentChatBindingFromQuery(c))
	response := map[string]any{"active": false}
	if view.Task != nil {
		response["active"] = !view.Task.Finished
		response["status"] = view.Task.Status
		response["task_id"] = view.Task.ID
		response["stream_cursor"] = view.Task.Cursor
	}
	if view.PendingAsk != nil {
		response["pending_ask"] = view.PendingAsk
	}
	addAgentRuntimeProjection(response, view.Runtime, agentRuntimeProjectionOptions{
		Available: view.RuntimeProjectionOK, StreamAttached: view.StreamAttached,
	})
	writeJSON(c, consts.StatusOK, response)
}

func (h *Handlers) HandleAgentChatCommand(ctx context.Context, c *app.RequestContext) {
	var body struct {
		Workspace         string                  `json:"workspace"`
		SessionID         string                  `json:"session_id"`
		Type              string                  `json:"type"`
		CommandID         string                  `json:"command_id"`
		TargetOperationID string                  `json:"target_operation_id"`
		TargetCommandID   string                  `json:"target_command_id,omitempty"`
		Input             appsvc.AgentChatRequest `json:"input"`
		Reason            string                  `json:"reason,omitempty"`
	}
	if err := c.BindJSON(&body); err != nil {
		writeAgentRuntimeError(c, consts.StatusBadRequest, "agent_runtime.invalid_command", "命令格式无效 / Invalid agent command", nil)
		return
	}
	kind, err := writingAgentCommandKind(body.Type)
	queueControl := kind == appsvc.AgentCommandSteerQueued || kind == appsvc.AgentCommandCancelQueued
	if err != nil || strings.TrimSpace(body.CommandID) == "" || strings.TrimSpace(body.TargetOperationID) == "" ||
		(queueControl && strings.TrimSpace(body.TargetCommandID) == "") ||
		(kind != appsvc.AgentCommandAbort && !queueControl && strings.TrimSpace(body.Input.Message) == "") {
		writeAgentRuntimeError(c, consts.StatusBadRequest, "agent_runtime.invalid_command", "AgentChat 命令 identity 或消息不完整 / AgentChat command identity or message is incomplete", nil)
		return
	}
	body.Input.Locale = requestLocale(c)
	receipt, err := h.app.SubmitAgentChatCommand(ctx, agentChatBinding(body.Workspace, body.SessionID), appsvc.ChatAgentCommand{
		Kind: kind, CommandID: strings.TrimSpace(body.CommandID),
		OperationID:     appsvc.AgentOperationID(strings.TrimSpace(body.TargetOperationID)),
		TargetCommandID: appsvc.AgentCommandID(strings.TrimSpace(body.TargetCommandID)),
		Reason:          body.Reason, Input: body.Input,
	})
	if err != nil {
		h.writeAgentCommandError(c, err, body.TargetOperationID)
		return
	}
	c.JSON(consts.StatusAccepted, agentCommandReceiptResponse{
		CommandID: string(receipt.CommandID), OperationID: string(receipt.OperationID), Cursor: uint64(receipt.Cursor),
	})
}

func (h *Handlers) HandleAgentChatRecovery(ctx context.Context, c *app.RequestContext) {
	var body struct {
		Workspace string                     `json:"workspace"`
		SessionID string                     `json:"session_id"`
		Action    agentRecoveryActionRequest `json:"action"`
	}
	if err := c.BindJSON(&body); err != nil {
		writeAgentRuntimeError(c, consts.StatusBadRequest, "agent_runtime.invalid_recovery", "恢复请求格式无效 / Invalid recovery request", nil)
		return
	}
	action := appsvc.AgentRuntimeRecoveryAction{
		Kind:        appsvc.AgentRuntimeRecoveryActionKind(strings.TrimSpace(body.Action.Kind)),
		CommandID:   appsvc.AgentCommandID(strings.TrimSpace(body.Action.CommandID)),
		OperationID: appsvc.AgentOperationID(strings.TrimSpace(body.Action.OperationID)),
	}
	if !validRecoveryActionKind(action.Kind) || appsvc.ValidateAgentRecoveryIdentity(string(action.CommandID), string(action.OperationID)) != nil {
		writeAgentRuntimeError(c, consts.StatusBadRequest, "agent_runtime.invalid_recovery", "恢复操作 identity 不完整 / Recovery action identity is incomplete", nil)
		return
	}
	request := appsvc.AgentRuntimeRecoveryRequest{Action: action}
	result, err := h.app.RecoverAgentChat(ctx, agentChatBinding(body.Workspace, body.SessionID), request)
	if err != nil {
		h.writeAgentRecoveryError(c, err, action)
		return
	}
	writeAgentRecoveryResponse(c, result)
}

func (h *Handlers) HandleAgentChatContextAnalysis(ctx context.Context, c *app.RequestContext) {
	var req agentChatTurnRequest
	if err := c.BindJSON(&req); err != nil || strings.TrimSpace(req.Message) == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidBody")
		return
	}
	req.Locale = requestLocale(c)
	analysis, err := h.app.AnalyzeAgentChatContext(ctx, agentChatBinding(req.Workspace, req.SessionID), req.AgentChatRequest)
	if err != nil {
		h.writeChatPreparationError(c, err)
		return
	}
	writeJSON(c, consts.StatusOK, analysis)
}

func (h *Handlers) HandleAgentChatMessages(ctx context.Context, c *app.RequestContext) {
	limit := 100
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed <= 0 {
			writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidQuery")
			return
		}
		limit = min(parsed, maxSessionMessagePageSize)
	}
	before := -1
	if raw := strings.TrimSpace(c.Query("before")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 0 {
			writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidQuery")
			return
		}
		before = parsed
	}
	page, err := h.app.AgentChatMessagesPage(ctx, agentChatBindingFromQuery(c), before, limit)
	if err != nil {
		writeError(c, consts.StatusNotFound, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, sessionMessagesPageDTO{
		Messages: agentui.MessagesFromHistoryAtOffset(page.Entries, page.NextBefore),
		Page: sessionMessagePageMeta{
			NextBefore: strconv.Itoa(page.NextBefore), HasMore: page.HasMore, Total: page.Total,
		},
	})
}

func (h *Handlers) HandleAgentChatAskAnswer(ctx context.Context, c *app.RequestContext) {
	var request struct {
		Workspace string                  `json:"workspace"`
		SessionID string                  `json:"session_id"`
		Answers   []appsvc.AgentAskAnswer `json:"answers"`
	}
	if err := c.BindJSON(&request); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidBody")
		return
	}
	result, err := h.app.AnswerAgentChatAsk(ctx, agentChatBinding(request.Workspace, request.SessionID), strings.TrimSpace(c.Param("ask_id")), request.Answers)
	if err != nil {
		writeAskResolutionError(c, err)
		return
	}
	writeJSON(c, consts.StatusOK, result)
}

func (h *Handlers) HandleAgentChatAskCancel(ctx context.Context, c *app.RequestContext) {
	var request struct {
		Workspace string `json:"workspace"`
		SessionID string `json:"session_id"`
		Reason    string `json:"reason"`
	}
	if err := c.BindJSON(&request); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidBody")
		return
	}
	result, err := h.app.CancelAgentChatAsk(ctx, agentChatBinding(request.Workspace, request.SessionID), strings.TrimSpace(c.Param("ask_id")), request.Reason)
	if err != nil {
		writeAskResolutionError(c, err)
		return
	}
	writeJSON(c, consts.StatusOK, result)
}

func (h *Handlers) HandleAgentChatSlashCommand(ctx context.Context, c *app.RequestContext) {
	var request struct {
		Workspace string `json:"workspace"`
		SessionID string `json:"session_id"`
		Command   string `json:"command"`
	}
	if err := c.BindJSON(&request); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidBody")
		return
	}
	switch strings.TrimSpace(request.Command) {
	case "clear":
		if err := h.app.ClearAgentChatSession(ctx, agentChatBinding(request.Workspace, request.SessionID)); err != nil {
			writeError(c, consts.StatusConflict, err.Error())
			return
		}
		writeJSON(c, consts.StatusOK, map[string]string{"result": "会话上下文已清空 / Conversation context cleared"})
	case "status":
		view := h.app.AgentChatActiveView(ctx, agentChatBinding(request.Workspace, request.SessionID))
		status := "空闲 / Idle"
		if view.Task != nil && !view.Task.Finished {
			status = "运行中 / Running"
		}
		writeJSON(c, consts.StatusOK, map[string]string{"result": status})
	case "help":
		writeJSON(c, consts.StatusOK, map[string]string{"result": "/clear · /status · /help"})
	case "compact":
		writeError(c, consts.StatusConflict, "AgentChat 暂不支持手动压缩 / Manual compaction is not available in AgentChat yet")
	default:
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidBody")
	}
}
