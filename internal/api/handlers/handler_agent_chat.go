package handlers

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	"denova/internal/api/agentui"
	"denova/internal/api/sse"
	appsvc "denova/internal/app"
	agentchatapp "denova/internal/app/agentchat"
	appagentruntime "denova/internal/app/agentruntime"
)

type agentChatSessionCreateRequest struct {
	Title string `json:"title"`
}

type agentChatSessionRequest struct {
	SessionID string `json:"session_id"`
}

type agentChatSessionRenameRequest struct {
	SessionID string `json:"session_id"`
	Title     string `json:"title"`
}

type agentChatTurnRequest struct {
	SessionID string `json:"session_id"`
	agentchatapp.ChatRequest
}

func agentChatBinding(projectID, sessionID string) agentchatapp.Binding {
	return agentchatapp.Binding{
		ProjectID: strings.TrimSpace(projectID), SessionID: strings.TrimSpace(sessionID),
	}
}

func requireAgentChatBinding(c *app.RequestContext, sessionID string) (agentchatapp.Binding, bool) {
	scope, ok := requireProjectScope(c)
	if !ok {
		return agentchatapp.Binding{}, false
	}
	return agentChatBinding(scope.ProjectID, sessionID), true
}

func requireAgentChatBindingFromQuery(c *app.RequestContext) (agentchatapp.Binding, bool) {
	return requireAgentChatBinding(c, c.Query("session_id"))
}

// HandleAgentChatProjects lists every project and never switches the open book.
func (h *Handlers) HandleAgentChatProjects(_ context.Context, c *app.RequestContext) {
	writeJSON(c, consts.StatusOK, map[string]any{"projects": h.app.AgentChat().Projects()})
}

// HandleAgentChatActivity is the lightweight detached-task heartbeat. Project
// metadata remains event/refocus driven and is refreshed only on transitions.
func (h *Handlers) HandleAgentChatActivity(_ context.Context, c *app.RequestContext) {
	writeJSON(c, consts.StatusOK, map[string]any{"bindings": h.app.AgentChat().Activity()})
}

func (h *Handlers) HandleAgentChatProjectCreate(_ context.Context, c *app.RequestContext) {
	var request struct {
		Path string `json:"path"`
	}
	if err := c.BindJSON(&request); err != nil || !filepath.IsAbs(strings.TrimSpace(request.Path)) {
		writeError(c, consts.StatusBadRequest, "项目目录无效 / Invalid project directory")
		return
	}
	record, err := h.app.AgentChat().AddProject(request.Path)
	if err != nil {
		writeAgentChatProjectError(c, consts.StatusBadRequest, "添加项目失败", "Failed to add project", err)
		return
	}
	writeJSON(c, consts.StatusCreated, record)
}

func (h *Handlers) HandleAgentChatProjectUpdate(ctx context.Context, c *app.RequestContext) {
	var request struct {
		Name *string `json:"name"`
		Path *string `json:"path"`
	}
	if err := c.BindJSON(&request); err != nil || (request.Name == nil) == (request.Path == nil) {
		writeError(c, consts.StatusBadRequest, "每次只能重命名或重新关联目录 / Rename or relink one project field at a time")
		return
	}
	projectID := strings.TrimSpace(c.Param("id"))
	var (
		record appsvc.ProjectRecord
		err    error
	)
	if request.Name != nil {
		record, err = h.app.AgentChat().RenameProject(projectID, *request.Name)
	} else {
		if !filepath.IsAbs(strings.TrimSpace(*request.Path)) {
			writeError(c, consts.StatusBadRequest, "项目目录必须是绝对路径 / Project directory must be an absolute path")
			return
		}
		record, err = h.app.RelinkProject(ctx, projectID, *request.Path)
	}
	if err != nil {
		writeAgentChatProjectError(c, consts.StatusConflict, "更新项目失败", "Failed to update project", err)
		return
	}
	writeJSON(c, consts.StatusOK, record)
}

