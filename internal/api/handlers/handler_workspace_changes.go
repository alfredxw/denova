package handlers

import (
	"context"
	"errors"
	"net/url"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	denovaapp "denova/internal/app"
	"denova/internal/workspacechange"
)

const workspaceChangeWorkspaceHeader = "X-Denova-Workspace"

// HandleWorkspaceChangeGroups lists durable workspace changes without loading
// manuscript blobs. Full before/after content is loaded only by the detail API.
func (h *Handlers) HandleWorkspaceChangeGroups(ctx context.Context, c *app.RequestContext) {
	if !h.requireWorkspace(c) {
		return
	}
	var groups []workspacechange.ChangeGroupSummary
	workspace, ok := h.withWorkspaceChangeService(c, func(service *workspacechange.Service) error {
		var err error
		groups, err = service.ListGroups(ctx, workspacechange.ChangeFilter{
			Status:         c.Query("status"),
			Path:           c.Query("path"),
			RunID:          c.Query("run_id"),
			SessionID:      c.Query("session_id"),
			ReviewThreadID: c.Query("review_thread_id"),
		})
		return err
	})
	if !ok {
		return
	}
	writeJSON(c, consts.StatusOK, map[string]any{"workspace": workspace, "groups": groups})
}

// HandleWorkspaceChangeReviewThread returns the cumulative, cross-run review
// projection while preserving each group's independent undo boundary.
func (h *Handlers) HandleWorkspaceChangeReviewThread(ctx context.Context, c *app.RequestContext) {
	if !h.requireWorkspace(c) {
		return
	}
	var thread workspacechange.ReviewThread
	workspace, ok := h.withWorkspaceChangeService(c, func(service *workspacechange.Service) error {
		var err error
		thread, err = service.GetReviewThread(ctx, c.Param("id"))
		return err
	})
	if !ok {
		return
	}
	writeJSON(c, consts.StatusOK, map[string]any{"workspace": workspace, "review_thread": thread})
}

// HandleWorkspaceChangeGroup returns one review group with hydrated diff text.
func (h *Handlers) HandleWorkspaceChangeGroup(ctx context.Context, c *app.RequestContext) {
	if !h.requireWorkspace(c) {
		return
	}
	var group workspacechange.ChangeGroup
	workspace, ok := h.withWorkspaceChangeService(c, func(service *workspacechange.Service) error {
		var err error
		group, err = service.GetGroup(ctx, c.Param("id"))
		return err
	})
	if !ok {
		return
	}
	writeJSON(c, consts.StatusOK, map[string]any{"workspace": workspace, "group": group})
}

func (h *Handlers) HandleWorkspaceChangeReview(ctx context.Context, c *app.RequestContext) {
	if !h.requireWorkspace(c) {
		return
	}
	var req workspacechange.ReviewRequest
	if err := c.BindJSON(&req); err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	req.GroupID = c.Param("id")
	rejectDecision := strings.EqualFold(strings.TrimSpace(req.Decision), workspacechange.ReviewDecisionReject)
	var group workspacechange.ChangeGroup
	var affectedPaths []string
	workspace, err := h.app.WithWorkspaceChangeMutation(
		ctx,
		workspaceChangeExpectedWorkspace(c),
		func(service *workspacechange.Service) (denovaapp.WorkspaceChangeMutationHooks, error) {
			result, reviewErr := service.ReviewWithResult(ctx, req)
			if reviewErr != nil {
				return denovaapp.WorkspaceChangeMutationHooks{}, reviewErr
			}
			group = result.Group
			affectedPaths = result.AffectedPaths
			if !rejectDecision || len(affectedPaths) == 0 {
				return denovaapp.WorkspaceChangeMutationHooks{}, nil
			}
			return denovaapp.WorkspaceChangeMutationHooks{
				ScheduleAutoVersion: true,
				AutomationSource:    "workspace_change_review_reject",
				Paths:               affectedPaths,
			}, nil
		},
	)
	if err != nil {
		h.writeWorkspaceChangeLeaseError(c, workspaceChangeExpectedWorkspace(c), err)
		return
	}
	writeJSON(c, consts.StatusOK, workspaceChangeMutationResponse(workspace, group, affectedPaths))
}

func (h *Handlers) HandleWorkspaceChangeUndo(ctx context.Context, c *app.RequestContext) {
	h.handleWorkspaceChangeHistory(ctx, c, false)
}

func (h *Handlers) HandleWorkspaceChangeRedo(ctx context.Context, c *app.RequestContext) {
	h.handleWorkspaceChangeHistory(ctx, c, true)
}

func (h *Handlers) handleWorkspaceChangeHistory(ctx context.Context, c *app.RequestContext, redo bool) {
	if !h.requireWorkspace(c) {
		return
	}
	req := workspacechange.HistoryRequest{GroupID: c.Param("id")}
	var group workspacechange.ChangeGroup
	var affectedPaths []string
	workspace, err := h.app.WithWorkspaceChangeMutation(
		ctx,
		workspaceChangeExpectedWorkspace(c),
		func(service *workspacechange.Service) (denovaapp.WorkspaceChangeMutationHooks, error) {
			var historyErr error
			if redo {
				group, historyErr = service.Redo(ctx, req)
			} else {
				group, historyErr = service.Undo(ctx, req)
			}
			if historyErr != nil {
				return denovaapp.WorkspaceChangeMutationHooks{}, historyErr
			}
			affectedPaths = workspaceChangeGroupPaths(group)
			if len(affectedPaths) == 0 {
				return denovaapp.WorkspaceChangeMutationHooks{}, nil
			}
			source := "workspace_change_undo"
			if redo {
				source = "workspace_change_redo"
			}
			return denovaapp.WorkspaceChangeMutationHooks{
				ScheduleAutoVersion: true,
				AutomationSource:    source,
				Paths:               affectedPaths,
			}, nil
		},
	)
	if err != nil {
		h.writeWorkspaceChangeLeaseError(c, workspaceChangeExpectedWorkspace(c), err)
		return
	}
	writeJSON(c, consts.StatusOK, workspaceChangeMutationResponse(workspace, group, affectedPaths))
}

