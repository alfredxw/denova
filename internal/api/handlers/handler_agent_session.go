package handlers

import (
	"context"
	"strconv"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	"denova/internal/api/agentui"
)

func (h *Handlers) HandleAgentSessionMessages(ctx context.Context, c *app.RequestContext) {
	if !h.app.HasWorkspace() {
		writeJSON(c, consts.StatusOK, []agentui.Message{})
		return
	}
	agentKind := strings.TrimSpace(c.Param("agent"))
	limitRaw := strings.TrimSpace(c.Query("limit"))
	if limitRaw != "" {
		limit, parseErr := strconv.Atoi(limitRaw)
		if parseErr != nil || limit <= 0 {
			writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidQuery")
			return
		}
		limit = min(limit, maxSessionMessagePageSize)
		before := -1
		if raw := strings.TrimSpace(c.Query("before")); raw != "" {
			before, parseErr = strconv.Atoi(raw)
			if parseErr != nil || before < 0 {
				writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidQuery")
				return
			}
		}
		page, err := h.app.AgentSessionMessagesPage(ctx, agentKind, before, limit)
		if err != nil {
			writeError(c, consts.StatusBadRequest, err.Error())
			return
		}
		writeJSON(c, consts.StatusOK, sessionMessagesPageDTO{
			Messages: agentui.MessagesFromHistoryAtOffset(page.Entries, page.NextBefore),
			Page:     sessionMessagePageMeta{NextBefore: strconv.Itoa(page.NextBefore), HasMore: page.HasMore, Total: page.Total},
		})
		return
	}
	entries, err := h.app.AgentSessionMessages(agentKind)
	if err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, agentui.MessagesFromHistory(entries))
}

func (h *Handlers) HandleAgentSessionClear(ctx context.Context, c *app.RequestContext) {
	if !h.requireWorkspace(c) {
		return
	}
	agentKind := strings.TrimSpace(c.Param("agent"))
	if err := h.app.ClearAgentSession(agentKind); err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, map[string]string{"status": "ok"})
}
