package handlers

import (
	"context"
	"errors"
	"io"
	"log"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	"nova/internal/book"
)

// MaxCharacterCardUploadBytes limits tavern character card uploads.
const MaxCharacterCardUploadBytes int64 = 32 * 1024 * 1024

// handleWorkspacePreviewCharacterCard POST /api/workspace/import-character-card/preview — 预览酒馆角色卡 PNG/JSON，不写入文件。
func (h *Handlers) HandleWorkspacePreviewCharacterCard(ctx context.Context, c *app.RequestContext) {
	filename, data, ok := readCharacterCardUpload(c)
	if !ok {
		return
	}
	preview, err := book.PreviewTavernCharacterCard(filename, data)
	if err != nil {
		writeError(c, consts.StatusBadRequest, "解析酒馆角色卡失败: "+err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, preview)
}

func readCharacterCardUpload(c *app.RequestContext) (string, []byte, bool) {
	fileHeader, err := c.FormFile("file")
	if err != nil {
		writeError(c, consts.StatusBadRequest, "请上传 PNG 或 JSON 格式的酒馆角色卡文件")
		return "", nil, false
	}
	if fileHeader.Size > MaxCharacterCardUploadBytes {
		writeError(c, consts.StatusBadRequest, "角色卡文件不能超过 32MB")
		return "", nil, false
	}

	file, err := fileHeader.Open()
	if err != nil {
		writeError(c, consts.StatusBadRequest, "读取上传文件失败: "+err.Error())
		return "", nil, false
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, MaxCharacterCardUploadBytes+1))
	if err != nil {
		writeError(c, consts.StatusBadRequest, "读取上传文件失败: "+err.Error())
		return "", nil, false
	}
	if int64(len(data)) > MaxCharacterCardUploadBytes {
		writeError(c, consts.StatusBadRequest, "角色卡文件不能超过 32MB")
		return "", nil, false
	}
	return fileHeader.Filename, data, true
}

// handleWorkspaceImportCharacterCard POST /api/workspace/import-character-card — 导入酒馆角色卡 PNG/JSON 到互动资料库。
func (h *Handlers) HandleWorkspaceImportCharacterCard(ctx context.Context, c *app.RequestContext) {
	filename, data, ok := readCharacterCardUpload(c)
	if !ok {
		return
	}

	targetMode := strings.TrimSpace(string(c.FormValue("target_mode")))
	if targetMode == "" {
		targetMode = "current"
	}
	log.Printf("[api] 导入酒馆角色卡 filename=%q size=%d workspace=%q target_mode=%q", filename, len(data), h.app.Workspace(), targetMode)

	var result book.CharacterCardImportResult
	var err error
	switch targetMode {
	case "current":
		if !h.requireWorkspace(c) {
			return
		}
		result, err = h.app.BookService().ImportTavernCharacterCard(filename, data)
	case "new_book":
		result, err = h.importCharacterCardToNewBook(ctx, filename, data, strings.TrimSpace(string(c.FormValue("book_title"))))
	default:
		writeError(c, consts.StatusBadRequest, "导入目标无效")
		return
	}
	if err != nil {
		log.Printf("[api] 导入酒馆角色卡失败 filename=%q target_mode=%q error=%v", filename, targetMode, err)
		status := consts.StatusBadRequest
		if strings.Contains(err.Error(), "已存在") {
			status = consts.StatusConflict
		}
		writeError(c, status, "导入酒馆角色卡失败: "+err.Error())
		return
	}
	log.Printf("[api] 导入酒馆角色卡完成 name=%q target=%q entries=%d items=%d", result.Name, result.TargetPath, result.EntryCount, result.ItemCount)
	writeJSON(c, consts.StatusOK, result)
}

func (h *Handlers) importCharacterCardToNewBook(ctx context.Context, filename string, data []byte, title string) (book.CharacterCardImportResult, error) {
	preview, err := book.PreviewTavernCharacterCard(filename, data)
	if err != nil {
		return book.CharacterCardImportResult{}, err
	}
	if title == "" {
		title = preview.Name
	}
	layered, err := h.app.Settings()
	if err != nil {
		return book.CharacterCardImportResult{}, err
	}
	if layered.Paths.NovaDir == "" {
		return book.CharacterCardImportResult{}, errors.New("Nova 数据目录未配置")
	}
	workspace, meta, err := h.app.CreateBook(ctx, layered.Paths.NovaDir, title, "", "")
	if err != nil {
		return book.CharacterCardImportResult{}, err
	}
	result, err := h.app.BookService().ImportTavernCharacterCard(filename, data)
	if err != nil {
		return book.CharacterCardImportResult{}, err
	}
	result.Workspace = workspace
	result.BookMeta = &meta
	return result, nil
}
