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
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	"denova/internal/app/resourceexchange"
	appsettings "denova/internal/app/settings"
	"denova/internal/book/character"
	"denova/internal/book/lore"
	"denova/internal/revisionfile"
)

// MaxCharacterCardUploadBytes limits tavern character card uploads.
const MaxCharacterCardUploadBytes int64 = 32 * 1024 * 1024

// Sentinels classifying invalid or unusable override confirmation state, so the
// response carries a specific localized error key instead of a generic failure.
var (
	errCharacterCardReviewPlanRequired = errors.New("confirmed character card import requires a review plan")
	errCharacterCardReviewUnavailable  = errors.New("character card review plan is unavailable or expired")
)

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
	// reviewPlanID is the held plan backing the pending overrides shown to the
	// user; a confirmed import must reuse it and its frozen preview.
	reviewPlanID string
	// overrideReview carries the per-entry decisions the user made about the
	// reviewed duplicates, with the local identity and version they saw.
	overrideReview []characterCardOverrideReview
}

// characterCardOverrideReview is one entry of the override_review form field: a
// reviewed duplicate with the user's decision. It echoes the preview-time local
// binding so the server can reject entries that changed since the preview.
type characterCardOverrideReview struct {
	Name            string `json:"name"`
	ExistingID      string `json:"existing_id"`
	ExistingVersion string `json:"existing_version"`
	Choice          string `json:"choice"`
}

func readCharacterCardImportFields(c *app.RequestContext) (characterCardImportFields, bool) {
	classificationMode := strings.TrimSpace(string(c.FormValue("lore_classification")))
	if classificationMode == "" {
		classificationMode = lore.ClassificationModeSemantic
	}
	overrideReview, err := parseOverrideReview(string(c.FormValue("override_review")))
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
		reviewPlanID:       strings.TrimSpace(string(c.FormValue("review_plan_id"))),
		overrideReview:     overrideReview,
	}, true
}

// parseOverrideReview decodes the optional override_review form field: a JSON
// array of per-entry decisions. Malformed entries are an error so a confirmed
// import never silently falls back to automatic detail resolution.
func parseOverrideReview(raw string) ([]characterCardOverrideReview, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	var review []characterCardOverrideReview
	if err := json.Unmarshal([]byte(raw), &review); err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for _, entry := range review {
		if entry.Name == "" {
			return nil, errors.New("review entry missing name")
		}
		if entry.Choice != "override" && entry.Choice != "keep" {
			return nil, errors.New("invalid review choice for " + entry.Name)
		}
		if seen[entry.Name] {
			return nil, errors.New("duplicate review entry for " + entry.Name)
		}
		seen[entry.Name] = true
	}
	return review, nil
}

