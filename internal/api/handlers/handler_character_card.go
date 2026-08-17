package handlers

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	appsettings "denova/internal/app/settings"
	"denova/internal/book/character"
	"denova/internal/book/lore"
)

// MaxCharacterCardUploadBytes limits tavern character card uploads.
const MaxCharacterCardUploadBytes int64 = 32 * 1024 * 1024

// HandleCharacterCardPreview parses an upload without selecting or mutating a Project.
func (h *Handlers) HandleCharacterCardPreview(ctx context.Context, c *app.RequestContext) {
	filename, data, ok := readCharacterCardUpload(c)
	if !ok {
		return
	}
	preview, err := character.PreviewTavernCard(filename, data)
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

type characterCardImportFields struct {
	bookTitle          string
	userCharacterName  string
	classificationMode string
}

func readCharacterCardImportFields(c *app.RequestContext) characterCardImportFields {
	classificationMode := strings.TrimSpace(string(c.FormValue("lore_classification")))
	if classificationMode == "" {
		classificationMode = lore.ClassificationModeSemantic
	}
	return characterCardImportFields{
		bookTitle:          strings.TrimSpace(string(c.FormValue("book_title"))),
		userCharacterName:  strings.TrimSpace(string(c.FormValue("user_character_name"))),
		classificationMode: classificationMode,
	}
}

func (h *Handlers) characterCardImportOptions(ctx context.Context, projectID string, fields characterCardImportFields) character.ImportOptions {
	return character.ImportOptions{
		UserCharacterName:  fields.userCharacterName,
		ClassificationMode: fields.classificationMode,
		ClassifyLore: func(inputs []lore.ClassificationInput) ([]lore.ClassificationSuggestion, error) {
			return h.app.Lore().ClassifyItems(ctx, projectID, inputs)
		},
	}
}

// HandleProjectCharacterCardImport imports directly into the Book Project
// resolved by middleware; foreground navigation is never consulted.
func (h *Handlers) HandleProjectCharacterCardImport(ctx context.Context, c *app.RequestContext) {
	scope, ok := requireProjectScope(c)
	if !ok {
		return
	}
	filename, data, ok := readCharacterCardUpload(c)
	if !ok {
		return
	}
	fields := readCharacterCardImportFields(c)
	slog.InfoContext(ctx, "[internal/api/handlers/handler_character_card.go] importing character card into Project",
		"project_id", scope.ProjectID,
		"filename", filename,
		"size", len(data),
		"classification_mode", fields.classificationMode,
	)
	result, err := character.NewService(scope.ContentRoot).ImportTavernCard(
		filename, data, h.characterCardImportOptions(ctx, scope.ProjectID, fields),
	)
	result.ProjectID = scope.ProjectID
	result.Workspace = scope.ContentRoot
	h.writeCharacterCardImportResult(ctx, c, filename, result, err)
}

// HandleNewBookCharacterCardImport creates a Book and returns its stable
// Project identity as part of the import result.
func (h *Handlers) HandleNewBookCharacterCardImport(ctx context.Context, c *app.RequestContext) {
	filename, data, ok := readCharacterCardUpload(c)
	if !ok {
		return
	}
	fields := readCharacterCardImportFields(c)
	result, err := h.importCharacterCardToNewBook(ctx, filename, data, fields)
	h.writeCharacterCardImportResult(ctx, c, filename, result, err)
}

func (h *Handlers) writeCharacterCardImportResult(ctx context.Context, c *app.RequestContext, filename string, result character.ImportResult, err error) {
	if err != nil {
		slog.ErrorContext(ctx, "[internal/api/handlers/handler_character_card.go] character card import failed",
			"filename", filename,
			"project_id", result.ProjectID,
			"error", err,
		)
		status := consts.StatusBadRequest
		if strings.Contains(err.Error(), "已存在") {
			status = consts.StatusConflict
		}
		writeErrorKey(c, status, "api.characterCard.importFailed", "detail", err.Error())
		return
	}
	result.Message = messageKey(c, "api.characterCard.imported", "name", result.Name)
	slog.InfoContext(ctx, "[internal/api/handlers/handler_character_card.go] character card import completed",
		"project_id", result.ProjectID,
		"name", result.Name,
		"target", result.TargetPath,
		"entries", result.EntryCount,
		"items", result.ItemCount,
	)
	writeJSON(c, consts.StatusOK, result)
}

func (h *Handlers) importCharacterCardToNewBook(ctx context.Context, filename string, data []byte, fields characterCardImportFields) (character.ImportResult, error) {
	preview, err := character.PreviewTavernCard(filename, data)
	if err != nil {
		return character.ImportResult{}, err
	}
	if fields.bookTitle == "" {
		fields.bookTitle = preview.Name
	}
	layered, err := h.app.SettingsService().Snapshot(appsettings.Global())
	if err != nil {
		return character.ImportResult{}, err
	}
	if layered.Paths.DenovaDir == "" {
		return character.ImportResult{}, errors.New("Denova data directory is not configured")
	}
	created, err := h.app.CreateBook(ctx, layered.Paths.DenovaDir, fields.bookTitle, "", "")
	if err != nil {
		return character.ImportResult{}, err
	}
	failedResult := character.ImportResult{ProjectID: created.ProjectID, Workspace: created.Workspace}
	cleanup := func() {
		if _, removeErr := h.app.RemoveBook(created.Workspace); removeErr != nil {
			slog.ErrorContext(ctx, "[internal/api/handlers/handler_character_card.go] failed to archive a new Book after character card import failed",
				"project_id", created.ProjectID,
				"workspace", created.Workspace,
				"error", removeErr,
			)
		}
		if removeErr := os.RemoveAll(created.Workspace); removeErr != nil {
			slog.ErrorContext(ctx, "[internal/api/handlers/handler_character_card.go] failed to remove a new Book directory after character card import failed",
				"project_id", created.ProjectID,
				"workspace", created.Workspace,
				"error", removeErr,
			)
		}
	}
	result, err := character.NewService(created.Workspace).ImportTavernCard(
		filename, data, h.characterCardImportOptions(ctx, created.ProjectID, fields),
	)
	if err != nil {
		cleanup()
		return failedResult, err
	}
	result.ProjectID = created.ProjectID
	result.Workspace = created.Workspace
	result.BookMeta = &created.Meta
	return result, nil
}