func (h *Handlers) HandleWorkspaceChangeCommentCreate(ctx context.Context, c *app.RequestContext) {
	if !h.requireWorkspace(c) {
		return
	}
	var req workspacechange.AddCommentRequest
	if err := c.BindJSON(&req); err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	var comment workspacechange.Comment
	workspace, ok := h.withWorkspaceChangeService(c, func(service *workspacechange.Service) error {
		var err error
		comment, err = service.AddComment(ctx, req)
		return err
	})
	if !ok {
		return
	}
	writeJSON(c, consts.StatusCreated, map[string]any{"workspace": workspace, "comment": comment})
}

func (h *Handlers) HandleWorkspaceChangeCommentUpdate(ctx context.Context, c *app.RequestContext) {
	if !h.requireWorkspace(c) {
		return
	}
	var req workspacechange.UpdateCommentRequest
	if err := c.BindJSON(&req); err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	req.ID = c.Param("id")
	var comment workspacechange.Comment
	workspace, ok := h.withWorkspaceChangeService(c, func(service *workspacechange.Service) error {
		var err error
		comment, err = service.UpdateComment(ctx, req)
		return err
	})
	if !ok {
		return
	}
	writeJSON(c, consts.StatusOK, map[string]any{"workspace": workspace, "comment": comment})
}

func (h *Handlers) HandleWorkspaceChangeCommentDelete(ctx context.Context, c *app.RequestContext) {
	if !h.requireWorkspace(c) {
		return
	}
	var comment workspacechange.Comment
	workspace, ok := h.withWorkspaceChangeService(c, func(service *workspacechange.Service) error {
		var err error
		comment, err = service.DeleteComment(ctx, workspacechange.DeleteCommentRequest{ID: c.Param("id")})
		return err
	})
	if !ok {
		return
	}
	writeJSON(c, consts.StatusOK, map[string]any{"workspace": workspace, "comment": comment})
}

func workspaceChangeMutationResponse(workspace string, group workspacechange.ChangeGroup, affectedPaths []string) map[string]any {
	return map[string]any{
		"workspace":      workspace,
		"group":          group,
		"change_group":   group,
		"affected_paths": append([]string{}, affectedPaths...),
	}
}

func workspaceChangeExpectedWorkspace(c *app.RequestContext) string {
	raw := strings.TrimSpace(string(c.Request.Header.Peek(workspaceChangeWorkspaceHeader)))
	if raw == "" {
		return ""
	}
	decoded, err := url.PathUnescape(raw)
	if err != nil {
		return raw
	}
	return strings.TrimSpace(decoded)
}

func (h *Handlers) withWorkspaceChangeService(c *app.RequestContext, action func(*workspacechange.Service) error) (string, bool) {
	expectedWorkspace := workspaceChangeExpectedWorkspace(c)
	canonicalWorkspace := ""
	err := h.app.WithWorkspaceChangeService(expectedWorkspace, func(service *workspacechange.Service) error {
		canonicalWorkspace = service.Workspace()
		return action(service)
	})
	if err != nil {
		h.writeWorkspaceChangeLeaseError(c, expectedWorkspace, err)
		return "", false
	}
	return canonicalWorkspace, true
}

func (h *Handlers) writeWorkspaceChangeLeaseError(c *app.RequestContext, expectedWorkspace string, err error) {
	if errors.Is(err, denovaapp.ErrWorkspaceChanged) {
		writeJSON(c, consts.StatusConflict, map[string]any{
			"error": messageKey(c, "api.workspace.changedDuringRequest"),
			"code":  "workspace_changed",
			"details": map[string]string{
				"expected_workspace": strings.TrimSpace(expectedWorkspace),
				"actual_workspace":   strings.TrimSpace(h.app.Workspace()),
			},
		})
		return
	}
	if errors.Is(err, denovaapp.ErrNoWorkspace) {
		writeErrorKey(c, consts.StatusConflict, "api.workspace.noWorkspace")
		return
	}
	writeWorkspaceChangeError(c, err)
}

func workspaceChangeGroupPaths(group workspacechange.ChangeGroup) []string {
	seen := make(map[string]bool, len(group.ChangeSets))
	paths := make([]string, 0, len(group.ChangeSets))
	for _, change := range group.ChangeSets {
		path := strings.TrimSpace(change.Path)
		if path == "" || seen[path] {
			continue
		}
		seen[path] = true
		paths = append(paths, path)
	}
	return paths
}

func writeWorkspaceChangeError(c *app.RequestContext, err error) {
	status := consts.StatusInternalServerError
	payload := map[string]any{"error": err.Error()}
	var changeErr *workspacechange.Error
	if errors.As(err, &changeErr) {
		payload["code"] = changeErr.Code
		if len(changeErr.Details) > 0 {
			payload["details"] = changeErr.Details
		}
		switch changeErr.Code {
		case workspacechange.ErrorCodeNotFound:
			status = consts.StatusNotFound
		case workspacechange.ErrorCodeRevisionConflict, workspacechange.ErrorCodeConflict, workspacechange.ErrorCodeNoRedo,
			workspacechange.ErrorCodeDurabilityPending:
			status = consts.StatusConflict
		case workspacechange.ErrorCodeInvalidEdit:
			status = consts.StatusBadRequest
		}
	}
	c.JSON(status, payload)
}
