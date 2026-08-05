package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	projectbookapp "denova/internal/app/projectbook"
	booklore "denova/internal/book/lore"
	"denova/internal/workspace/documentreview"
)

func (h *Handlers) HandleProjectBookSnapshot(ctx context.Context, c *app.RequestContext) {
	scope, ok := requireProjectScope(c)
	if !ok {
		return
	}
	snapshot, err := h.app.ProjectBook().Snapshot(ctx, scope.ProjectID)
	if err != nil {
		writeProjectBookError(c, err, "api.projectBook.readFailed")
		return
	}
	writeJSON(c, consts.StatusOK, snapshot)
}

func (h *Handlers) HandleProjectBookTree(ctx context.Context, c *app.RequestContext) {
	scope, ok := requireProjectScope(c)
	if !ok {
		return
	}
	workspace, tree, err := h.app.ProjectBook().Tree(ctx, scope.ProjectID)
	if err != nil {
		writeProjectBookError(c, err, "api.projectBook.readFailed")
		return
	}
	writeJSON(c, consts.StatusOK, map[string]any{"project_id": scope.ProjectID, "workspace": workspace, "tree": tree})
}

func (h *Handlers) HandleProjectBookSummary(ctx context.Context, c *app.RequestContext) {
	scope, ok := requireProjectScope(c)
	if !ok {
		return
	}
	workspace, summary, err := h.app.ProjectBook().Summary(ctx, scope.ProjectID)
	if err != nil {
		writeProjectBookError(c, err, "api.workspace.summaryFailed")
		return
	}
	writeJSON(c, consts.StatusOK, map[string]any{"project_id": scope.ProjectID, "workspace": workspace, "summary": summary})
}

func (h *Handlers) HandleProjectBookChapterStatus(_ context.Context, c *app.RequestContext) {
	scope, ok := requireProjectScope(c)
	if !ok {
		return
	}
	var request struct {
		Path      string `json:"path"`
		Confirmed bool   `json:"confirmed"`
	}
	if err := c.BindJSON(&request); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
		return
	}
	if strings.TrimSpace(request.Path) == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.workspace.chapterStatusPathRequired")
		return
	}
	if err := h.app.ProjectBook().SetChapterConfirmed(scope.ProjectID, request.Path, request.Confirmed); err != nil {
		writeProjectBookError(c, err, "api.workspace.chapterStatusFailed")
		return
	}
	writeJSON(c, consts.StatusOK, map[string]any{
		"project_id": scope.ProjectID,
		"path":       request.Path,
		"confirmed":  request.Confirmed,
		"message":    messageKey(c, "api.workspace.chapterStatusSaved"),
	})
}

func (h *Handlers) HandleProjectLoreItems(_ context.Context, c *app.RequestContext) {
	scope, ok := requireProjectScope(c)
	if !ok {
		return
	}
	items, err := h.app.ProjectBook().LoreItems(scope.ProjectID)
	if err != nil {
		writeProjectBookError(c, err, "api.projectBook.loreFailed")
		return
	}
	writeJSON(c, consts.StatusOK, map[string]any{"project_id": scope.ProjectID, "items": items})
}

func (h *Handlers) HandleProjectLoreItemCreate(_ context.Context, c *app.RequestContext) {
	scope, ok := requireProjectScope(c)
	if !ok {
		return
	}
	var input booklore.ItemInput
	if err := c.BindJSON(&input); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
		return
	}
	item, err := h.app.ProjectBook().CreateLoreItem(scope.ProjectID, input)
	if err != nil {
		writeProjectBookError(c, err, "api.projectBook.loreFailed")
		return
	}
	writeJSON(c, consts.StatusOK, map[string]any{"project_id": scope.ProjectID, "item": item})
}

func (h *Handlers) HandleProjectLoreItemUpdate(_ context.Context, c *app.RequestContext) {
	scope, ok := requireProjectScope(c)
	if !ok {
		return
	}
	input, ok := bindCompleteLoreItemInput(c, c.Param("id"))
	if !ok {
		return
	}
	item, err := h.app.ProjectBook().UpdateLoreItem(scope.ProjectID, c.Param("id"), input)
	if err != nil {
		if errors.Is(err, booklore.ErrRevisionConflict) {
			writeErrorKey(c, consts.StatusConflict, "api.resource.revisionConflict")
			return
		}
		writeProjectBookError(c, err, "api.projectBook.loreFailed")
		return
	}
	writeJSON(c, consts.StatusOK, map[string]any{"project_id": scope.ProjectID, "item": item})
}

func (h *Handlers) HandleProjectLoreItemDelete(_ context.Context, c *app.RequestContext) {
	scope, ok := requireProjectScope(c)
	if !ok {
		return
	}
	if err := h.app.ProjectBook().DeleteLoreItem(scope.ProjectID, c.Param("id")); err != nil {
		writeProjectBookError(c, err, "api.projectBook.loreFailed")
		return
	}
	writeJSON(c, consts.StatusOK, map[string]string{"project_id": scope.ProjectID, "status": "ok"})
}

func (h *Handlers) HandleProjectDocumentReview(ctx context.Context, c *app.RequestContext) {
	scope, ok := requireProjectScope(c)
	if !ok {
		return
	}
	var thread documentreview.Thread
	workspace, err := h.app.ProjectBook().WithDocumentReview(scope.ProjectID, func(service *documentreview.Service, _ documentreview.SnapshotResolver) error {
		var readErr error
		thread, readErr = service.CurrentThread(ctx)
		return readErr
	})
	if err != nil {
		h.writeProjectDocumentReviewError(c, err)
		return
	}
	writeJSON(c, consts.StatusOK, map[string]any{"project_id": scope.ProjectID, "workspace": workspace, "review_thread": thread})
}

