package handlers

import (
	"context"
	"errors"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	appsvc "denova/internal/app"
)

func (h *Handlers) HandleConversationConfigGet(ctx context.Context, c *app.RequestContext) {
	snapshot, err := h.app.ConversationConfig(ctx, conversationConfigBindingFromQuery(c))
	if err != nil {
		writeConversationConfigError(c, err)
		return
	}
	writeJSON(c, consts.StatusOK, snapshot)
}

func (h *Handlers) HandleConversationConfigPatch(ctx context.Context, c *app.RequestContext) {
	var body struct {
		Binding      appsvc.ConversationConfigBinding `json:"binding"`
		BaseRevision uint64                           `json:"base_revision"`
		Changes      appsvc.ConversationConfigPatch   `json:"changes"`
	}
	if err := decodeStrictJSONRequest(c.Request.Body(), &body); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
		return
	}
	snapshot, err := h.app.PatchConversationConfig(ctx, body.Binding, body.Changes, body.BaseRevision)
	if err != nil {
		writeConversationConfigError(c, err)
		return
	}
	writeJSON(c, consts.StatusOK, snapshot)
}

func conversationConfigBindingFromQuery(c *app.RequestContext) appsvc.ConversationConfigBinding {
	return appsvc.ConversationConfigBinding{
		Mode: c.Query("mode"), ProjectID: c.Query("project_id"), SessionID: c.Query("session_id"),
		StoryID: c.Query("story_id"), BranchID: c.Query("branch_id"),
		Origin: c.Query("origin"), ResourceID: c.Query("resource_id"), RunID: c.Query("run_id"),
	}
}

func writeConversationConfigError(c *app.RequestContext, err error) {
	switch {
	case appsvc.IsConversationConfigRevisionConflict(err):
		writeErrorKey(c, consts.StatusConflict, "api.conversationConfig.revisionConflict")
	case errors.Is(err, appsvc.ErrAgentOperationActive):
		writeErrorKey(c, consts.StatusConflict, "api.conversationConfig.active")
	case errors.Is(err, appsvc.ErrNoWorkspace), errors.Is(err, appsvc.ErrNoWorkspaceOpen):
		writeErrorKey(c, consts.StatusBadRequest, "api.settings.workspaceMissing")
	default:
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
	}
}
