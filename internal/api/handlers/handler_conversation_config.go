package handlers

import (
	"context"
	"denova/config"
	"errors"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	"denova/internal/agents/conversationconfig"
	"denova/internal/agents/external/codex"
	externaljournal "denova/internal/agents/external/journal"
	appsvc "denova/internal/app"
	"denova/internal/app/agentruntime"
)

func (h *Handlers) HandleConversationConfigGet(ctx context.Context, c *app.RequestContext) {
	binding, ok := conversationConfigBindingFromQuery(c)
	if !ok {
		return
	}
	snapshot, err := h.app.ConversationConfig(ctx, binding)
	if err != nil {
		writeConversationConfigError(c, err)
		return
	}
	h.writeConversationConfigSnapshot(c, snapshot)
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
	if !bindConversationConfigProject(c, &body.Binding) {
		return
	}
	snapshot, err := h.app.PatchConversationConfig(ctx, body.Binding, body.Changes, body.BaseRevision)
	if err != nil {
		writeConversationConfigError(c, err)
		return
	}
	h.writeConversationConfigSnapshot(c, snapshot)
}

// Runtime health and capabilities are read-only host projections. They never
// enter the durable configuration or trigger an external process on GET.
func (h *Handlers) writeConversationConfigSnapshot(c *app.RequestContext, snapshot conversationconfig.Snapshot) {
	selection := snapshot.Engine()
	snapshot.Runtime = &selection
	for _, descriptor := range h.app.AgentEngines().Catalog() {
		if descriptor.ID != selection.Kind {
			continue
		}
		writeJSON(c, consts.StatusOK, struct {
			conversationconfig.Snapshot
			Capabilities agentruntime.EngineCapabilities `json:"runtime_capabilities"`
			Status       string                          `json:"runtime_status"`
		}{snapshot, descriptor.Capabilities, descriptor.Status})
		return
	}
	writeErrorKey(c, consts.StatusNotFound, "agentRuntime.notFound")
}

func conversationConfigBindingFromQuery(c *app.RequestContext) (appsvc.ConversationConfigBinding, bool) {
	binding := appsvc.ConversationConfigBinding{
		Mode: c.Query("mode"), ProjectID: c.Query("project_id"), SessionID: c.Query("session_id"),
		StoryID: c.Query("story_id"), BranchID: c.Query("branch_id"),
		Origin: c.Query("origin"), ResourceID: c.Query("resource_id"), RunID: c.Query("run_id"),
	}
	return binding, bindConversationConfigProject(c, &binding)
}

func bindConversationConfigProject(c *app.RequestContext, binding *appsvc.ConversationConfigBinding) bool {
	if binding == nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidBody")
		return false
	}
	if scope := projectScope(c); scope.ProjectID != "" {
		binding.ProjectID = scope.ProjectID
		return true
	}
	writeErrorKey(c, consts.StatusBadRequest, "api.project.idRequired")
	return false
}

func writeConversationConfigError(c *app.RequestContext, err error) {
	switch {
	case errors.Is(err, config.ErrRuntimeModelProfile):
		writeErrorKey(c, consts.StatusUnprocessableEntity, "agentRuntime.apiProfileUnavailable")
	case errors.Is(err, agentruntime.ErrEngineNotFound):
		writeErrorKey(c, consts.StatusNotFound, "agentRuntime.notFound")
	case errors.Is(err, agentruntime.ErrEngineNotInstalled):
		writeErrorKey(c, consts.StatusServiceUnavailable, "agentRuntime.notInstalled")
	case errors.Is(err, codex.ErrVersionUnsupported):
		writeErrorKey(c, consts.StatusServiceUnavailable, "agentRuntime.incompatibleVersion")
	case errors.Is(err, agentruntime.ErrOperationActive), errors.Is(err, externaljournal.ErrBusy):
		writeErrorKey(c, consts.StatusConflict, "agentRuntime.busy")
	case errors.Is(err, conversationconfig.ErrRuntimeCapabilityUnsupported):
		writeErrorKey(c, consts.StatusBadRequest, "agentRuntime.capabilityUnsupported")
	case errors.Is(err, agentruntime.ErrEngineNotReady):
		writeErrorKey(c, consts.StatusConflict, "agentRuntime.notReady")
	case errors.Is(err, agentruntime.ErrEngineModelUnavailable):
		writeErrorKey(c, consts.StatusUnprocessableEntity, "agentRuntime.modelUnavailable")
	case errors.Is(err, appsvc.ErrConversationModelDefaultsNotSaved):
		writeErrorKey(c, consts.StatusInternalServerError, "api.conversationConfig.rememberModelFailed")
	case appsvc.IsConversationConfigRevisionConflict(err):
		writeErrorKey(c, consts.StatusConflict, "api.conversationConfig.revisionConflict")
	case errors.Is(err, appsvc.ErrNoWorkspace), errors.Is(err, appsvc.ErrNoWorkspaceOpen):
		writeErrorKey(c, consts.StatusBadRequest, "api.settings.workspaceMissing")
	default:
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
	}
}
