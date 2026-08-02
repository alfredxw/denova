package handlers

import (
	"context"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	novaApp "denova/internal/app"
)

type interactiveAgentCommandRequest struct {
	Type              string                   `json:"type"`
	CommandID         string                   `json:"command_id"`
	TargetOperationID string                   `json:"target_operation_id"`
	TargetCommandID   string                   `json:"target_command_id,omitempty"`
	StoryID           string                   `json:"story_id"`
	BranchID          string                   `json:"branch_id,omitempty"`
	Branch            string                   `json:"branch,omitempty"`
	Input             novaApp.AgentChatRequest `json:"input"`
	Reason            string                   `json:"reason,omitempty"`
}

// HandleInteractiveChatCommand controls the exact active game operation.
func (h *Handlers) HandleInteractiveChatCommand(ctx context.Context, c *app.RequestContext) {
	if !h.requireWorkspace(c) {
		return
	}
	var body interactiveAgentCommandRequest
	if err := c.BindJSON(&body); err != nil {
		writeAgentRuntimeError(c, consts.StatusBadRequest, "agent_runtime.invalid_command", "命令格式无效 / Invalid agent command", nil)
		return
	}
	kind, err := interactiveAgentCommandKind(body.Type)
	if err != nil || strings.TrimSpace(body.CommandID) == "" || strings.TrimSpace(body.TargetOperationID) == "" || strings.TrimSpace(body.StoryID) == "" {
		writeAgentRuntimeError(c, consts.StatusBadRequest, "agent_runtime.invalid_command", "命令类型、command_id、target_operation_id 和 story_id 为必填项 / Command type, command_id, target_operation_id, and story_id are required", nil)
		return
	}
	queueControl := kind == novaApp.CommandSteerQueued || kind == novaApp.CommandCancelQueued
	if queueControl && strings.TrimSpace(body.TargetCommandID) == "" {
		writeAgentRuntimeError(c, consts.StatusBadRequest, "agent_runtime.invalid_command", "target_command_id 为必填项 / target_command_id is required", nil)
		return
	}
	if kind == novaApp.CommandFollowUp && strings.TrimSpace(body.Input.Message) == "" {
		writeAgentRuntimeError(c, consts.StatusBadRequest, "agent_runtime.invalid_command", "消息不能为空 / Message is required", nil)
		return
	}
	branchID := strings.TrimSpace(body.BranchID)
	if branchID == "" {
		branchID = strings.TrimSpace(body.Branch)
	}
	body.Input.Locale = requestLocale(c)
	receipt, err := h.app.SubmitInteractiveAgentCommand(ctx, novaApp.InteractiveAgentCommand{
		Kind: kind, CommandID: strings.TrimSpace(body.CommandID),
		OperationID:     novaApp.AgentOperationID(strings.TrimSpace(body.TargetOperationID)),
		TargetCommandID: novaApp.AgentCommandID(strings.TrimSpace(body.TargetCommandID)),
		StoryID:         strings.TrimSpace(body.StoryID), BranchID: branchID,
		Reason: body.Reason, Input: body.Input,
	})
	if err != nil {
		h.writeAgentCommandError(c, err, body.TargetOperationID)
		return
	}
	c.JSON(consts.StatusAccepted, agentCommandReceiptResponse{
		CommandID: string(receipt.CommandID), OperationID: string(receipt.OperationID), Cursor: uint64(receipt.Cursor),
	})
}

func interactiveAgentCommandKind(value string) (novaApp.CommandKind, error) {
	switch strings.TrimSpace(value) {
	case string(novaApp.CommandFollowUp):
		return novaApp.CommandFollowUp, nil
	case string(novaApp.CommandSteerQueued):
		return novaApp.CommandSteerQueued, nil
	case string(novaApp.CommandCancelQueued):
		return novaApp.CommandCancelQueued, nil
	case string(novaApp.CommandAbort):
		return novaApp.CommandAbort, nil
	default:
		return "", novaApp.ErrInvalidAgentCommand
	}
}
