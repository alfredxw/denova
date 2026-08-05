package handlers

import (
	"context"
	"errors"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	denovaapp "denova/internal/app"
	projectdomain "denova/internal/project"
	workspacechange "denova/internal/workspace/change"
)

// HandleWorkspaceChangeGroups lists durable workspace changes without loading
// manuscript blobs. Full before/after content is loaded only by the detail API.
func (h *Handlers) HandleWorkspaceChangeGroups(ctx context.Context, c *app.RequestContext) {
	scope, ok := requireProjectScope(c)
	if !ok {
		return
	}
	var groups []workspacechange.ChangeGroupSummary
	layout, ok := h.withProjectChangeService(ctx, c, scope.ProjectID, func(service *workspacechange.Service) error {
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
	writeJSON(c, consts.StatusOK, map[string]any{"project_id": layout.ProjectID, "workspace": layout.ContentRoot, "groups": groups})
}

// HandleWorkspaceChangeReviewThread returns the cumulative, cross-run review
// projection while preserving each group's independent undo boundary.
func (h *Handlers) HandleWorkspaceChangeReviewThread(ctx context.Context, c *app.RequestContext) {
	scope, ok := requireProjectScope(c)
	if !ok {
		return
	}
	var thread workspacechange.ReviewThread
	layout, ok := h.withProjectChangeService(ctx, c, scope.ProjectID, func(service *workspacechange.Service) error {
		var err error
		thread, err = service.GetReviewThread(ctx, c.Param("id"))
		return err
	})
	if !ok {
		return
	}
	writeJSON(c, consts.StatusOK, map[string]any{"project_id": layout.ProjectID, "workspace": layout.ContentRoot, "review_thread": thread})
}

// HandleWorkspaceChangeGroup returns one review group with hydrated diff text.
func (h *Handlers) HandleWorkspaceChangeGroup(ctx context.Context, c *app.RequestContext) {
	scope, ok := requireProjectScope(c)
	if !ok {
		return
	}
	var group workspacechange.ChangeGroup
	layout, ok := h.withProjectChangeService(ctx, c, scope.ProjectID, func(service *workspacechange.Service) error {
		var err error
		group, err = service.GetGroup(ctx, c.Param("id"))
		return err
	})
	if !ok {
		return
	}
	writeJSON(c, consts.StatusOK, map[string]any{"project_id": layout.ProjectID, "workspace": layout.ContentRoot, "group": group})
}

func (h *Handlers) HandleWorkspaceChangeReview(ctx context.Context, c *app.RequestContext) {
	scope, ok := requireProjectScope(c)
	if !ok {
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
	layout, err := h.app.WithProjectChangeMutation(
		ctx,
		scope.ProjectID,
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
		h.writeProjectChangeError(c, scope.ProjectID, err)
		return
	}
	writeJSON(c, consts.StatusOK, workspaceChangeMutationResponse(layout.ProjectID, layout.ContentRoot, group, affectedPaths))
}

func (h *Handlers) HandleWorkspaceChangeUndo(ctx context.Context, c *app.RequestContext) {
	h.handleWorkspaceChangeHistory(ctx, c, false)
}

func (h *Handlers) HandleWorkspaceChangeRedo(ctx context.Context, c *app.RequestContext) {
	h.handleWorkspaceChangeHistory(ctx, c, true)
}

func (h *Handlers) handleWorkspaceChangeHistory(ctx context.Context, c *app.RequestContext, redo bool) {
	scope, ok := requireProjectScope(c)
	if !ok {
		return
	}
	req := workspacechange.HistoryRequest{GroupID: c.Param("id")}
	var group workspacechange.ChangeGroup
	var affectedPaths []string
	layout, err := h.app.WithProjectChangeMutation(
		ctx,
		scope.ProjectID,
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
		h.writeProjectChangeError(c, scope.ProjectID, err)
		return
	}
	writeJSON(c, consts.StatusOK, workspaceChangeMutationResponse(layout.ProjectID, layout.ContentRoot, group, affectedPaths))
}

func (h *Handlers) HandleWorkspaceChangeCommentCreate(ctx context.Context, c *app.RequestContext) {
	scope, ok := requireProjectScope(c)
	if !ok {
		return
	}
	var req workspacechange.AddCommentRequest
	if err := c.BindJSON(&req); err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	var comment workspacechange.Comment
	layout, ok := h.withProjectChangeService(ctx, c, scope.ProjectID, func(service *workspacechange.Service) error {
		var err error
		comment, err = service.AddComment(ctx, req)
		return err
	})
	if !ok {
		return
	}
	writeJSON(c, consts.StatusCreated, map[string]any{"project_id": layout.ProjectID, "workspace": layout.ContentRoot, "comment": comment})
}

func (h *Handlers) HandleWorkspaceChangeCommentUpdate(ctx context.Context, c *app.RequestContext) {
	scope, ok := requireProjectScope(c)
	if !ok {
		return
	}
	var req workspacechange.UpdateCommentRequest
	if err := c.BindJSON(&req); err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	req.ID = c.Param("id")
	var comment workspacechange.Comment
	layout, ok := h.withProjectChangeService(ctx, c, scope.ProjectID, func(service *workspacechange.Service) error {
		var err error
		comment, err = service.UpdateComment(ctx, req)
		return err
	})
	if !ok {
		return
	}
	writeJSON(c, consts.StatusOK, map[string]any{"project_id": layout.ProjectID, "workspace": layout.ContentRoot, "comment": comment})
}

func (h *Handlers) HandleWorkspaceChangeCommentDelete(ctx context.Context, c *app.RequestContext) {
	scope, ok := requireProjectScope(c)
	if !ok {
		return
	}
	var comment workspacechange.Comment
	layout, ok := h.withProjectChangeService(ctx, c, scope.ProjectID, func(service *workspacechange.Service) error {
		var err error
		comment, err = service.DeleteComment(ctx, workspacechange.DeleteCommentRequest{ID: c.Param("id")})
		return err
	})
	if !ok {
		return
	}
	writeJSON(c, consts.StatusOK, map[string]any{"project_id": layout.ProjectID, "workspace": layout.ContentRoot, "comment": comment})
}

func workspaceChangeMutationResponse(projectID, workspace string, group workspacechange.ChangeGroup, affectedPaths []string) map[string]any {
	return map[string]any{
		"project_id":     projectID,
		"workspace":      workspace,
		"group":          group,
		"change_group":   group,
		"affected_paths": append([]string{}, affectedPaths...),
	}
}

func (h *Handlers) withProjectChangeService(
	ctx context.Context,
	c *app.RequestContext,
	projectID string,
	action func(*workspacechange.Service) error,
) (projectdomain.Layout, bool) {
	layout, err := h.app.WithProjectChangeService(ctx, projectID, action)
	if err != nil {
		h.writeProjectChangeError(c, projectID, err)
		return projectdomain.Layout{}, false
	}
	return layout, true
}

func (h *Handlers) writeProjectChangeError(c *app.RequestContext, projectID string, err error) {
	if errors.Is(err, denovaapp.ErrWorkspaceTransition) {
		writeJSON(c, consts.StatusConflict, map[string]any{
			"error":   messageKey(c, "api.project.transitioning"),
			"code":    "project_transitioning",
			"details": map[string]string{"project_id": strings.TrimSpace(projectID)},
		})
		return
	}
	writeWorkspaceChangeError(c, err)
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
