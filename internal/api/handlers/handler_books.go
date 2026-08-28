package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"os"
	"os/user"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	bookapp "denova/internal/app/book"
	imageapp "denova/internal/app/image"
	appsettings "denova/internal/app/settings"
	"denova/internal/book"
	projectdomain "denova/internal/project"
)

const MaxBookCoverUploadBytes int64 = 16 * 1024 * 1024

// handleBooks GET /api/books — 返回当前 Nova 数据目录下实际存在的书籍工作目录。
func (h *Handlers) HandleBooks(ctx context.Context, c *app.RequestContext) {
	writeJSON(c, consts.StatusOK, map[string]interface{}{
		"books":     h.app.BookAssets().Books(),
		"sort_mode": h.app.BookAssets().SortMode(),
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
	req.Author = defaultBookAuthor(req.Author)
	layered, err := h.app.SettingsService().Snapshot(appsettings.Global())
	if err != nil {
		writeError(c, consts.StatusInternalServerError, err.Error())
		return
	}
	if layered.Paths.DenovaDir == "" {
		writeErrorKey(c, consts.StatusInternalServerError, "api.books.novaDirMissing")
		return
	}
	created, err := h.app.CreateBook(ctx, layered.Paths.DenovaDir, req.Title, req.Author, req.Description)
	if err != nil {
		status := consts.StatusInternalServerError
		if strings.Contains(err.Error(), "已存在") {
			status = consts.StatusConflict
		}
		writeError(c, status, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, map[string]interface{}{
		"project_id": created.ProjectID,
		"workspace":  created.Workspace,
		"book_meta":  created.Meta,
	})
}

func defaultBookAuthor(author string) string {
	current, _ := user.Current()
	return resolveBookAuthor(author, current)
}

func resolveBookAuthor(author string, current *user.User) string {
	if author = strings.TrimSpace(author); author != "" {
		return author
	}
	if current != nil {
		if name := strings.TrimSpace(current.Name); name != "" {
			return name
		}
		if username := strings.TrimSpace(current.Username); username != "" {
			return username
		}
	}
	return "User"
}

// HandleBookCover GET /api/books/cover?path=... — 读取指定书籍固定封面。
func (h *Handlers) HandleBookCover(ctx context.Context, c *app.RequestContext) {
	path := string(c.Query("path"))
	if path == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.pathRequired")
		return
	}
	data, contentType, err := h.app.BookAssets().ReadCover(path)
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
	var req bookapp.CoverGenerateRequest
	if err := c.BindJSON(&req); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequest")
		return
	}
	if req.Path == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.pathRequired")
		return
	}
	if strings.TrimSpace(req.Mode) == "agent" {
		meta, err := h.app.BookAssets().Info(req.Path)
		if err != nil {
			writeError(c, consts.StatusBadRequest, err.Error())
			return
		}
		projectID, err := h.app.BookAssets().ProjectIDForPath(req.Path)
		if err != nil {
			writeError(c, consts.StatusBadRequest, err.Error())
			return
		}
		source, err := json.Marshal(map[string]string{
			"title": meta.Title, "author": meta.Author, "description": meta.Description,
			"additional_user_requirements": strings.TrimSpace(req.Instruction),
		})
		if err != nil {
			writeError(c, consts.StatusInternalServerError, err.Error())
			return
		}
		agentResult, err := h.app.Images().GenerateProjectWithAgent(ctx, projectID, imageapp.AgentGenerateRequest{
			CommandID: req.CommandID, Purpose: "book_cover", SourceContext: string(source), ImagePresetID: req.ImagePresetID,
			SystemPrompt: "Generate exactly one vertical book-cover image grounded in the supplied book metadata. Do not generate text, titles, author names, watermarks, logos, UI panels, or QR codes.",
			AltText:      "Book cover: " + meta.Title,
		})
		if err != nil {
			writeError(c, consts.StatusBadRequest, err.Error())
			return
		}
		writeJSON(c, consts.StatusOK, agentResult.BookCover)
		return
	}
	result, err := h.app.BookAssets().GenerateCover(ctx, req)
	if err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, result)
}

