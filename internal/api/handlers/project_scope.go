package handlers

import (
	"context"
	"errors"
	"log/slog"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	appsvc "denova/internal/app"
	"denova/internal/concurrency"
	projectdomain "denova/internal/project"
)

const projectScopeContextKey = "denova.project.scope"

// ProjectScopeMiddleware resolves and leases every /projects/:project_id
// request once. Handlers receive a canonical typed layout and never parse a
// path supplied by the client or fall back to the foreground Book.
func (handlers *Handlers) ProjectScopeMiddleware(ctx context.Context, c *app.RequestContext) {
	projectID := strings.TrimSpace(c.Param("project_id"))
	if projectID == "" {
		abortProjectScopeError(c, consts.StatusBadRequest, "api.project.idRequired")
		return
	}
	if err := projectdomain.ValidateID(projectID); err != nil {
		abortProjectScopeError(c, consts.StatusBadRequest, "api.project.idInvalid")
		return
	}
	operation, err := handlers.app.AcquireProjectOperation(ctx, projectID)
	if err != nil {
		writeProjectScopeResolutionError(c, projectID, err)
		return
	}
	defer operation.Release()
	layout := operation.Layout()
	c.Set(projectScopeContextKey, layout)
	slog.DebugContext(operation.Context(), "[internal/api/handlers/project_scope.go] resolved project request scope",
		"project_id", layout.ProjectID,
		"project_type", layout.Type,
		"workspace", layout.ContentRoot,
	)
	c.Next(operation.Context())
}

func projectScope(c *app.RequestContext) projectdomain.Layout {
	if c == nil {
		return projectdomain.Layout{}
	}
	value, exists := c.Get(projectScopeContextKey)
	if !exists {
		return projectdomain.Layout{}
	}
	layout, _ := value.(projectdomain.Layout)
	return layout
}

func requireProjectScope(c *app.RequestContext) (projectdomain.Layout, bool) {
	layout := projectScope(c)
	if strings.TrimSpace(layout.ProjectID) == "" {
		abortProjectScopeError(c, consts.StatusInternalServerError, "api.project.scopeMissing")
		return projectdomain.Layout{}, false
	}
	return layout, true
}

func writeProjectScopeResolutionError(c *app.RequestContext, projectID string, err error) {
	slog.WarnContext(context.Background(), "[internal/api/handlers/project_scope.go] project request scope resolution failed",
		"project_id", projectID,
		"error", err,
	)
	switch {
	case errors.Is(err, projectdomain.ErrNotFound):
		abortProjectScopeError(c, consts.StatusNotFound, "api.project.notFound")
	case errors.Is(err, projectdomain.ErrArchived):
		abortProjectScopeError(c, consts.StatusConflict, "api.project.archived")
	case errors.Is(err, projectdomain.ErrUnavailable):
		abortProjectScopeError(c, consts.StatusConflict, "api.project.unavailable")
	case errors.Is(err, appsvc.ErrWorkspaceTransition),
		errors.Is(err, concurrency.ErrClosing),
		errors.Is(err, concurrency.ErrClosed):
		abortProjectScopeError(c, consts.StatusConflict, "api.project.transitioning")
	default:
		abortProjectScopeError(c, consts.StatusInternalServerError, "api.project.resolveFailed")
	}
}

func abortProjectScopeError(c *app.RequestContext, status int, key string) {
	c.AbortWithStatusJSON(status, map[string]string{"error": messageKey(c, key)})
}
