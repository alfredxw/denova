package handlers

import (
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	"nova/internal/i18n"
)

// writeJSON 写入 JSON 响应。
func writeJSON(c *app.RequestContext, code int, obj interface{}) {
	c.JSON(code, obj)
}

// writeError 写入错误响应。
func writeError(c *app.RequestContext, code int, msg string) {
	c.JSON(code, map[string]string{"error": msg})
}

func requestLocalizer(c *app.RequestContext) i18n.Localizer {
	return i18n.FromHeader(string(c.Request.Header.Peek("X-Nova-Locale")))
}

func requestLocale(c *app.RequestContext) string {
	header := strings.TrimSpace(string(c.Request.Header.Peek("X-Nova-Locale")))
	if header == "" {
		return ""
	}
	return i18n.FromHeader(header).Locale()
}

func writeErrorKey(c *app.RequestContext, code int, key string, args ...any) {
	writeError(c, code, requestLocalizer(c).T(key, args...))
}

func messageKey(c *app.RequestContext, key string, args ...any) string {
	return requestLocalizer(c).T(key, args...)
}

// requireWorkspace 校验当前 App 是否已绑定 workspace；
// 未绑定时直接写入 409 错误并返回 false，由调用方 return 终止处理。
func (h *Handlers) requireWorkspace(c *app.RequestContext) bool {
	if h.app.HasWorkspace() {
		return true
	}
	writeErrorKey(c, consts.StatusConflict, "api.workspace.noWorkspace")
	return false
}
