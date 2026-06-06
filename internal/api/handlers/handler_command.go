package handlers

import (
	"context"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
)

// commandRequest POST /api/command 请求体。
type commandRequest struct {
	Command string `json:"command"`
}

// handleCommand POST /api/command — 执行内置命令。
func (h *Handlers) HandleCommand(ctx context.Context, c *app.RequestContext) {
	var req commandRequest
	if err := c.BindJSON(&req); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidBody")
		return
	}

	cmd := strings.TrimSpace(req.Command)
	if cmd == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.command.empty")
		return
	}

	var result string
	localizer := requestLocalizer(c)
	switch cmd {
	case "clear":
		if !h.requireWorkspace(c) {
			return
		}
		if err := h.app.ClearSession(); err != nil {
			result = localizer.T("api.command.clearFailed", "detail", err.Error())
		} else {
			result = localizer.T("api.command.cleared")
		}
	case "status":
		if !h.requireWorkspace(c) {
			return
		}
		_, stateCtx := h.app.Status()
		if stateCtx == "" {
			result = localizer.T("api.command.noStatus")
		} else {
			result = stateCtx
		}
	case "help":
		result = localizer.T("api.command.help")
	default:
		writeErrorKey(c, consts.StatusBadRequest, "api.command.unknown", "command", cmd)
		return
	}

	writeJSON(c, consts.StatusOK, map[string]string{"result": result})
}