// HandleBookCoverUpload POST /api/books/cover/upload — 上传并应用指定书籍固定封面。
func (h *Handlers) HandleBookCoverUpload(ctx context.Context, c *app.RequestContext) {
	path := strings.TrimSpace(string(c.FormValue("path")))
	if path == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.books.pathRequired")
		return
	}
	filename, data, ok := readBookCoverUpload(c)
	if !ok {
		return
	}
	result, err := h.app.BookAssets().UploadCover(path, filename, data)
	if err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, result)
}

// HandleBookExport GET /api/books/export?path=...&format=txt — 导出指定书籍。
func (h *Handlers) HandleBookExport(ctx context.Context, c *app.RequestContext) {
	path := strings.TrimSpace(string(c.Query("path")))
	format := strings.TrimSpace(string(c.Query("format")))
	if path == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.books.pathQueryRequired")
		return
	}
	if format == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.books.exportFormatRequired")
		return
	}

	result, err := h.app.BookAssets().Export(bookapp.BookExportRequest{
		Path:   path,
		Format: bookapp.BookExportFormat(format),
	})
	if err != nil {
		switch {
		case errors.Is(err, bookapp.ErrUnsupportedBookExportFormat):
			writeErrorKey(c, consts.StatusBadRequest, "api.books.exportFormatUnsupported", "format", format)
		case errors.Is(err, book.ErrNoExportableChapters):
			writeErrorKey(c, consts.StatusBadRequest, "api.books.exportNoChapters")
		default:
			writeError(c, consts.StatusBadRequest, err.Error())
		}
		return
	}
	c.Response.Header.Set("Content-Disposition", attachmentContentDisposition(result.Filename))
	c.Response.Header.Set("Cache-Control", "no-store")
	c.Data(consts.StatusOK, result.ContentType, result.Data)
}

func attachmentContentDisposition(filename string) string {
	fallback := asciiDownloadFilename(filename)
	encoded := strings.ReplaceAll(url.PathEscape(filename), "+", "%20")
	return `attachment; filename="` + fallback + `"; filename*=UTF-8''` + encoded
}

func asciiDownloadFilename(filename string) string {
	filename = strings.TrimSpace(filename)
	if filename == "" {
		return "book.txt"
	}
	var b strings.Builder
	for _, r := range filename {
		switch {
		case r == '\\' || r == '"':
			b.WriteByte('_')
		case r >= 0x20 && r <= 0x7e:
			b.WriteRune(r)
		}
	}
	if strings.TrimSpace(b.String()) == "" {
		return "book.txt"
	}
	return b.String()
}

func readBookCoverUpload(c *app.RequestContext) (string, []byte, bool) {
	fileHeader, err := c.FormFile("file")
	if err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.books.coverUploadRequired")
		return "", nil, false
	}
	if fileHeader.Size > MaxBookCoverUploadBytes {
		writeErrorKey(c, consts.StatusBadRequest, "api.books.coverTooLarge")
		return "", nil, false
	}

	file, err := fileHeader.Open()
	if err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.books.coverReadFailed", "detail", err.Error())
		return "", nil, false
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, MaxBookCoverUploadBytes+1))
	if err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.books.coverReadFailed", "detail", err.Error())
		return "", nil, false
	}
	if int64(len(data)) > MaxBookCoverUploadBytes {
		writeErrorKey(c, consts.StatusBadRequest, "api.books.coverTooLarge")
		return "", nil, false
	}
	return fileHeader.Filename, data, true
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
	if err := h.app.BookAssets().Reorder(req.Paths); err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, map[string]string{"message": messageKey(c, "api.books.reordered")})
}

// HandleBookSortMode POST /api/books/sort-mode — 切换书架与快捷入口共用的排序方式。
func (h *Handlers) HandleBookSortMode(ctx context.Context, c *app.RequestContext) {
	var req struct {
		Mode projectdomain.SortMode `json:"mode"`
	}
	if err := c.BindJSON(&req); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequest")
		return
	}
	if err := h.app.BookAssets().SetSortMode(req.Mode); err != nil {
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
	meta, err := h.app.BookAssets().Info(path)
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
	meta, err := h.app.BookAssets().UpdateInfo(req.Path, req.Title, req.Author, req.Description)
	if err != nil {
		writeError(c, consts.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, meta)
}
