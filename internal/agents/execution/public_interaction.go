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
// response vocabulary. Only the owning Agent journal accepts the answer.
func (runtime *Runtime) ResolveAsk(
	ctx context.Context,
	options agentrun.Options,
	askID, status string,
	answers []agentconversation.HostAskAnswer,
	cancelReason string,
) (agentconversation.HostAskResolution, error) {
	response := agent.InteractionResponse{Cancelled: status == session.AskCancelled}
	if !response.Cancelled && len(answers) == 1 && answers[0].QuestionID == "tool-approval" {
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
		response.Answers = agentconversation.InteractionAnswers(answers)
	}
	request, resolution, err := runtime.ResolveInteraction(ctx, options, askID, response)
	if err != nil {
		return agentconversation.HostAskResolution{}, err
	}
	result := agentconversation.HostAskResolution{Schema: "ask.result.v1", ID: askID}
	if request.Verification != nil && resolution.Cancelled && cancelReason == "task_aborted" {
		root, _, err := runtime.public.openSession(ctx, options)
		if err != nil {
			return agentconversation.HostAskResolution{}, err
		}
		if _, err := runtime.public.agent.AbortTree(ctx, root.Key(), agent.AbortRequest{
			IdempotencyKey: "abort-verification:" + askID, Reason: "User cancelled the task during effect verification",
		}); err != nil {
			return agentconversation.HostAskResolution{}, err
		}
		result.Status, result.CancelReason = session.AskCancelled, cancelReason
		return result, nil
	}
	if request.Verification != nil && (resolution.Cancelled || len(resolution.Answers) != 1 || len(resolution.Answers[0].Values) != 1 ||
		(resolution.Answers[0].Values[0] != "executed" && resolution.Answers[0].Values[0] != "not_executed")) {
		result.Status = session.AskPending
		return result, nil
	}
	return agentconversation.ProjectAskResolution(request, resolution, cancelReason), nil
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
