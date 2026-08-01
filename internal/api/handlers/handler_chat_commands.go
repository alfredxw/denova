package handlers

import (
	"context"
	"errors"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	novaApp "denova/internal/app"
)

type chatAgentCommandRequest struct {
	Type              string                   `json:"type"`
	CommandID         string                   `json:"command_id"`
	TargetOperationID string                   `json:"target_operation_id"`
	TargetCommandID   string                   `json:"target_command_id,omitempty"`
	Input             novaApp.AgentChatRequest `json:"input"`
	Reason            string                   `json:"reason,omitempty"`
}

type agentCommandReceiptResponse struct {
	CommandID   string `json:"command_id"`
	OperationID string `json:"operation_id"`
	Cursor      uint64 `json:"cursor"`
}

type agentRuntimeErrorResponse struct {
	Error   string         `json:"error"`
	Code    string         `json:"code"`
	Details map[string]any `json:"details,omitempty"`
}

func (h *Handlers) HandleChatCommand(ctx context.Context, c *app.RequestContext) {
	if !h.requireWorkspace(c) {
		return
	}
	var body chatAgentCommandRequest
	if err := c.BindJSON(&body); err != nil {
		writeAgentRuntimeError(c, consts.StatusBadRequest, "agent_runtime.invalid_command", "命令格式无效 / Invalid agent command", nil)
		return
	}
	kind, err := writingAgentCommandKind(body.Type)
	if err != nil || strings.TrimSpace(body.CommandID) == "" || strings.TrimSpace(body.TargetOperationID) == "" {
		writeAgentRuntimeError(c, consts.StatusBadRequest, "agent_runtime.invalid_command", "命令类型、command_id 和 target_operation_id 为必填项 / Command type, command_id, and target_operation_id are required", nil)
		return
	}
	queueControl := kind == novaApp.CommandSteerQueued || kind == novaApp.CommandCancelQueued
	if queueControl && strings.TrimSpace(body.TargetCommandID) == "" {
		writeAgentRuntimeError(c, consts.StatusBadRequest, "agent_runtime.invalid_command", "target_command_id 为必填项 / target_command_id is required", nil)
		return
	}
	if kind != novaApp.CommandAbort && !queueControl && strings.TrimSpace(body.Input.Message) == "" {
		writeAgentRuntimeError(c, consts.StatusBadRequest, "agent_runtime.invalid_command", "消息不能为空 / Message is required", nil)
		return
	}
	body.Input.Locale = requestLocale(c)
	receipt, err := h.app.SubmitChatAgentCommand(ctx, novaApp.ChatAgentCommand{
		Kind: kind, CommandID: strings.TrimSpace(body.CommandID),
		OperationID:     novaApp.AgentOperationID(strings.TrimSpace(body.TargetOperationID)),
		TargetCommandID: novaApp.AgentCommandID(strings.TrimSpace(body.TargetCommandID)),
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

func writingAgentCommandKind(value string) (novaApp.CommandKind, error) {
	switch strings.TrimSpace(value) {
	case string(novaApp.CommandSteer):
		return novaApp.CommandSteer, nil
	case string(novaApp.CommandFollowUp):
		return novaApp.CommandFollowUp, nil
	case string(novaApp.CommandNextTurn):
		return novaApp.CommandNextTurn, nil
	case string(novaApp.CommandAbort):
		return novaApp.CommandAbort, nil
	case string(novaApp.CommandSteerQueued):
		return novaApp.CommandSteerQueued, nil
	case string(novaApp.CommandCancelQueued):
		return novaApp.CommandCancelQueued, nil
	default:
		return "", novaApp.ErrInvalidAgentCommand
	}
}

func (h *Handlers) writeAgentCommandError(c *app.RequestContext, err error, target string) {
	details := map[string]any{"target_operation_id": strings.TrimSpace(target)}
	switch {
	case errors.Is(err, novaApp.ErrNoActiveAgentOperation):
		writeAgentRuntimeError(c, consts.StatusConflict, "agent_runtime.invalid_phase", "当前没有运行中的 Agent / No agent operation is running", details)
	case errors.Is(err, novaApp.ErrStaleAgentOperation):
		writeAgentRuntimeError(c, consts.StatusConflict, "agent_runtime.target_operation_mismatch", "目标 Agent 运行已变化 / The target agent operation has changed", details)
	case errors.Is(err, novaApp.ErrAgentQueueConflict):
		writeAgentRuntimeError(c, consts.StatusConflict, "agent_runtime.queue_conflict", "队列操作与当前状态冲突 / Queue action conflicts with the current state", details)
	case errors.Is(err, novaApp.ErrAgentBusy):
		writeAgentRuntimeError(c, consts.StatusConflict, "agent_runtime.busy", "Agent 正忙 / Agent is busy", details)
	case errors.Is(err, novaApp.ErrAgentOperationActive):
		writeAgentRuntimeError(c, consts.StatusConflict, "agent_runtime.busy", "Agent 正忙 / Agent is busy", details)
	case errors.Is(err, novaApp.ErrAgentDomainCommitRejected):
		writeAgentRuntimeError(c, consts.StatusConflict, "agent_runtime.commit_won", "Agent 输出已进入规范提交，当前控制命令未生效 / The agent output is already committing; this control command was not applied", details)
	case errors.Is(err, novaApp.ErrWorkspaceTransition), errors.Is(err, novaApp.ErrAgentContextChanged), errors.Is(err, novaApp.ErrWorkspaceChanged):
		writeAgentRuntimeError(c, consts.StatusConflict, "agent_runtime.context_changed", "Agent 运行上下文已变化 / The agent runtime context changed", details)
	case errors.Is(err, novaApp.ErrInvalidAgentCommand), errors.Is(err, novaApp.ErrInvalidAgentBinding):
		writeAgentRuntimeError(c, consts.StatusBadRequest, "agent_runtime.invalid_command", "Agent 命令无效 / Invalid agent command", details)
	case errors.Is(err, novaApp.ErrNoWorkspace):
		writeAgentRuntimeError(c, consts.StatusConflict, "agent_runtime.no_workspace", "尚未选择工作区 / No workspace is open", nil)
	default:
		writeAgentRuntimeError(c, consts.StatusInternalServerError, "agent_runtime.failed", "Agent 命令提交失败 / Failed to submit agent command", details)
	}
}

func writeAgentRuntimeError(c *app.RequestContext, status int, code, message string, details map[string]any) {
	c.JSON(status, agentRuntimeErrorResponse{Error: message, Code: code, Details: details})
}
