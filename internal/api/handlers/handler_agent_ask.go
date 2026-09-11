package handlers

import (
	"context"
	"errors"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	appsvc "denova/internal/app"
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
		writeAskResolutionError(c, err)
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
		writeAskResolutionError(c, err)
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
		writeAskResolutionError(c, err)
		return
	}
	writeJSON(c, consts.StatusOK, result)
}

func writeAskResolutionError(c *app.RequestContext, err error) {
	switch {
	case errors.Is(err, appsvc.ErrAgentAskNotFound):
		writeAgentRuntimeError(c, consts.StatusNotFound, "agent_runtime.ask_not_found", "Ask interaction not found", nil)
	case errors.Is(err, appsvc.ErrNoWorkspace):
		writeAgentRuntimeError(c, consts.StatusConflict, "agent_runtime.no_workspace", "No workspace is open", nil)
	default:
		writeAgentRuntimeError(c, consts.StatusBadRequest, "agent_runtime.invalid_ask_answer", "Invalid ask answer", map[string]any{"detail": err.Error()})
	}
}