func (h *Handlers) HandleProjectDocumentCommentCreate(ctx context.Context, c *app.RequestContext) {
	scope, ok := requireProjectScope(c)
	if !ok {
		return
	}
	var request documentreview.AddCommentRequest
	if err := c.BindJSON(&request); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
		return
	}
	var thread documentreview.Thread
	var comment documentreview.Comment
	workspace, err := h.app.ProjectBook().WithDocumentReview(scope.ProjectID, func(service *documentreview.Service, resolver documentreview.SnapshotResolver) error {
		resolved, resolveErr := resolver.ResolveReviewTarget(ctx, request.Target)
		if resolveErr != nil {
			return resolveErr
		}
		request.Target = resolved.Target
		thread, comment, resolveErr = service.AddComment(ctx, request, resolved.Snapshot)
		return resolveErr
	})
	if err != nil {
		h.writeProjectDocumentReviewError(c, err)
		return
	}
	writeJSON(c, consts.StatusCreated, map[string]any{
		"project_id": scope.ProjectID, "workspace": workspace, "review_thread": thread, "comment": comment,
	})
}

func (h *Handlers) HandleProjectDocumentCommentUpdate(ctx context.Context, c *app.RequestContext) {
	scope, ok := requireProjectScope(c)
	if !ok {
		return
	}
	var request documentreview.UpdateCommentRequest
	if err := c.BindJSON(&request); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
		return
	}
	request.ID = c.Param("id")
	var thread documentreview.Thread
	var comment documentreview.Comment
	workspace, err := h.app.ProjectBook().WithDocumentReview(scope.ProjectID, func(service *documentreview.Service, _ documentreview.SnapshotResolver) error {
		var updateErr error
		thread, comment, updateErr = service.UpdateComment(ctx, request)
		return updateErr
	})
	if err != nil {
		h.writeProjectDocumentReviewError(c, err)
		return
	}
	writeJSON(c, consts.StatusOK, map[string]any{
		"project_id": scope.ProjectID, "workspace": workspace, "review_thread": thread, "comment": comment,
	})
}

func (h *Handlers) HandleProjectDocumentCommentDelete(ctx context.Context, c *app.RequestContext) {
	scope, ok := requireProjectScope(c)
	if !ok {
		return
	}
	var thread documentreview.Thread
	var comment documentreview.Comment
	workspace, err := h.app.ProjectBook().WithDocumentReview(scope.ProjectID, func(service *documentreview.Service, _ documentreview.SnapshotResolver) error {
		var deleteErr error
		thread, comment, deleteErr = service.DeleteComment(ctx, documentreview.DeleteCommentRequest{ID: c.Param("id")})
		return deleteErr
	})
	if err != nil {
		h.writeProjectDocumentReviewError(c, err)
		return
	}
	writeJSON(c, consts.StatusOK, map[string]any{
		"project_id": scope.ProjectID, "workspace": workspace, "review_thread": thread, "comment": comment,
	})
}

func bindCompleteLoreItemInput(c *app.RequestContext, resourceID string) (booklore.ItemInput, bool) {
	var input booklore.ItemInput
	if err := c.BindJSON(&input); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
		return booklore.ItemInput{}, false
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(c.Request.Body(), &fields); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
		return booklore.ItemInput{}, false
	}
	required := [...]string{"enabled", "type", "name", "importance", "tags", "brief_description", "keywords", "load_mode", "content"}
	missing := make([]string, 0, len(required))
	for _, field := range required {
		if _, exists := fields[field]; !exists {
			missing = append(missing, field)
		}
	}
	if len(missing) > 0 {
		writeErrorKey(c, consts.StatusBadRequest, "api.projectBook.loreFieldsRequired", "fields", strings.Join(missing, ", "))
		return booklore.ItemInput{}, false
	}
	resourceID = strings.TrimSpace(resourceID)
	if bodyID := strings.TrimSpace(input.ID); bodyID != "" && bodyID != resourceID {
		writeErrorKey(c, consts.StatusBadRequest, "api.projectBook.loreIDMismatch")
		return booklore.ItemInput{}, false
	}
	return input, true
}

func writeProjectBookError(c *app.RequestContext, err error, fallbackKey string) {
	status := consts.StatusBadRequest
	if errors.Is(err, os.ErrNotExist) {
		status = consts.StatusNotFound
	}
	if errors.Is(err, projectbookapp.ErrBookProjectRequired) {
		writeErrorKey(c, status, "api.projectBook.bookRequired")
		return
	}
	writeErrorKey(c, status, fallbackKey, "detail", err.Error())
}

func (h *Handlers) writeProjectDocumentReviewError(c *app.RequestContext, err error) {
	if errors.Is(err, projectbookapp.ErrBookProjectRequired) {
		writeProjectBookError(c, err, "api.projectBook.reviewFailed")
		return
	}
	status := consts.StatusInternalServerError
	payload := map[string]any{
		"error": messageKey(c, "api.projectBook.reviewFailed", "detail", err.Error()),
		"code":  "project_document_review_error",
	}
	var reviewErr *documentreview.Error
	if errors.As(err, &reviewErr) {
		payload["code"] = reviewErr.Code
		if len(reviewErr.Details) > 0 {
			payload["details"] = reviewErr.Details
		}
		switch reviewErr.Code {
		case documentreview.ErrorCodeNotFound:
			status = consts.StatusNotFound
		case documentreview.ErrorCodeConflict:
			status = consts.StatusConflict
		case documentreview.ErrorCodeInvalid:
			status = consts.StatusBadRequest
		}
	} else if errors.Is(err, os.ErrNotExist) {
		status = consts.StatusNotFound
	}
	writeJSON(c, status, payload)
}