// confirmedReview converts the reviewed override decisions into the binding the
// resource exchange expects. It is always present on a confirmed import so
// explicit choices replace, never the detail heuristic.
func (fields characterCardImportFields) confirmedReview() *resourceexchange.ConfirmReview {
	confirm := &resourceexchange.ConfirmReview{Choices: map[string]string{}}
	for _, entry := range fields.overrideReview {
		confirm.Duplicates = append(confirm.Duplicates, resourceexchange.ImportDuplicate{
			Name:            entry.Name,
			ExistingID:      entry.ExistingID,
			ExistingVersion: entry.ExistingVersion,
		})
		confirm.Choices[entry.Name] = entry.Choice
	}
	return confirm
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
	var result character.ImportResult
	var resultErr error
	if fields.confirmOverrides {
		result, resultErr = h.confirmCharacterCardOverrides(ctx, scope.ProjectID, fields)
	} else {
		result, resultErr = h.installCharacterResources(ctx, filename, data, scope.ProjectID, fields)
	}
	result.ProjectID = scope.ProjectID
	result.Workspace = scope.ContentRoot
	h.writeCharacterCardImportResult(ctx, c, filename, result, resultErr)
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
		switch {
		case errors.Is(err, errCharacterCardReviewPlanRequired):
			writeErrorKey(c, consts.StatusBadRequest, "api.characterCard.reviewPlanRequired")
			return
		case errors.Is(err, errCharacterCardReviewUnavailable):
			writeErrorKey(c, consts.StatusBadRequest, "api.characterCard.reviewExpired")
			return
		}
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
	if fields.reviewPlanID != "" {
		// A new Book is created per request and never seeds lore entries, so
		// there is nothing to review against. Rejecting a review binding here
		// also prevents a held plan/preview from being silently orphaned.
		return character.ImportResult{}, errors.New("new book import does not support override confirmation")
	}
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
	candidate := preview.Candidates[0]
	selected := make([]string, 0, len(candidate.Resources))
	for _, resource := range candidate.Resources {
		if resource.Kind == "project.cover" && fields.target != characterCardNewBook {
			preview.Character.CoverPath = ""
			continue
		}
		selected = append(selected, resource.ID)
	}
	plan, err := exchange.Plan(ctx, resourceexchange.PlanRequest{PreviewID: preview.ID, CandidateID: candidate.ID, ProjectID: projectID, Resources: selected, OverrideByDetail: true})
	if err != nil {
		// No plan is held yet; the frozen preview has no pending confirmation
		// referencing it, so release it before returning.
		if discardErr := exchange.DiscardPreview(preview.ID); discardErr != nil {
			slog.WarnContext(ctx, "[internal/api/handlers/handler_character_card.go] failed to discard unusable preview", "preview_id", preview.ID, "error", discardErr)
		}
		return character.ImportResult{}, err
	}
	result := *preview.Character
	result.ProjectID = projectID
	// Same-name lore entries must not be replaced before the user reviews them:
	// hold the plan and its frozen preview for the confirm request instead of
	// applying or discarding them.
	if len(plan.ImportDuplicates) > 0 {
		result.PendingOverrides = convertDuplicatePreviews(plan.ImportDuplicates)
		result.ReviewPlanID = plan.ID
		return result, nil
	}
	installed, err := h.applyCharacterCardPlan(ctx, plan, preview.ID)
	if err != nil {
		// Nothing referenced either record on this path: this preview only
		// serves the current request, and the user restarts the import afresh.
		_ = exchange.DiscardPlan(plan.ID)
		_ = exchange.DiscardPreview(preview.ID)
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

// convertDuplicatePreviews projects resource-exchange duplicates onto the
// character import preview type; field order matches for direct conversion.
func convertDuplicatePreviews(duplicates []resourceexchange.ImportDuplicate) []character.ImportDuplicatePreview {
	previews := make([]character.ImportDuplicatePreview, 0, len(duplicates))
	for _, dup := range duplicates {
		previews = append(previews, character.ImportDuplicatePreview(dup))
	}
	return previews
}

// applyCharacterCardPlan applies an import plan and releases its plan and
// preview records on success. Apply is safe to retry and its revision checks
// surface any concurrent edit as a conflict error for the caller to handle.
func (h *Handlers) applyCharacterCardPlan(ctx context.Context, plan resourceexchange.Plan, previewID string) (resourceexchange.Installation, error) {
	installed, err := h.app.ApplyResourcePlan(ctx, plan.ID)
	if err != nil {
		return resourceexchange.Installation{}, err
	}
	exchange := h.app.ResourceExchange()
	if err := exchange.DiscardPlan(plan.ID); err != nil {
		slog.WarnContext(ctx, "[internal/api/handlers/handler_character_card.go] failed to discard applied plan", "plan_id", plan.ID, "error", err)
	}
	if err := exchange.DiscardPreview(previewID); err != nil {
		slog.WarnContext(ctx, "[internal/api/handlers/handler_character_card.go] failed to discard used preview", "preview_id", previewID, "error", err)
	}
	return installed, nil
}

// confirmCharacterCardOverrides applies the review the user confirmed against
// the held plan returned by the preview request. It reuses the frozen preview
// and validates that every replaced entry still matches the reviewed identity
// and version; anything that changed is reported back for a fresh preview.
func (h *Handlers) confirmCharacterCardOverrides(ctx context.Context, projectID string, fields characterCardImportFields) (character.ImportResult, error) {
	exchange := h.app.ResourceExchange()
	if fields.reviewPlanID == "" {
		return character.ImportResult{}, errCharacterCardReviewPlanRequired
	}
	held, err := exchange.ReadPlan(ctx, fields.reviewPlanID)
	if err != nil {
		return character.ImportResult{}, errCharacterCardReviewUnavailable
	}
	if held.Installation.ProjectID != projectID {
		return character.ImportResult{}, errCharacterCardReviewUnavailable
	}
	if time.Now().After(held.ExpiresAt) {
		return character.ImportResult{}, errCharacterCardReviewUnavailable
	}
	preview, err := exchange.ReadPreview(ctx, held.PreviewID)
	if err != nil {
		return character.ImportResult{}, errCharacterCardReviewUnavailable
	}
	if preview.Character == nil {
		return character.ImportResult{}, errors.New("character card review preview has no character data")
	}
	confirm := fields.confirmedReview()
	resources := make([]string, 0, len(held.Items))
	for _, item := range held.Items {
		resources = append(resources, item.ResourceID)
	}
	plan, err := exchange.Plan(ctx, resourceexchange.PlanRequest{
		PreviewID: held.PreviewID, CandidateID: held.CandidateID, ProjectID: projectID,
		Resources: resources, OverrideByDetail: true, OverrideChoices: confirm.Choices, ConfirmReview: confirm,
	})
	if err != nil {
		return character.ImportResult{}, err
	}
	result := *preview.Character
	result.ProjectID = projectID
	// The target changed since the user reviewed it: never write; hand back a
	// fresh review bound to the current state instead.
	if len(plan.ImportChanged) > 0 || len(plan.ImportUnreviewed) > 0 {
		if err := exchange.DiscardPlan(fields.reviewPlanID); err != nil {
			slog.WarnContext(ctx, "[internal/api/handlers/handler_character_card.go] failed to discard stale review plan", "plan_id", fields.reviewPlanID, "error", err)
		}
		result.PendingOverrides = convertDuplicatePreviews(plan.ImportDuplicates)
		result.ReviewPlanID = plan.ID
		result.ReviewChanged = true
		return result, nil
	}
	installed, err := h.applyCharacterCardPlan(ctx, plan, held.PreviewID)
	if err != nil {
		var conflict *revisionfile.ConflictError
		if !errors.As(err, &conflict) {
			// Not a concurrent edit: the just-built plan has no other reference
			// and can only reappear through a fresh import. Drop it so retries
			// cannot accumulate orphaned plans; the held review stays for retry.
			if discardErr := exchange.DiscardPlan(plan.ID); discardErr != nil {
				slog.WarnContext(ctx, "[internal/api/handlers/handler_character_card.go] failed to discard failed confirm plan", "plan_id", plan.ID, "error", discardErr)
			}
			return character.ImportResult{}, err
		}
		// An Apply conflict means the target changed after this plan was built;
		// restart the review so the user approves the current state. The held
		// review is stale too: both plans are dropped, the frozen preview is
		// reused by the restarted review.
		if discardErr := exchange.DiscardPlan(plan.ID); discardErr != nil {
			slog.WarnContext(ctx, "[internal/api/handlers/handler_character_card.go] failed to discard conflicted plan", "plan_id", plan.ID, "error", discardErr)
		}
		if discardErr := exchange.DiscardPlan(fields.reviewPlanID); discardErr != nil {
			slog.WarnContext(ctx, "[internal/api/handlers/handler_character_card.go] failed to discard conflicted review plan", "plan_id", fields.reviewPlanID, "error", discardErr)
		}
		return h.restartCharacterCardReview(ctx, exchange, held, projectID, confirm)
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
	if err := exchange.DiscardPlan(fields.reviewPlanID); err != nil {
		slog.WarnContext(ctx, "[internal/api/handlers/handler_character_card.go] failed to discard confirmed plan", "plan_id", fields.reviewPlanID, "error", err)
	}
	return result, nil
}

// restartCharacterCardReview replans the frozen preview and returns a fresh
// pending review; used when the target changed after a previous plan was built.
func (h *Handlers) restartCharacterCardReview(ctx context.Context, exchange *resourceexchange.Service, held resourceexchange.Plan, projectID string, confirm *resourceexchange.ConfirmReview) (character.ImportResult, error) {
	resources := make([]string, 0, len(held.Items))
	for _, item := range held.Items {
		resources = append(resources, item.ResourceID)
	}
	retry, err := exchange.Plan(ctx, resourceexchange.PlanRequest{
		PreviewID: held.PreviewID, CandidateID: held.CandidateID, ProjectID: projectID,
		Resources: resources, OverrideByDetail: true, OverrideChoices: confirm.Choices, ConfirmReview: confirm,
	})
	if err != nil {
		// Both plans were already dropped by the conflict path; the frozen
		// preview is now unreferenced, so release it before surfacing the error.
		if discardErr := exchange.DiscardPreview(held.PreviewID); discardErr != nil {
			slog.WarnContext(ctx, "[internal/api/handlers/handler_character_card.go] failed to discard restarted review preview", "preview_id", held.PreviewID, "error", discardErr)
		}
		return character.ImportResult{}, err
	}
	preview, err := exchange.ReadPreview(ctx, held.PreviewID)
	if err != nil {
		// The pending review was already returned below on the success path;
		// dropping the retry plan keeps the preview from being orphaned.
		if discardErr := exchange.DiscardPlan(retry.ID); discardErr != nil {
			slog.WarnContext(ctx, "[internal/api/handlers/handler_character_card.go] failed to discard unreadable review plan", "plan_id", retry.ID, "error", discardErr)
		}
		return character.ImportResult{}, err
	}
	if preview.Character == nil {
		if discardErr := exchange.DiscardPlan(retry.ID); discardErr != nil {
			slog.WarnContext(ctx, "[internal/api/handlers/handler_character_card.go] failed to discard empty review plan", "plan_id", retry.ID, "error", discardErr)
		}
		return character.ImportResult{}, errors.New("character card review preview has no character data")
	}
	result := *preview.Character
	result.ProjectID = projectID
	result.PendingOverrides = convertDuplicatePreviews(retry.ImportDuplicates)
	result.ReviewPlanID = retry.ID
	result.ReviewChanged = true
	return result, nil
}
