package execution

import (
	"context"
	"strings"

	agentconversation "denova/internal/agents/conversation"
	agentrun "denova/internal/agents/run"
	"denova/internal/agents/session"

	agent "github.com/alfredxw/denova/agent"
)

// ResolveAsk adapts Denova's stable transport shape to the public Interaction
// response vocabulary. It never writes the product Session journal.
func (runtime *Runtime) ResolveAsk(
	ctx context.Context,
	options agentrun.Options,
	askID, status string,
	answers []agentconversation.HostAskAnswer,
	cancelReason string,
) (agentconversation.HostAskResolution, error) {
	statusSnapshot, err := runtime.RuntimeStatusProjection(ctx, options)
	if err != nil {
		return agentconversation.HostAskResolution{}, err
	}
	var pending agent.InteractionRequest
	for _, candidate := range statusSnapshot.PendingInteractions {
		if candidate.ID == strings.TrimSpace(askID) {
			pending = candidate
			break
		}
	}
	if pending.ID == "" {
		return agentconversation.HostAskResolution{}, agent.ErrInteractionStale
	}
	response := agent.InteractionResponse{Cancelled: status == session.AskCancelled}
	if !response.Cancelled && pending.Kind == agent.InteractionPermission {
		if len(answers) != 1 || len(answers[0].SelectedOptionIDs) != 1 {
			return agentconversation.HostAskResolution{}, agent.ErrInteractionStale
		}
		switch strings.TrimSpace(answers[0].SelectedOptionIDs[0]) {
		case session.ToolApprovalAllowOnceOptionID:
			response.Permission = agent.PermissionAllowOnce
		case session.ToolApprovalAllowWorkspaceOptionID:
			response.Permission = agent.PermissionRemember
		case session.ToolApprovalDenyOptionID:
			response.Permission = agent.PermissionDeny
		default:
			return agentconversation.HostAskResolution{}, agent.ErrInteractionStale
		}
	} else if !response.Cancelled {
		response.Answers = make([]agent.InteractionAnswer, len(answers))
		for index, answer := range answers {
			values := make([]string, 0, len(answer.SelectedOptionIDs))
			for _, value := range answer.SelectedOptionIDs {
				if value = strings.TrimSpace(value); value != "" && value != "other" {
					values = append(values, value)
				}
			}
			response.Answers[index] = agent.InteractionAnswer{
				QuestionID: answer.QuestionID, Values: values, Text: answer.CustomInput,
			}
		}
	}
	request, resolution, err := runtime.ResolveInteraction(ctx, options, askID, response)
	if err != nil {
		return agentconversation.HostAskResolution{}, err
	}
	result := agentconversation.HostAskResolution{Schema: "ask.result.v1", ID: askID}
	if resolution.Cancelled {
		result.Status, result.CancelReason = session.AskCancelled, strings.TrimSpace(cancelReason)
		return result, nil
	}
	result.Status = session.AskAnswered
	if request.Kind == agent.InteractionPermission {
		return result, nil
	}
	questions := make(map[string]agent.InteractionQuestion, len(request.Questions))
	for _, question := range request.Questions {
		questions[question.ID] = question
	}
	for _, answer := range resolution.Answers {
		question := questions[answer.QuestionID]
		projected := agentconversation.HostAskAnswerResult{
			QuestionID: answer.QuestionID, Question: strings.TrimSpace(question.Prompt), CustomInput: answer.Text,
		}
		byValue := make(map[string]agent.InteractionOption, len(question.Options))
		for _, option := range question.Options {
			byValue[option.Value] = option
		}
		for _, value := range answer.Values {
			option := byValue[value]
			projected.SelectedOptions = append(projected.SelectedOptions, agentconversation.HostAskSelectedOption{
				ID: value, Label: strings.TrimSpace(option.Label),
			})
		}
		result.Answers = append(result.Answers, projected)
	}
	return result, nil
}

// ResolvePermission maps the existing tool-approval action IDs to the public
// typed response before durable admission.
func (runtime *Runtime) ResolvePermission(ctx context.Context, options agentrun.Options, interactionID, optionID string) error {
	choice := agent.PermissionChoice("")
	switch strings.TrimSpace(optionID) {
	case session.ToolApprovalAllowOnceOptionID:
		choice = agent.PermissionAllowOnce
	case session.ToolApprovalAllowWorkspaceOptionID:
		choice = agent.PermissionRemember
	case session.ToolApprovalDenyOptionID:
		choice = agent.PermissionDeny
	}
	_, _, err := runtime.ResolveInteraction(ctx, options, interactionID, agent.InteractionResponse{Permission: choice})
	return err
}
