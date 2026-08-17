package chat

import (
	"strings"
	"time"

	"denova/internal/agents/run"
	"denova/internal/agents/session"

	agent "github.com/alfredxw/denova/agent"
)

// ProjectPendingInteraction is a read-only UI projection. The product
// transport carries the public Run identity but owns no interaction state.
func ProjectPendingInteraction(request agent.InteractionRequest, status agentrun.RuntimeStatus) *session.AskInteraction {
	if strings.TrimSpace(request.ID) == "" {
		return nil
	}
	now := time.Now().UTC()
	projected := &session.AskInteraction{
		Schema: "ask.pending.v1", ID: request.ID, Kind: session.AskKindQuestion,
		ToolCallID: request.ID, AgentKind: status.Binding.AgentKind,
		AgentCommandID: string(status.ActiveCommandID), AgentOperationID: string(status.ActiveOperation), AgentCycle: status.ActiveCycle,
		Status: session.AskPending, AllowOther: request.AllowOther, CreatedAt: now,
	}
	if request.Kind == agent.InteractionPermission && request.Permission != nil {
		permission := request.Permission
		projected.Kind = session.AskKindToolApproval
		projected.ToolCallID = firstNonEmpty(permission.CallID, request.ID)
		projected.Approval = &session.ToolApprovalPresentation{
			Mode: permission.Mode, ToolName: permission.Tool,
			Command: permission.Command, Details: permission.Details, Cwd: permission.Cwd,
			Risk: permission.Risk, RuleID: permission.RuleID, ArgsHash: permission.ArgsHash,
			CanRemember: permission.CanRemember, RuleMatcherVersion: permission.RuleMatcherVersion,
			RuleCommandKey: permission.RuleCommandKey, RuleCommandPattern: permission.RuleCommandPattern,
		}
	}
	projected.Questions = make([]session.AskQuestion, len(request.Questions))
	for index, question := range request.Questions {
		item := session.AskQuestion{
			ID: question.ID, Question: strings.TrimSpace(question.Prompt), MultiSelect: question.Multiple,
			Options: make([]session.AskOption, len(question.Options)),
		}
		for optionIndex, option := range question.Options {
			item.Options[optionIndex] = session.AskOption{
				ID: option.Value, Label: strings.TrimSpace(option.Label),
				Description: strings.TrimSpace(option.Description),
			}
			if option.Recommended && item.RecommendedOptionID == "" {
				item.RecommendedOptionID = option.Value
			}
		}
		projected.Questions[index] = item
	}
	return projected
}
