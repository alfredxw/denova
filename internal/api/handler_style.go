package api

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
)

// handleStyles GET /api/styles — 返回用户级 styles/ 下可用的风格参考文件。
func (s *Server) handleStyles(ctx context.Context, c *app.RequestContext) {
	if !s.app.HasWorkspace() {
		writeJSON(c, consts.StatusOK, map[string][]string{"styles": {}})
		return
	}
	styles, err := s.app.BookService().StyleFiles()
	if err != nil {
		writeError(c, consts.StatusInternalServerError, "获取风格参考失败: "+err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, map[string][]string{"styles": styles})
}
