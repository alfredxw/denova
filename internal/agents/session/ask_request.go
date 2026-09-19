package session

import (
	"strings"
	"time"

	agent "github.com/alfredxw/denova/agent"
)

// ProjectInteractionRequest is a display-only projection shared by live Native
// interactions and external journal replay. Ownership is filled by the caller.
func ProjectInteractionRequest(request agent.InteractionRequest, createdAt time.Time) *AskInteraction {
	if strings.TrimSpace(request.ID) == "" {
		return nil
	}
	projected := &AskInteraction{
		Schema: "ask.pending.v1", ID: request.ID, Kind: AskKindQuestion, ToolCallID: request.ID,
		Status: AskPending, AllowOther: request.AllowOther, CreatedAt: createdAt, Verification: request.Verification,
	}
	if request.Kind == agent.InteractionPermission && request.Permission != nil {
		permission := request.Permission
		projected.Kind = AskKindToolApproval
		projected.ToolCallID = permission.CallID
		if strings.TrimSpace(projected.ToolCallID) == "" {
			projected.ToolCallID = request.ID
		}
		projected.Approval = &ToolApprovalPresentation{
			Mode: permission.Mode, ToolName: permission.Tool,
			Command: permission.Command, Details: permission.Details, Cwd: permission.Cwd,
			Risk: permission.Risk, RuleID: permission.RuleID, ArgsHash: permission.ArgsHash,
			CanRemember: permission.CanRemember, RuleMatcherVersion: permission.RuleMatcherVersion,
			RuleMatchKey: permission.RuleMatchKey, RuleDisplayPattern: permission.RuleDisplayPattern,
		}
	}
	projected.Questions = make([]AskQuestion, len(request.Questions))
	for index, question := range request.Questions {
		item := AskQuestion{ID: question.ID, Question: strings.TrimSpace(question.Prompt), MultiSelect: question.Multiple, Options: make([]AskOption, len(question.Options))}
		for optionIndex, option := range question.Options {
			item.Options[optionIndex] = AskOption{ID: option.Value, Label: strings.TrimSpace(option.Label), Description: strings.TrimSpace(option.Description)}
			if option.Recommended && item.RecommendedOptionID == "" {
				item.RecommendedOptionID = option.Value
			}
		}
		projected.Questions[index] = item
	}
	return projected
}
