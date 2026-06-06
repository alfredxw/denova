package handlers

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
)

// handleStyles GET /api/styles — 返回用户级 styles/ 下可用的风格参考文件。
func (h *Handlers) HandleStyles(ctx context.Context, c *app.RequestContext) {
	if !h.app.HasWorkspace() {
		writeJSON(c, consts.StatusOK, map[string][]string{"styles": {}})
		return
	}
	styles, err := h.app.BookService().StyleFiles()
	if err != nil {
		writeErrorKey(c, consts.StatusInternalServerError, "api.styles.failed", "detail", err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, map[string][]string{"styles": styles})
}
