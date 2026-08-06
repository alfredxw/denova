package handlers

import (
	"context"
	"errors"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	novaApp "denova/internal/app"
)

type agentRecoveryActionRequest struct {
	Kind        string `json:"kind"`
	CommandID   string `json:"command_id"`
	OperationID string `json:"operation_id"`
}

type agentRecoveryRequest struct {
	Action    agentRecoveryActionRequest `json:"action"`
	SessionID string                     `json:"session_id,omitempty"`
	StoryID   string                     `json:"story_id,omitempty"`
	BranchID  string                     `json:"branch_id,omitempty"`
	Branch    string                     `json:"branch,omitempty"`
}

type agentRecoveryBinding int

const (
	agentRecoveryWriting agentRecoveryBinding = iota
	agentRecoveryInteractive
	agentRecoveryConfigManager
)

type agentRecoveryResponse struct {
	TaskID         string                        `json:"task_id"`
	Status         string                        `json:"status"`
	StreamCursor   uint64                        `json:"stream_cursor"`
	Cursor         uint64                        `json:"cursor"`
	Replayed       bool                          `json:"replayed"`
	RecoveryAction agentRuntimeRecoveryActionDTO `json:"recovery_action"`
}

func (h *Handlers) HandleChatRecovery(ctx context.Context, c *app.RequestContext) {
	if !h.requireWorkspace(c) {
		return
	}
	request, sessionID, ok := bindAgentRecoveryRequest(c, agentRecoveryWriting)
	if !ok {
		return
	}
	result, err := h.app.RecoverWritingAgentForSession(ctx, sessionID, request)
	if err != nil {
		h.writeAgentRecoveryError(c, err, request.Action)
		return
	}
	writeAgentRecoveryResponse(c, result)
}

func (h *Handlers) HandleInteractiveChatRecovery(ctx context.Context, c *app.RequestContext) {
	if !h.requireWorkspace(c) {
		return
	}
	request, _, ok := bindAgentRecoveryRequest(c, agentRecoveryInteractive)
	if !ok {
		return
	}
	result, err := h.app.RecoverInteractiveAgent(ctx, request)
	if err != nil {
		h.writeAgentRecoveryError(c, err, request.Action)
		return
	}
	writeAgentRecoveryResponse(c, result)
}

func bindAgentRecoveryRequest(c *app.RequestContext, binding agentRecoveryBinding) (novaApp.AgentRuntimeRecoveryRequest, string, bool) {
	var body agentRecoveryRequest
	if err := c.BindJSON(&body); err != nil {
		writeAgentRuntimeError(c, consts.StatusBadRequest, "agent_runtime.invalid_recovery", "恢复请求格式无效 / Invalid recovery request", nil)
		return novaApp.AgentRuntimeRecoveryRequest{}, "", false
	}
	action := novaApp.AgentRuntimeRecoveryAction{
		Kind:        novaApp.AgentRuntimeRecoveryActionKind(strings.TrimSpace(body.Action.Kind)),
		CommandID:   novaApp.AgentCommandID(strings.TrimSpace(body.Action.CommandID)),
		OperationID: novaApp.AgentOperationID(strings.TrimSpace(body.Action.OperationID)),
	}
	if !validRecoveryActionKind(action.Kind) ||
		novaApp.ValidateAgentRecoveryIdentity(string(action.CommandID), string(action.OperationID)) != nil {
		writeAgentRuntimeError(c, consts.StatusBadRequest, "agent_runtime.invalid_recovery", "恢复操作 identity 不完整 / Recovery action identity is incomplete", nil)
		return novaApp.AgentRuntimeRecoveryRequest{}, "", false
	}
	request := novaApp.AgentRuntimeRecoveryRequest{Action: action}
	sessionID := ""
	switch binding {
	case agentRecoveryInteractive:
		request.StoryID = strings.TrimSpace(body.StoryID)
		request.BranchID = strings.TrimSpace(body.BranchID)
		if request.BranchID == "" {
			request.BranchID = strings.TrimSpace(body.Branch)
		}
		if request.StoryID == "" {
			writeAgentRuntimeError(c, consts.StatusBadRequest, "agent_runtime.invalid_recovery", "story_id 为必填项 / story_id is required", nil)
			return novaApp.AgentRuntimeRecoveryRequest{}, "", false
		}
	case agentRecoveryWriting:
		var ok bool
		sessionID, ok = requiredWritingSessionID(c, body.SessionID)
		if !ok {
			return novaApp.AgentRuntimeRecoveryRequest{}, "", false
		}
	case agentRecoveryConfigManager:
		// Config Manager identity comes from the project-scoped route/query.
	}
	return request, sessionID, true
}

