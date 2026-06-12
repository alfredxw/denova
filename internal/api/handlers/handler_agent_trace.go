package handlers

import (
	"context"
	"strconv"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
)

// HandleAgentRunTraces GET /api/agent-runs — 返回最近 Agent run trace 摘要。
func (h *Handlers) HandleAgentRunTraces(ctx context.Context, c *app.RequestContext) {
	_ = ctx
	limit, _ := strconv.Atoi(c.Query("limit"))
	traces, err := h.app.AgentRunTraces(limit)
	if err != nil {
		writeError(c, consts.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, map[string]any{"runs": traces})
}

// HandleAgentRunTrace GET /api/agent-runs/:id — 返回单轮 Agent run trace 明细。
func (h *Handlers) HandleAgentRunTrace(ctx context.Context, c *app.RequestContext) {
	_ = ctx
	if !h.requireWorkspace(c) {
		return
	}
	id := c.Param("id")
	trace, err := h.app.AgentRunTrace(id)
	if err != nil {
		writeError(c, consts.StatusNotFound, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, trace)
}
