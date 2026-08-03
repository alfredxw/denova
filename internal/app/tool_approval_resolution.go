package app

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	agentconversation "denova/internal/agents/conversation"
	"denova/internal/agents/session"
	"denova/internal/agents/toolapproval"
)

// resolveAgentAsk is the single application transaction boundary shared by
// Writing, Agent Chat, and Config Manager. A permanent command authorization
// is persisted before the blocked tool call is released, and it is derived
// exclusively from server-owned pending Ask metadata.
func (a *App) resolveAgentAsk(
	ctx context.Context,
	target *session.Session,
	projectID, workspace, askID, status string,
	answers []agentconversation.HostAskAnswer,
	cancelReason string,
) (agentconversation.HostAskResolution, error) {
	if a == nil || target == nil {
		return agentconversation.HostAskResolution{}, ErrNoWorkspace
	}
	a.agentAskResolutionMu.Lock()
	defer a.agentAskResolutionMu.Unlock()

	pending := target.PendingAsk(askID)
	wantsRule := isWorkspaceRuleAnswer(status, answers)
	var ruleID string
	created := false
	if wantsRule {
		if pending == nil || pending.Kind != session.AskKindToolApproval || pending.Approval == nil || !pending.Approval.CanRemember {
			return agentconversation.HostAskResolution{}, fmt.Errorf("pending ask does not offer a workspace command rule")
		}
		approval := pending.Approval
		rule, err := toolapproval.NewWorkspaceRule(
			projectID, workspace, approval.ToolName,
			toolapproval.RuleProposal{
				MatcherVersion: approval.RuleMatcherVersion,
				CommandKey:     approval.RuleCommandKey,
				CommandPattern: approval.RuleCommandPattern,
			},
			approval.ArgsHash, approval.Command, approval.Cwd, approval.RuleID, time.Now(),
		)
		if err != nil {
			return agentconversation.HostAskResolution{}, err
		}
		ruleID = rule.ID
		created, err = a.SettingsService().EnsureAgentApprovalRule(rule)
		if err != nil {
			if created {
				a.rollbackAgentApprovalRule(ruleID)
			}
			return agentconversation.HostAskResolution{}, fmt.Errorf("persist workspace command approval: %w", err)
		}
	}

	resolution, err := agentconversation.ResolveAsk(ctx, target, askID, status, answers, cancelReason)
	if created && (err != nil || resolution.Status != session.AskAnswered) {
		a.rollbackAgentApprovalRule(ruleID)
	}
	return resolution, err
}

func isWorkspaceRuleAnswer(status string, answers []agentconversation.HostAskAnswer) bool {
	if strings.TrimSpace(status) != session.AskAnswered || len(answers) != 1 {
		return false
	}
	answer := answers[0]
	return strings.TrimSpace(answer.QuestionID) == session.ToolApprovalQuestionID &&
		strings.TrimSpace(answer.CustomInput) == "" && len(answer.SelectedOptionIDs) == 1 &&
		strings.TrimSpace(answer.SelectedOptionIDs[0]) == session.ToolApprovalAllowWorkspaceOptionID
}

func (a *App) rollbackAgentApprovalRule(ruleID string) {
	if _, _, err := a.SettingsService().RemoveAgentApprovalRule(ruleID); err != nil {
		slog.ErrorContext(context.Background(), fmt.Sprintf(
			"[app/approval] failed to roll back uncommitted command rule id=%s err=%v", ruleID, err,
		))
	}
}
