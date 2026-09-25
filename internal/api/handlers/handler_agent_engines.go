package handlers

import (
	"context"
	"errors"
	"log/slog"

	"denova/config"
	"denova/internal/agents/conversationconfig"
	agentruntime "denova/internal/agents/runtime"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
)

func (h *Handlers) HandleAgentEngines(_ context.Context, c *app.RequestContext) {
	writeJSON(c, consts.StatusOK, map[string]any{"items": h.app.AgentEngines().Catalog()})
}

func (h *Handlers) HandleAgentEngineCheck(ctx context.Context, c *app.RequestContext) {
	var body struct{}
	if err := decodeStrictJSONRequest(c.Request.Body(), &body); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequest")
		return
	}
	result, err := h.app.AgentEngines().Check(ctx, config.RuntimeID(c.Param("id")))
	if err != nil {
		writeEngineError(ctx, c, err)
		return
	}
	writeJSON(c, consts.StatusOK, result)
}

func (h *Handlers) HandleAgentEngineModels(ctx context.Context, c *app.RequestContext) {
	result, err := h.app.AgentEngines().Models(ctx, config.RuntimeID(c.Param("id")))
	if err != nil {
		writeEngineError(ctx, c, err)
		return
	}
	writeJSON(c, consts.StatusOK, result)
}

func writeEngineError(ctx context.Context, c *app.RequestContext, err error) {
	status, key := consts.StatusServiceUnavailable, "agentRuntime.connectionFailed"
	switch {
	case errors.Is(err, agentruntime.ErrEngineNotFound):
		status, key = consts.StatusNotFound, "agentRuntime.notFound"
	case errors.Is(err, conversationconfig.ErrRuntimeCapabilityUnsupported):
		status, key = consts.StatusBadRequest, "agentRuntime.capabilityUnsupported"
	case errors.Is(err, agentruntime.ErrOperationActive):
		status, key = consts.StatusConflict, "agentRuntime.busy"
	case errors.Is(err, agentruntime.ErrEngineModelUnavailable):
		status, key = consts.StatusUnprocessableEntity, "agentRuntime.modelUnavailable"
	case errors.Is(err, agentruntime.ErrEngineNotReady):
		status, key = consts.StatusConflict, "agentRuntime.notReady"
	case errors.Is(err, agentruntime.ErrEngineNotInstalled):
		key = "agentRuntime.notInstalled"
	case agentruntime.VersionUnsupportedReasonKey(err) != "":
		key = agentruntime.VersionUnsupportedReasonKey(err)
	}
	// Do not log raw authentication errors or URLs, which can contain secrets.
	slog.WarnContext(ctx, "Agent runtime API operation failed", "runtime", c.Param("id"), "reason", key)
	writeErrorKey(c, status, key, "detail", err.Error())
}
