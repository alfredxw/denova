package handlers

import (
	"context"
	"io"
	"log"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	"nova/internal/book"
)

// MaxNovelImportUploadBytes limits txt/md novel imports.
const MaxNovelImportUploadBytes int64 = 64 * 1024 * 1024

// HandlePreviewNovelImport POST /api/books/import-novel/preview — 预览 txt/md 小说章节，不写入 workspace。
func (h *Handlers) HandlePreviewNovelImport(ctx context.Context, c *app.RequestContext) {
	_ = ctx
	filename, data, ok := readNovelImportUpload(c)
	if !ok {
		return
	}
	preview, err := book.PreviewNovelImport(filename, data)
	if err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.novelImport.parseFailed", "detail", err.Error())
		return
	}
	localizeNovelImportWarnings(c, &preview)
	writeJSON(c, consts.StatusOK, preview)
}

// HandleNovelImport POST /api/books/import-novel — 导入 txt/md 小说为新书并写入章节。
func (h *Handlers) HandleNovelImport(ctx context.Context, c *app.RequestContext) {
	filename, data, ok := readNovelImportUpload(c)
	if !ok {
		return
	}
	preview, err := book.PreviewNovelImport(filename, data)
	if err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.novelImport.parseFailed", "detail", err.Error())
		return
	}

	title := strings.TrimSpace(string(c.FormValue("book_title")))
	if title == "" {
		title = preview.Title
	}
	author := strings.TrimSpace(string(c.FormValue("author")))
	description := strings.TrimSpace(string(c.FormValue("description")))

	layered, err := h.app.Settings()
	if err != nil {
		writeError(c, consts.StatusInternalServerError, err.Error())
		return
	}
	if layered.Paths.NovaDir == "" {
		writeErrorKey(c, consts.StatusInternalServerError, "api.books.novaDirMissing")
		return
	}

	log.Printf("[api] 导入小说 filename=%q size=%d title=%q", filename, len(data), title)
	workspace, meta, err := h.app.CreateBook(ctx, layered.Paths.NovaDir, title, author, description)
	if err != nil {
		status := consts.StatusInternalServerError
		if strings.Contains(err.Error(), "已存在") {
			status = consts.StatusConflict
		}
		writeErrorKey(c, status, "api.novelImport.importFailed", "detail", err.Error())
		return
	}

	importPreview, paths, err := book.ImportNovelToWorkspace(workspace, filename, data)
	if err != nil {
		writeErrorKey(c, consts.StatusInternalServerError, "api.novelImport.importFailed", "detail", err.Error())
		return
	}

	log.Printf("[api] 导入小说完成 workspace=%q chapters=%d", workspace, importPreview.ChapterCount)
	writeJSON(c, consts.StatusOK, book.NovelImportResult{
		Workspace:    workspace,
		BookMeta:     &meta,
		Title:        importPreview.Title,
		ChapterCount: importPreview.ChapterCount,
		TotalChars:   importPreview.TotalChars,
		ChapterPaths: paths,
		Message:      messageKey(c, "api.novelImport.imported"),
	})
}

func readNovelImportUpload(c *app.RequestContext) (string, []byte, bool) {
	fileHeader, err := c.FormFile("file")
	if err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.novelImport.uploadRequired")
		return "", nil, false
	}
	if fileHeader.Size > MaxNovelImportUploadBytes {
		writeErrorKey(c, consts.StatusBadRequest, "api.novelImport.tooLarge")
		return "", nil, false
	}

	file, err := fileHeader.Open()
	if err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.novelImport.readFailed", "detail", err.Error())
		return "", nil, false
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, MaxNovelImportUploadBytes+1))
	if err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.novelImport.readFailed", "detail", err.Error())
		return "", nil, false
	}
	if int64(len(data)) > MaxNovelImportUploadBytes {
		writeErrorKey(c, consts.StatusBadRequest, "api.novelImport.tooLarge")
		return "", nil, false
	}
	return fileHeader.Filename, data, true
}

func localizeNovelImportWarnings(c *app.RequestContext, preview *book.NovelImportPreview) {
	for i, warning := range preview.Warnings {
		switch warning {
		case book.NovelImportSingleChapterWarning:
			preview.Warnings[i] = messageKey(c, "api.novelImport.singleChapterWarning")
		}
	}
}