func (h *Handlers) HandleAgentChatProjectArchive(ctx context.Context, c *app.RequestContext) {
	record, err := h.app.ArchiveProject(ctx, strings.TrimSpace(c.Param("id")))
	if err != nil {
		writeAgentChatProjectError(c, consts.StatusConflict, "移除项目失败", "Failed to remove project", err)
		return
	}
	writeJSON(c, consts.StatusOK, record)
}

func (h *Handlers) HandleAgentChatProjectReorder(_ context.Context, c *app.RequestContext) {
	var request struct {
		ProjectIDs []string `json:"project_ids"`
	}
	if err := c.BindJSON(&request); err != nil || len(request.ProjectIDs) == 0 {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidBody")
		return
	}
	if err := h.app.AgentChat().ReorderProjects(request.ProjectIDs); err != nil {
		writeAgentChatProjectError(c, consts.StatusBadRequest, "项目排序失败", "Failed to reorder projects", err)
		return
	}
	writeJSON(c, consts.StatusOK, map[string]any{"project_ids": request.ProjectIDs})
}

func writeAgentChatProjectError(c *app.RequestContext, status int, chinese, english string, err error) {
	writeError(c, status, fmt.Sprintf("%s / %s: %v", chinese, english, err))
}

const (
	defaultAgentChatHistoryPageSize = 80
	maxAgentChatHistoryPageSize     = 200
)

// HandleAgentChatHistory searches durable conversations across every registered project without
// changing the Writing workspace or loading message bodies.
func (h *Handlers) HandleAgentChatHistory(_ context.Context, c *app.RequestContext) {
	offset := 0
	limit := defaultAgentChatHistoryPageSize
	var err error
	if raw := strings.TrimSpace(c.Query("offset")); raw != "" {
		offset, err = strconv.Atoi(raw)
		if err != nil || offset < 0 {
			writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidQuery")
			return
		}
	}
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		limit, err = strconv.Atoi(raw)
		if err != nil || limit <= 0 {
			writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidQuery")
			return
		}
		limit = min(limit, maxAgentChatHistoryPageSize)
	}
	writeJSON(c, consts.StatusOK, h.app.AgentChat().History(agentchatapp.HistoryQuery{
		ProjectID: c.Query("project_id"),
		Search:    c.Query("query"),
		Offset:    offset,
		Limit:     limit,
	}))
}

func (h *Handlers) HandleAgentChatSessionCreate(ctx context.Context, c *app.RequestContext) {
	scope, ok := requireProjectScope(c)
	if !ok {
		return
	}
	var req agentChatSessionCreateRequest
	if err := c.BindJSON(&req); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidBody")
		return
	}
	sess, err := h.app.AgentChat().CreateSession(scope.ProjectID, strings.TrimSpace(req.Title))
	if err != nil {
		slog.ErrorContext(ctx, fmt.Sprintf("[api/handlers/handler_agent_chat.go] creating session failed project_id=%q err=%v", scope.ProjectID, err))
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, sess)
}

func (h *Handlers) HandleAgentChatSessionRename(_ context.Context, c *app.RequestContext) {
	binding, ok := requireAgentChatBinding(c, "")
	if !ok {
		return
	}
	var req agentChatSessionRenameRequest
	if err := c.BindJSON(&req); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidBody")
		return
	}
	if strings.TrimSpace(req.SessionID) == "" || strings.TrimSpace(req.Title) == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidBody")
		return
	}
	if err := h.app.AgentChat().RenameSession(binding.ProjectID, req.SessionID, req.Title); err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, map[string]any{"session_id": req.SessionID, "title": req.Title})
}

