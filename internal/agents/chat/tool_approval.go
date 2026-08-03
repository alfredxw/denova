package chat

import (
	"context"
	"strings"

	"denova/config"
	"denova/internal/agents/session"
	"denova/internal/agents/toolapproval"
	agenttoolruntime "denova/internal/agents/toolruntime"
)

const toolApprovalDetailsMax = 16 * 1024

func (interaction *runAskInteraction) ApproveTool(ctx context.Context, request agenttoolruntime.ApprovalRequest) (agenttoolruntime.ApprovalResult, error) {
	if interaction == nil {
		return agenttoolruntime.ApprovalResult{Choice: agenttoolruntime.ApprovalDenied}, nil
	}
	presentation := &session.ToolApprovalPresentation{
		Mode: string(config.NormalizeAgentApprovalMode(request.Mode)), ToolName: strings.TrimSpace(request.ToolName),
		Command: request.Decision.Command, Details: toolApprovalDetails(request), Cwd: request.Decision.Cwd,
		Risk: string(request.Decision.Risk), RuleID: request.Decision.RuleID,
		ArgsHash: toolapproval.ArgumentsHash(request.Arguments),
	}
	options := []session.AskOption{{
		ID: session.ToolApprovalAllowOnceOptionID, Label: "Allow once",
		Description: "Execute only this call without changing saved permissions.",
	}}
	if request.Decision.Remember != nil {
		presentation.CanRemember = true
		presentation.RuleMatcherVersion = request.Decision.Remember.MatcherVersion
		presentation.RuleCommandKey = request.Decision.Remember.CommandKey
		presentation.RuleCommandPattern = request.Decision.Remember.CommandPattern
		options = append(options, session.AskOption{
			ID: session.ToolApprovalAllowWorkspaceOptionID, Label: "Always allow in this workspace",
			Description: "Save the displayed command rule for this workspace.",
		})
	}
	options = append(options, session.AskOption{
		ID: session.ToolApprovalDenyOptionID, Label: "Deny",
		Description: "Block this call and let the Agent choose another approach.",
	})
	resolution, err := interaction.Ask(ctx, session.AskInteraction{
		Kind:           session.AskKindToolApproval,
		ToolCallID:     strings.TrimSpace(request.ExecutionID),
		ProviderCallID: strings.TrimSpace(request.ProviderCallID),
		Approval:       presentation,
		Questions: []session.AskQuestion{{
			ID: session.ToolApprovalQuestionID, Question: "Approve this tool call?", Options: options,
		}},
	})
	if err != nil || resolution.Status != session.AskAnswered || len(resolution.Answers) != 1 {
		return agenttoolruntime.ApprovalResult{Choice: agenttoolruntime.ApprovalDenied}, err
	}
	selected := resolution.Answers[0].SelectedOptions
	if len(selected) != 1 {
		return agenttoolruntime.ApprovalResult{Choice: agenttoolruntime.ApprovalDenied}, nil
	}
	switch selected[0].ID {
	case session.ToolApprovalAllowOnceOptionID:
		return agenttoolruntime.ApprovalResult{Choice: agenttoolruntime.ApprovalAllowOnce}, nil
	case session.ToolApprovalAllowWorkspaceOptionID:
		if presentation.CanRemember {
			return agenttoolruntime.ApprovalResult{Choice: agenttoolruntime.ApprovalAllowWorkspace}, nil
		}
	}
	return agenttoolruntime.ApprovalResult{Choice: agenttoolruntime.ApprovalDenied}, nil
}

func toolApprovalDetails(request agenttoolruntime.ApprovalRequest) string {
	if strings.TrimSpace(request.Decision.Command) != "" {
		return ""
	}
	value := strings.TrimSpace(strings.ToValidUTF8(request.Arguments, "\uFFFD"))
	if len(value) <= toolApprovalDetailsMax {
		return value
	}
	const marker = "\n… [details truncated / 详情已截断]"
	return truncateUTF8StringBytes(value, toolApprovalDetailsMax-len(marker)) + marker
}
