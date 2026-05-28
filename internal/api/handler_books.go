package api

import (
	"context"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
)

// handleBooks GET /api/books — 返回最近打开的书籍工作目录。
func (s *Server) handleBooks(ctx context.Context, c *app.RequestContext) {
	writeJSON(c, consts.StatusOK, map[string]interface{}{
		"books": s.app.Books(),
	})
}

// handleCreateBook POST /api/books/create — 创建新书籍工作区。
func (s *Server) handleCreateBook(ctx context.Context, c *app.RequestContext) {
	var req struct {
		Title       string `json:"title"`
		Author      string `json:"author,omitempty"`
		Description string `json:"description,omitempty"`
	}
	if err := c.BindJSON(&req); err != nil {
		writeError(c, consts.StatusBadRequest, "请求参数无效")
		return
	}
	if req.Title == "" {
		writeError(c, consts.StatusBadRequest, "title 不能为空")
		return
	}
	layered, err := s.app.Settings()
	if err != nil {
		writeError(c, consts.StatusInternalServerError, err.Error())
		return
	}
	if layered.Paths.NovaDir == "" {
		writeError(c, consts.StatusInternalServerError, "Nova 数据目录未配置")
		return
	}
	workspace, meta, err := s.app.CreateBook(ctx, layered.Paths.NovaDir, req.Title, req.Author, req.Description)
	if err != nil {
		status := consts.StatusInternalServerError
		if strings.Contains(err.Error(), "已存在") {
			status = consts.StatusConflict
		}
		writeError(c, status, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, map[string]interface{}{
		"workspace": workspace,
		"book_meta": meta,
	})
}

// handleBookRemove POST /api/books/remove — 移除书籍记录，不删除磁盘目录。
func (s *Server) handleBookRemove(ctx context.Context, c *app.RequestContext) {
	var req struct {
		Path string `json:"path"`
	}
	if err := c.BindJSON(&req); err != nil || req.Path == "" {
		writeError(c, consts.StatusBadRequest, "请提供 path 参数")
		return
	}
	if err := s.app.RemoveBook(req.Path); err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, map[string]string{"message": "已移除书籍记录"})
}

// handleBookInfo GET /api/books/info — 读取指定工作区的书籍元信息。
func (s *Server) handleBookInfo(ctx context.Context, c *app.RequestContext) {
	path := string(c.Query("path"))
	if path == "" {
		writeError(c, consts.StatusBadRequest, "path 参数不能为空")
		return
	}
	meta, err := s.app.BookInfo(path)
	if err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, meta)
}

// handleUpdateBookInfo PUT /api/books/info — 更新指定工作区的书籍元信息。
func (s *Server) handleUpdateBookInfo(ctx context.Context, c *app.RequestContext) {
	var req struct {
		Path        string `json:"path"`
		Title       string `json:"title"`
		Author      string `json:"author"`
		Description string `json:"description"`
	}
	if err := c.BindJSON(&req); err != nil {
		writeError(c, consts.StatusBadRequest, "请求参数无效")
		return
	}
	if req.Path == "" {
		writeError(c, consts.StatusBadRequest, "path 不能为空")
		return
	}
	meta, err := s.app.UpdateBookInfo(req.Path, req.Title, req.Author, req.Description)
	if err != nil {
		writeError(c, consts.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, meta)
}
