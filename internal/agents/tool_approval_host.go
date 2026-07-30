package agents

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"denova/config"
	"denova/internal/agents/session"
	"denova/internal/agents/toolapproval"
)

const (
	toolApprovalQuestionID = "tool-approval"
	toolApprovalAllowID    = "allow-once"
	toolApprovalDenyID     = "deny"
	toolApprovalDetailsMax = 16 * 1024
)

type toolApprovalRequest struct {
	Mode           config.AgentApprovalMode
	ToolName       string
	ProviderCallID string
	ExecutionID    string
	Arguments      string
	Decision       toolapproval.Decision
}

type toolApprovalHost interface {
	ApproveTool(context.Context, toolApprovalRequest) (bool, error)
}

type toolApprovalHostContextKey struct{}

func contextWithToolApprovalHost(ctx context.Context, host toolApprovalHost) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if host == nil {
		return ctx
	}
	return context.WithValue(ctx, toolApprovalHostContextKey{}, host)
}

func toolApprovalHostFromContext(ctx context.Context) (toolApprovalHost, bool) {
	if ctx == nil {
		return nil, false
	}
	host, ok := ctx.Value(toolApprovalHostContextKey{}).(toolApprovalHost)
	return host, ok && host != nil
}

func (interaction *runAskInteraction) ApproveTool(ctx context.Context, request toolApprovalRequest) (bool, error) {
	if interaction == nil {
		return false, nil
	}
	digest := sha256.Sum256([]byte(request.Arguments))
	resolution, err := interaction.Ask(ctx, session.AskInteraction{
		Kind:           session.AskKindToolApproval,
		ToolCallID:     strings.TrimSpace(request.ExecutionID),
		ProviderCallID: strings.TrimSpace(request.ProviderCallID),
		Approval: &session.ToolApprovalPresentation{
			Mode: string(config.NormalizeAgentApprovalMode(request.Mode)), ToolName: strings.TrimSpace(request.ToolName),
			Command: request.Decision.Command, Details: toolApprovalDetails(request), Cwd: request.Decision.Cwd,
			Risk: string(request.Decision.Risk), Reason: request.Decision.Reason,
			RuleID: request.Decision.RuleID, ArgsHash: hex.EncodeToString(digest[:]),
		},
		Questions: []session.AskQuestion{{
			ID:       toolApprovalQuestionID,
			Question: "是否仅允许本次工具调用？ / Allow this tool call once?",
			Options: []session.AskOption{
				{ID: toolApprovalAllowID, Label: "仅允许本次 / Allow once", Description: "只执行当前这一次调用，不改变安全模式。 / Execute only this call without changing the safety mode."},
				{ID: toolApprovalDenyID, Label: "拒绝 / Deny", Description: "阻止本次调用并让 Agent 继续选择其他方案。 / Block this call and let the Agent choose another approach."},
			},
		}},
	})
	if err != nil || resolution.Status != session.AskAnswered || len(resolution.Answers) != 1 {
		return false, err
	}
	selected := resolution.Answers[0].SelectedOptions
	return len(selected) == 1 && selected[0].ID == toolApprovalAllowID, nil
}

func toolApprovalDetails(request toolApprovalRequest) string {
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
