package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"maps"
	"os"
	"slices"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	"denova/internal/app/resourceexchange"
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

type characterCardImportTarget string

const (
	characterCardCurrentBook characterCardImportTarget = "current"
	characterCardNewBook     characterCardImportTarget = "new_book"
)

type characterCardImportFields struct {
	target             characterCardImportTarget
	bookTitle          string
	userCharacterName  string
	classificationMode string
	// confirmOverrides acknowledges replacing existing same-name lore entries;
	// without it the import returns PendingOverrides instead of applying.
	confirmOverrides bool
	// overrideChoices map incoming lore entry names to "override" or "keep".
	overrideChoices map[string]string
}

func readCharacterCardImportFields(c *app.RequestContext) (characterCardImportFields, bool) {
	classificationMode := strings.TrimSpace(string(c.FormValue("lore_classification")))
	if classificationMode == "" {
		classificationMode = lore.ClassificationModeSemantic
	}
	overrideChoices, err := parseOverrideChoices(string(c.FormValue("override_choices")))
	if err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.characterCard.invalidOverrideChoices", "detail", err.Error())
		return characterCardImportFields{}, false
	}
	return characterCardImportFields{
		target:             characterCardCurrentBook,
		bookTitle:          strings.TrimSpace(string(c.FormValue("book_title"))),
		userCharacterName:  strings.TrimSpace(string(c.FormValue("user_character_name"))),
		classificationMode: classificationMode,
		confirmOverrides:   string(c.FormValue("confirm_overrides")) == "1",
		overrideChoices:    overrideChoices,
	}, true
}

// parseOverrideChoices decodes the optional override_choices form field: a JSON
// object mapping lore entry names to "override" or "keep". An empty field means
// no explicit choices; malformed JSON is an error so a confirmed import never
// silently falls back to automatic resolution.
func parseOverrideChoices(raw string) (map[string]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return map[string]string{}, nil
	}
	choices := map[string]string{}
	if err := json.Unmarshal([]byte(raw), &choices); err != nil {
		return nil, err
	}
	return choices, nil
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
	fields, ok := readCharacterCardImportFields(c)
	if !ok {
		return
	}
	slog.InfoContext(ctx, "[internal/api/handlers/handler_character_card.go] importing character card into Project",
		"project_id", scope.ProjectID,
		"filename", filename,
		"size", len(data),
		"classification_mode", fields.classificationMode,
	)
	result, err := h.installCharacterResources(ctx, filename, data, scope.ProjectID, fields)
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
	fields, ok := readCharacterCardImportFields(c)
	if !ok {
		return
	}
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
	fields.target = characterCardNewBook
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
	created, err := h.app.CreateBook(ctx, layered.Paths.DenovaDir, fields.bookTitle, "", "", "")
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
	result, err := h.installCharacterResources(ctx, filename, data, created.ProjectID, fields)
	if err != nil {
		cleanup()
		return failedResult, err
	}
	// A pending confirmation must not leak an empty Book: no lore entries are
	// written until the confirmed request, so the created Book is removed and
	// recreated by that request. Currently unreachable because a fresh Book has
	// no lore entries to collide with; kept defensively in case CreateBook ever
	// seeds default entries.
	if len(result.PendingOverrides) > 0 {
		cleanup()
		result.ProjectID = ""
		result.Workspace = ""
		result.BookMeta = nil
		return result, nil
	}
	result.ProjectID = created.ProjectID
	result.Workspace = created.Workspace
	result.BookMeta = &created.Meta
	return result, nil
}

// The character-specific picker preserves conversion options, but all writes
// and source records go through the same resource transaction as market imports.
func (h *Handlers) installCharacterResources(ctx context.Context, filename string, data []byte, projectID string, fields characterCardImportFields) (character.ImportResult, error) {
	exchange := h.app.ResourceExchange()
	categories, err := h.app.Lore().Categories(ctx, projectID)
	if err != nil {
		return character.ImportResult{}, err
	}
	preview, err := exchange.PreviewCharacter(ctx, resourceexchange.Source{Kind: "file", Filename: filename}, data, h.characterCardImportOptions(ctx, projectID, fields), categories)
	if err != nil {
		return character.ImportResult{}, err
	}
	defer exchange.DiscardPreview(preview.ID)
	candidate := preview.Candidates[0]
	selected := make([]string, 0, len(candidate.Resources))
	for _, resource := range candidate.Resources {
		if resource.Kind == "project.cover" && fields.target != characterCardNewBook {
			preview.Character.CoverPath = ""
			continue
		}
		selected = append(selected, resource.ID)
	}
	plan, err := exchange.Plan(ctx, resourceexchange.PlanRequest{PreviewID: preview.ID, CandidateID: candidate.ID, ProjectID: projectID, Resources: selected, OverrideByDetail: true, OverrideChoices: fields.overrideChoices})
	if err != nil {
		return character.ImportResult{}, err
	}
	result := *preview.Character
	result.ProjectID = projectID
	// Same-name lore entries must not be replaced before the user reviews them:
	// return the affected list and release the held plan instead of applying.
	if !fields.confirmOverrides && len(plan.ImportDuplicates) > 0 {
		result.PendingOverrides = make([]character.ImportDuplicatePreview, 0, len(plan.ImportDuplicates))
		for _, dup := range plan.ImportDuplicates {
			result.PendingOverrides = append(result.PendingOverrides, character.ImportDuplicatePreview(dup))
		}
		if err := exchange.DiscardPlan(plan.ID); err != nil {
			slog.WarnContext(ctx, "[internal/api/handlers/handler_character_card.go] failed to discard pending plan", "plan_id", plan.ID, "error", err)
		}
		return result, nil
	}
	installed, err := h.app.ApplyResourcePlan(ctx, plan.ID)
	if err != nil {
		return character.ImportResult{}, err
	}
	result.Overridden = plan.ImportOverrides
	result.Skipped = plan.ImportSkipped
	for _, binding := range installed.Bindings {
		if binding.Local.Kind == "lore.collection" {
			for _, sourceID := range slices.Sorted(maps.Keys(binding.Members)) {
				result.ItemIDs = append(result.ItemIDs, binding.Members[sourceID].ID)
			}
		}
	}
	return result, nil
}
