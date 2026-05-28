package api

import (
	"context"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	"nova/internal/book"
)

type loreAgentRequest struct {
	Instruction string   `json:"instruction"`
	References  []string `json:"references"`
}

type loreVersionCreateRequest struct {
	Message string `json:"message"`
}

func (s *Server) handleLoreItems(ctx context.Context, c *app.RequestContext) {
	if !s.requireWorkspace(c) {
		return
	}
	items, err := s.app.LoreItems()
	if err != nil {
		writeError(c, consts.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, map[string]any{"items": items})
}

func (s *Server) handleLoreItemCreate(ctx context.Context, c *app.RequestContext) {
	if !s.requireWorkspace(c) {
		return
	}
	var body book.LoreItemInput
	if err := c.BindJSON(&body); err != nil {
		writeError(c, consts.StatusBadRequest, "请求参数无效: "+err.Error())
		return
	}
	item, err := s.app.CreateLoreItem(body)
	if err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, item)
}

func (s *Server) handleLoreItemUpdate(ctx context.Context, c *app.RequestContext) {
	if !s.requireWorkspace(c) {
		return
	}
	var body book.LoreItemInput
	if err := c.BindJSON(&body); err != nil {
		writeError(c, consts.StatusBadRequest, "请求参数无效: "+err.Error())
		return
	}
	item, err := s.app.UpdateLoreItem(c.Param("id"), body)
	if err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, item)
}

func (s *Server) handleLoreItemDelete(ctx context.Context, c *app.RequestContext) {
	if !s.requireWorkspace(c) {
		return
	}
	if err := s.app.DeleteLoreItem(c.Param("id")); err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleLoreAgent(ctx context.Context, c *app.RequestContext) {
	if !s.requireWorkspace(c) {
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
	result, err := s.app.RunLoreAgent(ctx, body.Instruction, body.References)
	if err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, result)
}

func (s *Server) handleLoreAgentStream(ctx context.Context, c *app.RequestContext) {
	if !s.requireWorkspace(c) {
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
	task := s.app.StartLoreAgentTask(body.Instruction, body.References)
	if task == nil {
		writeError(c, consts.StatusConflict, "尚未选择书籍工作区，请先在书籍管理页选择或创建书籍")
		return
	}
	streamTask(c, task)
}

func (s *Server) handleLoreAgentMessages(ctx context.Context, c *app.RequestContext) {
	if !s.app.HasWorkspace() {
		writeJSON(c, consts.StatusOK, []messageDTO{})
		return
	}
	entries, err := s.app.LoreAgentMessages()
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

func (s *Server) handleLoreAgentClear(ctx context.Context, c *app.RequestContext) {
	if !s.requireWorkspace(c) {
		return
	}
	if err := s.app.ClearLoreAgentSession(); err != nil {
		writeError(c, consts.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleLoreVersions(ctx context.Context, c *app.RequestContext) {
	if !s.requireWorkspace(c) {
		return
	}
	versions, err := s.app.LoreVersions()
	if err != nil {
		writeError(c, consts.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, map[string]any{"versions": versions})
}

func (s *Server) handleLoreVersionCreate(ctx context.Context, c *app.RequestContext) {
	if !s.requireWorkspace(c) {
		return
	}
	var body loreVersionCreateRequest
	if err := c.BindJSON(&body); err != nil {
		writeError(c, consts.StatusBadRequest, "请求参数无效: "+err.Error())
		return
	}
	version, err := s.app.CreateLoreVersion(body.Message)
	if err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, version)
}

func (s *Server) handleLoreVersionRestore(ctx context.Context, c *app.RequestContext) {
	if !s.requireWorkspace(c) {
		return
	}
	items, err := s.app.RestoreLoreVersion(c.Param("id"))
	if err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, map[string]any{"items": items})
}
