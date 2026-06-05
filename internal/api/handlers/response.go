package handlers

import (
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
)

// writeJSON 写入 JSON 响应。
func writeJSON(c *app.RequestContext, code int, obj interface{}) {
	c.JSON(code, obj)
}

// writeError 写入错误响应。
func writeError(c *app.RequestContext, code int, msg string) {
	c.JSON(code, map[string]string{"error": msg})
}

// requireWorkspace 校验当前 App 是否已绑定 workspace；
// 未绑定时直接写入 409 错误并返回 false，由调用方 return 终止处理。
func (h *Handlers) requireWorkspace(c *app.RequestContext) bool {
	if h.app.HasWorkspace() {
		return true
	}
	writeError(c, consts.StatusConflict, "尚未选择书籍工作区，请先在书籍管理页选择或创建书籍")
	return false
}
