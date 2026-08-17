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

func (h *Handlers) HandleConfigManagerAskAnswer(ctx context.Context, c *app.RequestContext) {
	var request askAnswerRequest
	if err := c.BindJSON(&request); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidBody")
		return
	}
	result, err := h.app.ConfigManager().AnswerAsk(ctx, configManagerRequestFromQuery(c), strings.TrimSpace(c.Param("ask_id")), request.Answers)
	if err != nil {
		writeAskResolutionError(c, err)
		return
	}
	writeJSON(c, consts.StatusOK, result)
}

func (h *Handlers) HandleConfigManagerAskCancel(ctx context.Context, c *app.RequestContext) {
	var request askCancelRequest
	if err := c.BindJSON(&request); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidBody")
		return
	}
	result, err := h.app.ConfigManager().CancelAsk(ctx, configManagerRequestFromQuery(c), strings.TrimSpace(c.Param("ask_id")), request.Reason)
	if err != nil {
		writeAskResolutionError(c, err)
		return
	}
	writeJSON(c, consts.StatusOK, result)
}

func writeAskResolutionError(c *app.RequestContext, err error) {
	switch {
	case errors.Is(err, appsvc.ErrAgentAskNotFound):
		writeAgentRuntimeError(c, consts.StatusNotFound, "agent_runtime.ask_not_found", "未找到该问题 / Ask interaction not found", nil)
	case errors.Is(err, appsvc.ErrNoWorkspace):
		writeAgentRuntimeError(c, consts.StatusConflict, "agent_runtime.no_workspace", "尚未选择工作区 / No workspace is open", nil)
	default:
		writeAgentRuntimeError(c, consts.StatusBadRequest, "agent_runtime.invalid_ask_answer", "回答格式无效 / Invalid ask answer", map[string]any{"detail": err.Error()})
	}
}
