package handlers

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	"denova/config"
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
	preview.ResidentLoreLimitKB = config.DefaultResidentLoreLimitKB
	preview.MaxResidentLoreLimitKB = config.MaxResidentLoreLimitKB
	preview.RequiredNewBookKB = characterCardRequiredKB(preview.ResidentLoreBytes)
	layered, settingsErr := h.app.Settings()
	if settingsErr == nil {
		if layered.Effective.ResidentLoreLimitKB != nil {
			preview.ResidentLoreLimitKB = *layered.Effective.ResidentLoreLimitKB
		}
		if workspace := h.app.Workspace(); workspace != "" {
			preview.CurrentResidentBytes, _ = book.NewLoreStore(workspace).ResidentContentBytes()
		}
		preview.RequiredCurrentKB = characterCardRequiredKB(preview.CurrentResidentBytes + preview.ResidentLoreBytes)
		preview.RequiredNewBookKB = characterCardRequiredKB(preview.ResidentLoreBytes)
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
	importOptions := book.CharacterCardImportOptions{
		UserCharacterName: strings.TrimSpace(string(c.FormValue("user_character_name"))),
	}
	allowLimitIncrease := strings.EqualFold(strings.TrimSpace(string(c.FormValue("raise_resident_lore_limit"))), "true")
	log.Printf("[api] 导入酒馆角色卡 filename=%q size=%d workspace=%q target_mode=%q", filename, len(data), h.app.Workspace(), targetMode)

	var result book.CharacterCardImportResult
	var err error
	switch targetMode {
	case "current":
		if !h.requireWorkspace(c) {
			return
		}
		var rollbackSettings func()
		importOptions, rollbackSettings, err = h.prepareCharacterCardResidentBudget(data, filename, false, allowLimitIncrease, importOptions)
		if err == nil {
			result, err = h.app.BookService().ImportTavernCharacterCard(filename, data, importOptions)
		}
		if err != nil && rollbackSettings != nil {
			rollbackSettings()
		}
	case "new_book":
		result, err = h.importCharacterCardToNewBook(ctx, filename, data, strings.TrimSpace(string(c.FormValue("book_title"))), allowLimitIncrease, importOptions)
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
		var limitErr *book.ResidentLoreLimitError
		var maxLimitErr *book.ResidentLoreMaxLimitError
		switch {
		case errors.As(err, &limitErr):
			writeErrorKey(c, consts.StatusBadRequest, "api.characterCard.residentLimitRequired", "current", limitErr.CurrentKB, "required", limitErr.RequiredKB)
		case errors.As(err, &maxLimitErr):
			writeErrorKey(c, consts.StatusBadRequest, "api.characterCard.residentLimitTooLarge", "required", maxLimitErr.RequiredKB, "maximum", maxLimitErr.MaximumKB)
		default:
			writeErrorKey(c, status, "api.characterCard.importFailed", "detail", err.Error())
		}
		return
	}
	result.Message = messageKey(c, "api.characterCard.imported", "name", result.Name)
	log.Printf("[api] 导入酒馆角色卡完成 name=%q target=%q entries=%d items=%d", result.Name, result.TargetPath, result.EntryCount, result.ItemCount)
	writeJSON(c, consts.StatusOK, result)
}

func (h *Handlers) importCharacterCardToNewBook(ctx context.Context, filename string, data []byte, title string, allowLimitIncrease bool, options book.CharacterCardImportOptions) (book.CharacterCardImportResult, error) {
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
	options, rollbackSettings, err := h.prepareCharacterCardResidentBudget(data, filename, true, allowLimitIncrease, options)
	if err != nil {
		cleanup()
		return book.CharacterCardImportResult{}, err
	}
	result, err := h.app.BookService().ImportTavernCharacterCard(filename, data, options)
	if err != nil {
		if rollbackSettings != nil {
			rollbackSettings()
		}
		cleanup()
		return book.CharacterCardImportResult{}, err
	}
	result.Workspace = workspace
	result.BookMeta = &meta
	return result, nil
}

func (h *Handlers) prepareCharacterCardResidentBudget(data []byte, filename string, newBook, allowIncrease bool, options book.CharacterCardImportOptions) (book.CharacterCardImportOptions, func(), error) {
	preview, err := book.PreviewTavernCharacterCard(filename, data)
	if err != nil {
		return options, nil, err
	}
	layered, err := h.app.Settings()
	if err != nil {
		return options, nil, err
	}
	limitKB := config.DefaultResidentLoreLimitKB
	if layered.Effective.ResidentLoreLimitKB != nil {
		limitKB = *layered.Effective.ResidentLoreLimitKB
	}
	existingBytes := 0
	if !newBook {
		existingBytes, err = book.NewLoreStore(h.app.Workspace()).ResidentContentBytes()
		if err != nil {
			return options, nil, err
		}
	}
	requiredKB := characterCardRequiredKB(existingBytes + preview.ResidentLoreBytes)
	if requiredKB > config.MaxResidentLoreLimitKB {
		return options, nil, &book.ResidentLoreMaxLimitError{RequiredKB: requiredKB, MaximumKB: config.MaxResidentLoreLimitKB}
	}
	if requiredKB <= limitKB {
		options.ResidentLoreLimitKB = limitKB
		return options, nil, nil
	}
	if !allowIncrease {
		return options, nil, &book.ResidentLoreLimitError{CurrentKB: limitKB, RequiredKB: requiredKB}
	}
	previous := layered.Workspace
	next := layered.Workspace
	next.ResidentLoreLimitKB = &requiredKB
	next.InteractiveRuleLoreLimitKB = nil
	if _, err := h.app.UpdateWorkspaceSettings(next, layered.Revisions.Workspace); err != nil {
		return options, nil, fmt.Errorf("提升常驻资料上限失败: %w", err)
	}
	options.ResidentLoreLimitKB = requiredKB
	rollback := func() {
		if _, rollbackErr := h.app.UpdateWorkspaceSettings(previous); rollbackErr != nil {
			log.Printf("[api] 回滚常驻资料上限失败 workspace=%q err=%v", h.app.Workspace(), rollbackErr)
		}
	}
	return options, rollback, nil
}

func characterCardRequiredKB(bytes int) int {
	if bytes <= 0 {
		return 0
	}
	return (bytes + 1023) / 1024
}
