package handlers

import (
	"context"
	"errors"
	"io"
	"log"
	"os"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	"denova/internal/book"
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
		writeErrorKey(c, consts.StatusBadRequest, "api.characterCard.parseFailed", "detail", err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, preview)
}

func readCharacterCardUpload(c *app.RequestContext) (string, []byte, bool) {
	fileHeader, err := c.FormFile("file")
	if err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.characterCard.uploadRequired")
		return "", nil, false
	}
	if fileHeader.Size > MaxCharacterCardUploadBytes {
		writeErrorKey(c, consts.StatusBadRequest, "api.characterCard.tooLarge")
		return "", nil, false
	}

	file, err := fileHeader.Open()
	if err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.characterCard.readFailed", "detail", err.Error())
		return "", nil, false
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, MaxCharacterCardUploadBytes+1))
	if err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.characterCard.readFailed", "detail", err.Error())
		return "", nil, false
	}
	if int64(len(data)) > MaxCharacterCardUploadBytes {
		writeErrorKey(c, consts.StatusBadRequest, "api.characterCard.tooLarge")
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
	classificationMode := strings.TrimSpace(string(c.FormValue("lore_classification")))
	if classificationMode == "" {
		classificationMode = book.LoreClassificationModeSemantic
	}
	importOptions := book.CharacterCardImportOptions{
		UserCharacterName:  strings.TrimSpace(string(c.FormValue("user_character_name"))),
		ClassificationMode: classificationMode,
		ClassifyLore: func(inputs []book.LoreClassificationInput) ([]book.LoreClassificationSuggestion, error) {
			return h.app.ClassifyLoreItems(ctx, inputs)
		},
	}
	log.Printf("[api] 导入酒馆角色卡 filename=%q size=%d workspace=%q target_mode=%q lore_classification=%q", filename, len(data), h.app.Workspace(), targetMode, classificationMode)

	var result book.CharacterCardImportResult
	var err error
	switch targetMode {
	case "current":
		if !h.requireWorkspace(c) {
			return
		}
		result, err = h.app.BookService().ImportTavernCharacterCard(filename, data, importOptions)
	case "new_book":
		result, err = h.importCharacterCardToNewBook(ctx, filename, data, strings.TrimSpace(string(c.FormValue("book_title"))), importOptions)
	default:
		writeErrorKey(c, consts.StatusBadRequest, "api.characterCard.invalidTarget")
		return
	}
	if err != nil {
		log.Printf("[api] 导入酒馆角色卡失败 filename=%q target_mode=%q error=%v", filename, targetMode, err)
		status := consts.StatusBadRequest
		if strings.Contains(err.Error(), "已存在") {
			status = consts.StatusConflict
		}
		writeErrorKey(c, status, "api.characterCard.importFailed", "detail", err.Error())
		return
	}
	result.Message = messageKey(c, "api.characterCard.imported", "name", result.Name)
	log.Printf("[api] 导入酒馆角色卡完成 name=%q target=%q entries=%d items=%d", result.Name, result.TargetPath, result.EntryCount, result.ItemCount)
	writeJSON(c, consts.StatusOK, result)
}

func (h *Handlers) importCharacterCardToNewBook(ctx context.Context, filename string, data []byte, title string, options book.CharacterCardImportOptions) (book.CharacterCardImportResult, error) {
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
		return book.CharacterCardImportResult{}, errors.New("Denova 数据目录未配置")
	}
	workspace, meta, err := h.app.CreateBook(ctx, layered.Paths.NovaDir, title, "", "")
	if err != nil {
		return book.CharacterCardImportResult{}, err
	}
	cleanup := func() {
		if _, removeErr := h.app.RemoveBook(workspace); removeErr != nil {
			log.Printf("[api] 清理导入失败的新书记录失败 workspace=%q err=%v", workspace, removeErr)
		}
		if removeErr := os.RemoveAll(workspace); removeErr != nil {
			log.Printf("[api] 清理导入失败的新书目录失败 workspace=%q err=%v", workspace, removeErr)
		}
	}
	result, err := h.app.BookService().ImportTavernCharacterCard(filename, data, options)
	if err != nil {
		cleanup()
		return book.CharacterCardImportResult{}, err
	}
	result.Workspace = workspace
	result.BookMeta = &meta
	return result, nil
}
