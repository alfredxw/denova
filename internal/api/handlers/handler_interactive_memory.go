package handlers

import (
	"context"
	"encoding/json"
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

// HandleInteractiveMemoryAppend 直接往叙事记忆事件日志注入一条事件。
// POST /api/interactive/stories/:id/memory?branch=
//
// 请求体:{"source_turn_id":"...","records":[...]}。
//
// 落库前会走 Store 写入路径的全部校验与实体对齐,所以面板、检索、向量召回
// 看到的与模型抽取产出无差别。返回完整事件,含 trace 与对齐留痕。
func (h *Handlers) HandleInteractiveMemoryAppend(ctx context.Context, c *app.RequestContext) {
	body := c.Request.Body()
	if len(body) == 0 {
		writeError(c, consts.StatusBadRequest, "请求体为空")
		return
	}
	var payload struct {
		SourceTurnID string                               `json:"source_turn_id"`
		Records      []interactive.NarrativeMemoryRecord  `json:"records"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		writeError(c, consts.StatusBadRequest, "解析请求体失败: "+err.Error())
		return
	}
	if strings.TrimSpace(payload.SourceTurnID) == "" {
		writeError(c, consts.StatusBadRequest, "source_turn_id 必填且必须指向当前分支上的回合")
		return
	}
	if len(payload.Records) == 0 {
		writeError(c, consts.StatusBadRequest, "records 不能为空")
		return
	}
	event, err := h.app.AppendInteractiveMemory(
		c.Param("id"),
		strings.TrimSpace(string(c.Query("branch"))),
		interactive.NarrativeMemoryEvent{
			SourceTurnID: payload.SourceTurnID,
			Records:      payload.Records,
		},
	)
	if err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, event)
}

// splitMemoryQueryKeywords 把空格分隔的查询拆为关键词列表。
func splitMemoryQueryKeywords(query string) []string {
	fields := strings.Fields(strings.TrimSpace(query))
	if len(fields) == 0 {
		return nil
	}
	return fields
}
