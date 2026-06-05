package handlers

import (
	"context"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	"nova/internal/api/sse"
	"nova/internal/book"
)

type loreAgentRequest struct {
	Instruction string   `json:"instruction"`
	References  []string `json:"references"`
}

type loreVersionCreateRequest struct {
	Message string `json:"message"`
}

func (h *Handlers) HandleLoreItems(ctx context.Context, c *app.RequestContext) {
	if !h.requireWorkspace(c) {
		return
	}
	items, err := h.app.LoreItems()
	if err != nil {
		writeError(c, consts.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, map[string]any{"items": items})
}

func (h *Handlers) HandleLoreItemCreate(ctx context.Context, c *app.RequestContext) {
	if !h.requireWorkspace(c) {
		return
	}
	var body book.LoreItemInput
	if err := c.BindJSON(&body); err != nil {
		writeError(c, consts.StatusBadRequest, "请求参数无效: "+err.Error())
		return
	}
	item, err := h.app.CreateLoreItem(body)
	if err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, item)
}

func (h *Handlers) HandleLoreItemUpdate(ctx context.Context, c *app.RequestContext) {
	if !h.requireWorkspace(c) {
		return
	}
	var body book.LoreItemInput
	if err := c.BindJSON(&body); err != nil {
		writeError(c, consts.StatusBadRequest, "请求参数无效: "+err.Error())
		return
	}
	item, err := h.app.UpdateLoreItem(c.Param("id"), body)
	if err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, item)
}

func (h *Handlers) HandleLoreItemDelete(ctx context.Context, c *app.RequestContext) {
	if !h.requireWorkspace(c) {
		return
	}
	if err := h.app.DeleteLoreItem(c.Param("id")); err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handlers) HandleLoreAgent(ctx context.Context, c *app.RequestContext) {
	if !h.requireWorkspace(c) {
		return
	}
	var body loreAgentRequest
	if err := c.BindJSON(&body); err != nil {
		writeError(c, consts.StatusBadRequest, "请求参数无效: "+err.Error())
		return
	}
	if strings.TrimSpace(body.Instruction) == "" {
		writeError(c, consts.StatusBadRequest, "资料库编辑指令不能为空")
		return
	}
	result, err := h.app.RunLoreAgent(ctx, body.Instruction, body.References)
	if err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, result)
}

func (h *Handlers) HandleLoreAgentStream(ctx context.Context, c *app.RequestContext) {
	if !h.requireWorkspace(c) {
		return
	}
	var body loreAgentRequest
	if err := c.BindJSON(&body); err != nil {
		writeError(c, consts.StatusBadRequest, "请求参数无效: "+err.Error())
		return
	}
	if strings.TrimSpace(body.Instruction) == "" {
		writeError(c, consts.StatusBadRequest, "资料库编辑指令不能为空")
		return
	}
	task := h.app.StartLoreAgentTask(body.Instruction, body.References)
	if task == nil {
		writeError(c, consts.StatusConflict, "尚未选择书籍工作区，请先在书籍管理页选择或创建书籍")
		return
	}
	sse.StreamTask(c, task)
}

func (h *Handlers) HandleLoreAgentMessages(ctx context.Context, c *app.RequestContext) {
	if !h.app.HasWorkspace() {
		writeJSON(c, consts.StatusOK, []messageDTO{})
		return
	}
	entries, err := h.app.LoreAgentMessages()
	if err != nil {
		writeError(c, consts.StatusInternalServerError, err.Error())
		return
	}
	result := make([]messageDTO, 0, len(entries))
	for _, entry := range entries {
		if entry.Type == "clear" {
			result = append(result, messageDTO{
				Type:      entry.Type,
				CreatedAt: formatTime(entry.CreatedAt),
			})
			continue
		}
		if entry.Content == "" {
			continue
		}
		result = append(result, messageDTO{
			Type:    entry.Type,
			Role:    entry.Role,
			Content: entry.Content,
		})
	}
	writeJSON(c, consts.StatusOK, result)
}

func (h *Handlers) HandleLoreAgentClear(ctx context.Context, c *app.RequestContext) {
	if !h.requireWorkspace(c) {
		return
	}
	if err := h.app.ClearLoreAgentSession(); err != nil {
		writeError(c, consts.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handlers) HandleLoreVersions(ctx context.Context, c *app.RequestContext) {
	if !h.requireWorkspace(c) {
		return
	}
	versions, err := h.app.LoreVersions()
	if err != nil {
		writeError(c, consts.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, map[string]any{"versions": versions})
}

func (h *Handlers) HandleLoreVersionCreate(ctx context.Context, c *app.RequestContext) {
	if !h.requireWorkspace(c) {
		return
	}
	var body loreVersionCreateRequest
	if err := c.BindJSON(&body); err != nil {
		writeError(c, consts.StatusBadRequest, "请求参数无效: "+err.Error())
		return
	}
	version, err := h.app.CreateLoreVersion(body.Message)
	if err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, version)
}

func (h *Handlers) HandleLoreVersionRestore(ctx context.Context, c *app.RequestContext) {
	if !h.requireWorkspace(c) {
		return
	}
	items, err := h.app.RestoreLoreVersion(c.Param("id"))
	if err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, map[string]any{"items": items})
}