func (h *Handlers) HandleAgentChatSessionDelete(_ context.Context, c *app.RequestContext) {
	binding, ok := requireAgentChatBinding(c, "")
	if !ok {
		return
	}
	var req agentChatSessionRequest
	if err := c.BindJSON(&req); err != nil || strings.TrimSpace(req.SessionID) == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidBody")
		return
	}
	if err := h.app.AgentChat().DeleteSession(binding.ProjectID, req.SessionID); err != nil {
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
	if strings.TrimSpace(req.Message) == "" && len(req.AttachmentUploads) == 0 {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.messageRequired")
		return
	}
	if strings.TrimSpace(req.CommandID) == "" {
		writeAgentRuntimeError(c, consts.StatusBadRequest, "agent_runtime.invalid_command", "缺少 command_id，无法安全重试请求 / command_id is required for safe request retries", nil)
		return
	}
	req.Locale = requestLocale(c)
	binding, ok := requireAgentChatBinding(c, req.SessionID)
	if !ok {
		return
	}
	if err := h.app.AgentChat().MaterializeAttachments(ctx, binding, req.CommandID, &req.ChatRequest); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
		return
	}
	task, err := h.app.AgentChat().StartTask(ctx, binding, req.ChatRequest)
	if err != nil {
		h.writeChatPreparationError(c, err)
		return
	}
	slog.InfoContext(ctx, fmt.Sprintf("[agent-chat-sse] attach new task_id=%s project_id=%s session_id=%s", task.ID(), binding.ProjectID, req.SessionID))
	sse.StreamTaskUI(ctx, c, task)
}

func (h *Handlers) HandleAgentChatStream(ctx context.Context, c *app.RequestContext) {
	binding, ok := requireAgentChatBindingFromQuery(c)
	if !ok {
		return
	}
	taskID := strings.TrimSpace(c.Query("task_id"))
	if taskID == "" {
		writeAgentRuntimeError(c, consts.StatusBadRequest, "agent_runtime.invalid_command", "缺少 task_id，无法精确恢复 Agent 流 / task_id is required for exact Agent stream recovery", nil)
		return
	}
	task := h.app.AgentChat().DisplayTask(binding, taskID)
	if task == nil {
		writeAgentRuntimeError(c, consts.StatusConflict, "agent_runtime.rehydrate_required", "旧的任务流已失效，请从 active projection 重新挂接 / The old task stream is stale; rehydrate from the active projection", map[string]any{"task_id": taskID})
		return
	}
	sse.StreamTaskUI(ctx, c, task)
}

func (h *Handlers) HandleAgentChatActive(ctx context.Context, c *app.RequestContext) {
	binding, ok := requireAgentChatBindingFromQuery(c)
	if !ok {
		return
	}
	view := h.app.AgentChat().ActiveView(ctx, binding)
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
		SessionID         string                   `json:"session_id"`
		Type              string                   `json:"type"`
		CommandID         string                   `json:"command_id"`
		TargetOperationID string                   `json:"target_operation_id"`
		TargetCommandID   string                   `json:"target_command_id,omitempty"`
		Input             agentchatapp.ChatRequest `json:"input"`
		Reason            string                   `json:"reason,omitempty"`
	}
	if err := c.BindJSON(&body); err != nil {
		writeAgentRuntimeError(c, consts.StatusBadRequest, "agent_runtime.invalid_command", "命令格式无效 / Invalid agent command", nil)
		return
	}
	kind, err := writingAgentCommandKind(body.Type)
	queueControl := kind == appsvc.CommandSteerQueued || kind == appsvc.CommandCancelQueued
	if err != nil || strings.TrimSpace(body.CommandID) == "" || strings.TrimSpace(body.TargetOperationID) == "" ||
		(queueControl && strings.TrimSpace(body.TargetCommandID) == "") ||
		(kind != appsvc.CommandAbort && !queueControl && strings.TrimSpace(body.Input.Message) == "" && len(body.Input.AttachmentUploads) == 0) {
		writeAgentRuntimeError(c, consts.StatusBadRequest, "agent_runtime.invalid_command", "AgentChat 命令 identity 或消息不完整 / AgentChat command identity or message is incomplete", nil)
		return
	}
	body.Input.Locale = requestLocale(c)
	binding, ok := requireAgentChatBinding(c, body.SessionID)
	if !ok {
		return
	}
	if kind != appsvc.CommandAbort && !queueControl {
		if err := h.app.AgentChat().MaterializeAttachments(ctx, binding, body.CommandID, &body.Input); err != nil {
			writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
			return
		}
	}
	receipt, err := h.app.AgentChat().SubmitCommand(ctx, binding, appagentruntime.Command{
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
		SessionID string                     `json:"session_id"`
		Action    agentRecoveryActionRequest `json:"action"`
	}
	if err := c.BindJSON(&body); err != nil {
		writeAgentRuntimeError(c, consts.StatusBadRequest, "agent_runtime.invalid_recovery", "恢复请求格式无效 / Invalid recovery request", nil)
		return
	}
	action := appsvc.AgentRuntimeRecoveryAction{
		ActionID:    strings.TrimSpace(body.Action.ActionID),
		Kind:        appsvc.AgentRuntimeRecoveryActionKind(strings.TrimSpace(body.Action.Kind)),
		CommandID:   appsvc.AgentCommandID(strings.TrimSpace(body.Action.CommandID)),
		OperationID: appsvc.AgentOperationID(strings.TrimSpace(body.Action.OperationID)),
	}
	if !validRecoveryActionKind(action.Kind) || appsvc.ValidateAgentRecoveryIdentity(string(action.CommandID), string(action.OperationID)) != nil {
		writeAgentRuntimeError(c, consts.StatusBadRequest, "agent_runtime.invalid_recovery", "恢复操作 identity 不完整 / Recovery action identity is incomplete", nil)
		return
	}
	request := appagentruntime.RecoveryRequest{Action: action}
	binding, ok := requireAgentChatBinding(c, body.SessionID)
	if !ok {
		return
	}
	result, err := h.app.AgentChat().Recover(ctx, binding, request)
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
	binding, ok := requireAgentChatBinding(c, req.SessionID)
	if !ok {
		return
	}
	analysis, err := h.app.AgentChat().AnalyzeContext(ctx, binding, req.ChatRequest)
	if err != nil {
		h.writeChatPreparationError(c, err)
		return
	}
	writeJSON(c, consts.StatusOK, analysis)
}

