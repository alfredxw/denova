package handlers

import (
	"context"
	"fmt"
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
		writeError(c, consts.StatusBadRequest, "无效请求体")
		return
	}

	cmd := strings.TrimSpace(req.Command)
	if cmd == "" {
		writeError(c, consts.StatusBadRequest, "命令不能为空")
		return
	}

	var result string
	switch cmd {
	case "clear":
		if !h.requireWorkspace(c) {
			return
		}
		if err := h.app.ClearSession(); err != nil {
			result = fmt.Sprintf("清空失败: %v", err)
		} else {
			result = "上下文已清理，历史消息已保留"
		}
	case "status":
		if !h.requireWorkspace(c) {
			return
		}
		_, stateCtx := h.app.Status()
		if stateCtx == "" {
			result = "当前无作品状态数据，请先创建大纲"
		} else {
			result = stateCtx
		}
	case "help":
		result = helpText()
	default:
		writeError(c, consts.StatusBadRequest, fmt.Sprintf("未知命令: %s", cmd))
		return
	}

	writeJSON(c, consts.StatusOK, map[string]string{"result": result})
}

// helpText 返回帮助信息。
func helpText() string {
	return `可用命令:

  plan   — 先规划再执行（/plan <需求描述>）
  clear  — 清理当前 Agent 上下文并保留历史消息
  status — 显示当前作品状态
  help   — 显示此帮助信息

在聊天中直接输入创作想法即可开始与 Nova 对话。`
}
