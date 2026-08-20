package handlers

import (
	"context"
	"strconv"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	"denova/internal/interactive"
)

// HandleInteractiveMemoryBrowse 返回故事叙事记忆库浏览视图(含健康统计)。
// GET /api/interactive/stories/:id/memory?branch=&kind=&before_turn_id=
func (h *Handlers) HandleInteractiveMemoryBrowse(ctx context.Context, c *app.RequestContext) {
	view, err := h.app.BrowseInteractiveMemory(
		c.Param("id"),
		strings.TrimSpace(string(c.Query("branch"))),
		strings.TrimSpace(string(c.Query("kind"))),
		strings.TrimSpace(string(c.Query("before_turn_id"))),
	)
	if err != nil {
		writeError(c, consts.StatusNotFound, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, view)
}

// HandleInteractiveMemorySearch 跑完整记忆检索管线,返回带 Explain 的调试结果。
// GET /api/interactive/stories/:id/memory/search?q=&kind=&subject=&before_turn_id=&limit=
func (h *Handlers) HandleInteractiveMemorySearch(ctx context.Context, c *app.RequestContext) {
	req := interactive.MemorySearchRequest{
		Keywords:     splitMemoryQueryKeywords(string(c.Query("q"))),
		Kind:         strings.TrimSpace(string(c.Query("kind"))),
		Subject:      strings.TrimSpace(string(c.Query("subject"))),
		BeforeTurnID: strings.TrimSpace(string(c.Query("before_turn_id"))),
	}
	if limit, err := strconv.Atoi(strings.TrimSpace(string(c.Query("limit")))); err == nil && limit > 0 {
		req.Limit = limit
	}
	if hops, err := strconv.Atoi(strings.TrimSpace(string(c.Query("expand_hops")))); err == nil && hops > 0 {
		req.ExpandHops = hops
	}
	result, err := h.app.SearchInteractiveMemory(ctx, c.Param("id"), strings.TrimSpace(string(c.Query("branch"))), req)
	if err != nil {
		writeError(c, consts.StatusNotFound, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, result)
}

// splitMemoryQueryKeywords 把空格分隔的查询拆为关键词列表。
func splitMemoryQueryKeywords(query string) []string {
	fields := strings.Fields(strings.TrimSpace(query))
	if len(fields) == 0 {
		return nil
	}
	return fields
}
