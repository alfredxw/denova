package handlers

import (
	"context"
	"errors"
	"os"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	denovaapp "denova/internal/app"
	"denova/internal/book"
	"denova/internal/documentreview"
)

// HandleDocumentReview returns the author's current one-shot review batch.
func (h *Handlers) HandleDocumentReview(ctx context.Context, c *app.RequestContext) {
	if !h.requireWorkspace(c) {
		return
	}
	var thread documentreview.Thread
	workspace, ok := h.withDocumentReviewService(c, func(service *documentreview.Service, _ *book.Service) error {
		var err error
		thread, err = service.CurrentThread(ctx)
		return err
	})
	if !ok {
		return
	}
	writeJSON(c, consts.StatusOK, map[string]any{"workspace": workspace, "review_thread": thread})
}

func (h *Handlers) HandleDocumentCommentCreate(ctx context.Context, c *app.RequestContext) {
	if !h.requireWorkspace(c) {
		return
	}
	var req documentreview.AddCommentRequest
	if err := c.BindJSON(&req); err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	var thread documentreview.Thread
	var comment documentreview.Comment
	workspace, ok := h.withDocumentReviewService(c, func(service *documentreview.Service, files *book.Service) error {
		content, revision, err := files.ReadFileWithRevision(req.Path)
		if err != nil {
			return err
		}
		thread, comment, err = service.AddComment(ctx, req, documentreview.Snapshot{Content: content, Revision: revision})
		return err
	})
	if !ok {
		return
	}
	writeJSON(c, consts.StatusCreated, map[string]any{"workspace": workspace, "review_thread": thread, "comment": comment})
}

func (h *Handlers) HandleDocumentCommentUpdate(ctx context.Context, c *app.RequestContext) {
	if !h.requireWorkspace(c) {
		return
	}
	var req documentreview.UpdateCommentRequest
	if err := c.BindJSON(&req); err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	req.ID = c.Param("id")
	var thread documentreview.Thread
	var comment documentreview.Comment
	workspace, ok := h.withDocumentReviewService(c, func(service *documentreview.Service, _ *book.Service) error {
		var err error
		thread, comment, err = service.UpdateComment(ctx, req)
		return err
	})
	if !ok {
		return
	}
	writeJSON(c, consts.StatusOK, map[string]any{"workspace": workspace, "review_thread": thread, "comment": comment})
}

func (h *Handlers) HandleDocumentCommentDelete(ctx context.Context, c *app.RequestContext) {
	if !h.requireWorkspace(c) {
		return
	}
	var thread documentreview.Thread
	var comment documentreview.Comment
	workspace, ok := h.withDocumentReviewService(c, func(service *documentreview.Service, _ *book.Service) error {
		var err error
		thread, comment, err = service.DeleteComment(ctx, documentreview.DeleteCommentRequest{ID: c.Param("id")})
		return err
	})
	if !ok {
		return
	}
	writeJSON(c, consts.StatusOK, map[string]any{"workspace": workspace, "review_thread": thread, "comment": comment})
}

func (h *Handlers) withDocumentReviewService(c *app.RequestContext, action func(*documentreview.Service, *book.Service) error) (string, bool) {
	expectedWorkspace := workspaceChangeExpectedWorkspace(c)
	workspace, err := h.app.WithDocumentReviewService(expectedWorkspace, action)
	if err != nil {
		h.writeDocumentReviewError(c, expectedWorkspace, err)
		return "", false
	}
	return workspace, true
}

func (h *Handlers) writeDocumentReviewError(c *app.RequestContext, expectedWorkspace string, err error) {
	if errors.Is(err, denovaapp.ErrWorkspaceChanged) || errors.Is(err, denovaapp.ErrNoWorkspace) {
		h.writeWorkspaceChangeLeaseError(c, expectedWorkspace, err)
		return
	}
	status := consts.StatusInternalServerError
	payload := map[string]any{"error": err.Error()}
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
	c.JSON(status, payload)
}
