package handlers

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	appsvc "denova/internal/app"
)

func (h *Handlers) HandleConversationGoalGet(ctx context.Context, c *app.RequestContext) {
	binding, ok := conversationConfigBindingFromQuery(c)
	if !ok {
		return
	}
	current, found, err := h.app.ConversationGoal(ctx, binding)
	if err != nil {
		writeConversationGoalError(c, err)
		return
	}
	if !found {
		writeJSON(c, consts.StatusOK, map[string]any{"goal": nil})
		return
	}
	writeJSON(c, consts.StatusOK, map[string]any{"goal": current})
}

func (h *Handlers) HandleConversationGoalMutate(ctx context.Context, c *app.RequestContext) {
	var body struct {
		Binding appsvc.ConversationConfigBinding `json:"binding"`
		appsvc.ConversationGoalMutation
	}
	if err := decodeStrictJSONRequest(c.Request.Body(), &body); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
		return
	}
	if !bindConversationConfigProject(c, &body.Binding) {
		return
	}
	next, err := h.app.MutateConversationGoal(ctx, body.Binding, body.ConversationGoalMutation)
	if err != nil {
		writeConversationGoalError(c, err)
		return
	}
	writeJSON(c, consts.StatusOK, map[string]any{"goal": next})
}

func writeConversationGoalError(c *app.RequestContext, err error) {
	switch {
	case appsvc.IsConversationGoalRevisionConflict(err):
		writeErrorKey(c, consts.StatusConflict, "api.goal.revisionConflict")
	case appsvc.IsConversationGoalStateChanged(err):
		writeErrorKey(c, consts.StatusConflict, "api.goal.stateChanged")
	default:
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
	}
}
