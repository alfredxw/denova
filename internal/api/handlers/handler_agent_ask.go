package handlers

import (
	"context"
	"errors"
	"log/slog"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	"denova/internal/agents/runtime/external"
	appsvc "denova/internal/app"
	agent "github.com/alfredxw/denova/agent"
)

type askAnswerRequest struct {
	SessionID string                  `json:"session_id,omitempty"`
	Answers   []appsvc.AgentAskAnswer `json:"answers"`
}

type askCancelRequest struct {
	SessionID string `json:"session_id,omitempty"`
	Reason    string `json:"reason,omitempty"`
}

func (h *Handlers) HandleInteractiveAskAnswer(ctx context.Context, c *app.RequestContext) {
	h.handleInteractiveAsk(ctx, c, "answered")
}

func (h *Handlers) HandleInteractiveAskCancel(ctx context.Context, c *app.RequestContext) {
	h.handleInteractiveAsk(ctx, c, "cancelled")
}

func (h *Handlers) handleInteractiveAsk(ctx context.Context, c *app.RequestContext, status string) {
	if !h.requireWorkspace(c) {
		return
	}
	var request struct {
		StoryID  string                  `json:"story_id"`
		BranchID string                  `json:"branch_id"`
		Answers  []appsvc.AgentAskAnswer `json:"answers"`
		Reason   string                  `json:"reason,omitempty"`
	}
	if err := c.BindJSON(&request); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidBody")
		return
	}
	if strings.TrimSpace(request.StoryID) == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.interactive.storyIDRequired")
		return
	}
	result, err := h.app.ResolveInteractiveAsk(ctx, request.StoryID, request.BranchID, strings.TrimSpace(c.Param("ask_id")), status, request.Answers, request.Reason)
	if err != nil {
		writeAskResolutionError(ctx, c, err)
		return
	}
	writeJSON(c, consts.StatusOK, result)
}

func (h *Handlers) HandleSessionAskAnswer(ctx context.Context, c *app.RequestContext) {
	if !h.requireWorkspace(c) {
		return
	}
	var request askAnswerRequest
	if err := c.BindJSON(&request); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidBody")
		return
	}
	result, err := h.app.AnswerSessionAsk(ctx, request.SessionID, strings.TrimSpace(c.Param("ask_id")), request.Answers)
	if err != nil {
		writeAskResolutionError(ctx, c, err)
		return
	}
	writeJSON(c, consts.StatusOK, result)
}

func (h *Handlers) HandleSessionAskCancel(ctx context.Context, c *app.RequestContext) {
	if !h.requireWorkspace(c) {
		return
	}
	var request askCancelRequest
	if err := c.BindJSON(&request); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidBody")
		return
	}
	result, err := h.app.CancelSessionAsk(ctx, request.SessionID, strings.TrimSpace(c.Param("ask_id")), request.Reason)
	if err != nil {
		writeAskResolutionError(ctx, c, err)
		return
	}
	writeJSON(c, consts.StatusOK, result)
}

func writeAskResolutionError(ctx context.Context, c *app.RequestContext, err error) {
	slog.ErrorContext(ctx, "agent_interaction_resolution_failed", "interaction_id", c.Param("ask_id"), "error", err)
	status, code, key := consts.StatusInternalServerError, "agent_runtime.ask_failed", "api.ask.failed"
	switch {
	case errors.Is(err, agent.ErrDefinitionMismatch):
		status, code, key = consts.StatusConflict, "agent_runtime.definition_mismatch", "api.ask.definitionMismatch"
	case errors.Is(err, agent.ErrInteractionStale):
		status, code, key = consts.StatusConflict, "agent_runtime.ask_stale", "api.ask.stale"
	case errors.Is(err, agent.ErrInvalidInteractionResponse):
		status, code, key = consts.StatusBadRequest, "agent_runtime.invalid_ask_answer", "api.ask.invalidAnswer"
	case errors.Is(err, external.ErrAskConflict), errors.Is(err, agent.ErrIdempotencyConflict):
		status, code, key = consts.StatusConflict, "agent_runtime.ask_conflict", "api.ask.conflict"
	case errors.Is(err, appsvc.ErrAgentAskNotFound):
		status, code, key = consts.StatusNotFound, "agent_runtime.ask_not_found", "api.ask.notFound"
	case errors.Is(err, appsvc.ErrNoWorkspace):
		status, code, key = consts.StatusConflict, "agent_runtime.no_workspace", "api.workspace.noWorkspace"
	}
	writeAgentRuntimeError(c, status, code, messageKey(c, key), map[string]any{"operation": "agent.interaction.resolve", "detail": err.Error()})
}