func validRecoveryActionKind(kind novaApp.AgentRuntimeRecoveryActionKind) bool {
	switch kind {
	case novaApp.AgentRuntimeRecoveryAttach,
		novaApp.AgentRuntimeRecoveryAbort,
		novaApp.AgentRuntimeRecoverySteer,
		novaApp.AgentRuntimeRecoveryFollowUp,
		novaApp.AgentRuntimeRecoveryNextTurn,
		novaApp.AgentRuntimeRecoveryCompactContext,
		novaApp.AgentRuntimeRecoveryRemoveCompaction:
		return true
	default:
		return false
	}
}

func writeAgentRecoveryResponse(c *app.RequestContext, result novaApp.AgentRuntimeRecoveryResult) {
	snapshot := result.Task.Snapshot()
	c.JSON(consts.StatusAccepted, agentRecoveryResponse{
		TaskID: snapshot.ID, Status: string(snapshot.Status), StreamCursor: snapshot.Cursor,
		Cursor: uint64(result.Receipt.Cursor), Replayed: result.Receipt.Replayed,
		RecoveryAction: agentRuntimeRecoveryActionDTO{
			Kind: string(result.Action.Kind), CommandID: string(result.Action.CommandID), OperationID: string(result.Action.OperationID),
		},
	})
}

func (h *Handlers) writeAgentRecoveryError(c *app.RequestContext, err error, action novaApp.AgentRuntimeRecoveryAction) {
	details := map[string]any{
		"kind": string(action.Kind), "command_id": string(action.CommandID), "operation_id": string(action.OperationID),
	}
	switch {
	case errors.Is(err, novaApp.ErrAgentRecoveryActionChanged), errors.Is(err, novaApp.ErrAgentRuntimeRecoveryActionChanged):
		writeAgentRuntimeError(c, consts.StatusConflict, "agent_runtime.recovery_changed", "可恢复操作已变化，请刷新状态 / The recoverable action changed; refresh runtime status", details)
	case errors.Is(err, novaApp.ErrAgentOperationActive):
		writeAgentRuntimeError(c, consts.StatusConflict, "agent_runtime.stream_attached", "该运行已有展示流 / This runtime already has an attached display stream", details)
	case errors.Is(err, novaApp.ErrWorkspaceTransition), errors.Is(err, novaApp.ErrAgentContextChanged), errors.Is(err, novaApp.ErrWorkspaceChanged):
		writeAgentRuntimeError(c, consts.StatusConflict, "agent_runtime.context_changed", "Agent 运行上下文已变化 / The agent runtime context changed", details)
	case errors.Is(err, novaApp.ErrInvalidAgentCommand), errors.Is(err, novaApp.ErrInvalidAgentBinding):
		writeAgentRuntimeError(c, consts.StatusBadRequest, "agent_runtime.invalid_recovery", "Agent 恢复请求无效 / Invalid agent recovery request", details)
	case errors.Is(err, novaApp.ErrNoWorkspace):
		writeAgentRuntimeError(c, consts.StatusConflict, "agent_runtime.no_workspace", "尚未选择工作区 / No workspace is open", nil)
	default:
		writeAgentRuntimeError(c, consts.StatusInternalServerError, "agent_runtime.recovery_failed", "Agent 恢复失败 / Failed to recover agent runtime", details)
	}
}
