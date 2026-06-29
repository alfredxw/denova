package handlers

import (
	"context"
	"os"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	novaApp "nova/internal/app"
)

// handleBooks GET /api/books — 返回当前 Nova 数据目录下实际存在的书籍工作目录。
func (h *Handlers) HandleBooks(ctx context.Context, c *app.RequestContext) {
	writeJSON(c, consts.StatusOK, map[string]interface{}{
		"books": h.app.Books(),
	})
}

// handleCreateBook POST /api/books/create — 创建新书籍工作区。
func (h *Handlers) HandleCreateBook(ctx context.Context, c *app.RequestContext) {
	var req struct {
		Title       string `json:"title"`
		Author      string `json:"author,omitempty"`
		Description string `json:"description,omitempty"`
	}
	if err := c.BindJSON(&req); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequest")
		return
	}
	if req.Title == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.books.titleRequired")
		return
	}
	layered, err := h.app.Settings()
	if err != nil {
		writeError(c, consts.StatusInternalServerError, err.Error())
		return
	}
	if layered.Paths.NovaDir == "" {
		writeErrorKey(c, consts.StatusInternalServerError, "api.books.novaDirMissing")
		return
	}
	workspace, meta, err := h.app.CreateBook(ctx, layered.Paths.NovaDir, req.Title, req.Author, req.Description)
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

// HandleBookCover GET /api/books/cover?path=... — 读取指定书籍固定封面。
func (h *Handlers) HandleBookCover(ctx context.Context, c *app.RequestContext) {
	path := string(c.Query("path"))
	if path == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.pathRequired")
		return
	}
	data, contentType, err := h.app.ReadBookCover(path)
	if err != nil {
		status := consts.StatusBadRequest
		if os.IsNotExist(err) {
			status = consts.StatusNotFound
		}
		writeError(c, status, err.Error())
		return
	}
	c.Data(consts.StatusOK, contentType, data)
}

// HandleBookCoverGenerate POST /api/books/cover/generate — 为指定书籍生成并应用封面。
func (h *Handlers) HandleBookCoverGenerate(ctx context.Context, c *app.RequestContext) {
	var req novaApp.BookCoverGenerateRequest
	if err := c.BindJSON(&req); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequest")
		return
	}
	if req.Path == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.pathRequired")
		return
	}
	result, err := h.app.GenerateBookCover(ctx, req)
	if err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, result)
}

// handleBookRemove POST /api/books/remove — 移除书籍记录，不删除磁盘目录。
func (h *Handlers) HandleBookRemove(ctx context.Context, c *app.RequestContext) {
	var req struct {
		Path string `json:"path"`
	}
	if err := c.BindJSON(&req); err != nil || req.Path == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.pathRequired")
		return
	}
	workspace, err := h.app.RemoveBook(req.Path)
	if err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, map[string]string{
		"message":   messageKey(c, "api.books.removed"),
		"workspace": workspace,
	})
}

// handleBookReorder POST /api/books/reorder — 保存书籍管理页自定义排序。
func (h *Handlers) HandleBookReorder(ctx context.Context, c *app.RequestContext) {
	var req struct {
		Paths []string `json:"paths"`
	}
	if err := c.BindJSON(&req); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequest")
		return
	}
	if err := h.app.ReorderBooks(req.Paths); err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, map[string]string{"message": messageKey(c, "api.books.reordered")})
}

// handleBookInfo GET /api/books/info — 读取指定工作区的书籍元信息。
func (h *Handlers) HandleBookInfo(ctx context.Context, c *app.RequestContext) {
	path := string(c.Query("path"))
	if path == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.books.pathQueryRequired")
		return
	}
	meta, err := h.app.BookInfo(path)
	if err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, meta)
}

// handleUpdateBookInfo PUT /api/books/info — 更新指定工作区的书籍元信息。
func (h *Handlers) HandleUpdateBookInfo(ctx context.Context, c *app.RequestContext) {
	var req struct {
		Path        string `json:"path"`
		Title       string `json:"title"`
		Author      string `json:"author"`
		Description string `json:"description"`
	}
	if err := c.BindJSON(&req); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequest")
		return
	}
	if req.Path == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.books.pathRequired")
		return
	}
	meta, err := h.app.UpdateBookInfo(req.Path, req.Title, req.Author, req.Description)
	if err != nil {
		writeError(c, consts.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, meta)
}
