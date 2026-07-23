package handlers

import (
	"context"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	"denova/internal/agent"
	"denova/internal/agentruntime"
	novaApp "denova/internal/app"
)

type interactiveAgentCommandRequest struct {
	Type              string            `json:"type"`
	CommandID         string            `json:"command_id"`
	TargetOperationID string            `json:"target_operation_id"`
	StoryID           string            `json:"story_id"`
	BranchID          string            `json:"branch_id,omitempty"`
	Branch            string            `json:"branch,omitempty"`
	Input             agent.ChatRequest `json:"input"`
	Reason            string            `json:"reason,omitempty"`
}

// HandleInteractiveChatCommand accepts a typed command for the current game
// operation. The client selects a story/branch, while App derives and verifies
// the workspace-scoped durable binding from server state.
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
	if kind != agent.AgentCommandAbort && strings.TrimSpace(body.Input.Message) == "" {
		writeAgentRuntimeError(c, consts.StatusBadRequest, "agent_runtime.invalid_command", "消息不能为空 / Message is required", nil)
		return
	}
	body.Input.Locale = requestLocale(c)
	branchID := strings.TrimSpace(body.BranchID)
	if branchID == "" {
		branchID = strings.TrimSpace(body.Branch)
	}
	receipt, err := h.app.SubmitInteractiveAgentCommand(ctx, novaApp.InteractiveAgentCommand{
		Kind: kind, CommandID: strings.TrimSpace(body.CommandID),
		OperationID: agentruntime.OperationID(strings.TrimSpace(body.TargetOperationID)),
		StoryID:     strings.TrimSpace(body.StoryID), BranchID: branchID,
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

func interactiveAgentCommandKind(value string) (agent.AgentCommandKind, error) {
	switch strings.TrimSpace(value) {
	case string(agent.AgentCommandSteer):
		return agent.AgentCommandSteer, nil
	case string(agent.AgentCommandFollowUp):
		return agent.AgentCommandFollowUp, nil
	case string(agent.AgentCommandNextTurn):
		return agent.AgentCommandNextTurn, nil
	case string(agent.AgentCommandAbort):
		return agent.AgentCommandAbort, nil
	default:
		return "", agentruntime.ErrInvalidCommand
	}
}