func (h *Handlers) HandleAgentChatMessages(ctx context.Context, c *app.RequestContext) {
	binding, ok := requireAgentChatBindingFromQuery(c)
	if !ok {
		return
	}
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
	page, err := h.app.AgentChat().MessagesPage(ctx, binding, before, limit)
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
		SessionID string                  `json:"session_id"`
		Answers   []appsvc.AgentAskAnswer `json:"answers"`
	}
	if err := c.BindJSON(&request); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidBody")
		return
	}
	binding, ok := requireAgentChatBinding(c, request.SessionID)
	if !ok {
		return
	}
	result, err := h.app.AgentChat().AnswerAsk(ctx, binding, strings.TrimSpace(c.Param("ask_id")), request.Answers)
	if err != nil {
		writeAskResolutionError(c, err)
		return
	}
	writeJSON(c, consts.StatusOK, result)
}

func (h *Handlers) HandleAgentChatAskCancel(ctx context.Context, c *app.RequestContext) {
	var request struct {
		SessionID string `json:"session_id"`
		Reason    string `json:"reason"`
	}
	if err := c.BindJSON(&request); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidBody")
		return
	}
	binding, ok := requireAgentChatBinding(c, request.SessionID)
	if !ok {
		return
	}
	result, err := h.app.AgentChat().CancelAsk(ctx, binding, strings.TrimSpace(c.Param("ask_id")), request.Reason)
	if err != nil {
		writeAskResolutionError(c, err)
		return
	}
	writeJSON(c, consts.StatusOK, result)
}

func (h *Handlers) HandleAgentChatSlashCommand(ctx context.Context, c *app.RequestContext) {
	var request struct {
		SessionID string `json:"session_id"`
		Command   string `json:"command"`
	}
	if err := c.BindJSON(&request); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidBody")
		return
	}
	binding, ok := requireAgentChatBinding(c, request.SessionID)
	if !ok {
		return
	}
	switch strings.TrimSpace(request.Command) {
	case "clear":
		if err := h.app.AgentChat().ClearSession(ctx, binding); err != nil {
			writeError(c, consts.StatusConflict, err.Error())
			return
		}
		writeJSON(c, consts.StatusOK, map[string]string{"result": "会话上下文已清空 / Conversation context cleared"})
	case "status":
		view := h.app.AgentChat().ActiveView(ctx, binding)
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
