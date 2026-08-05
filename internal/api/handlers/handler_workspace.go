package handlers

import (
	"context"
	"errors"
	"os"
	"strconv"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	projectfilesapp "denova/internal/app/projectfiles"
	"denova/internal/book"
	workspacechange "denova/internal/workspace/change"
)

// HandleWorkspaceSearch searches text and paths in the scoped Project.
func (h *Handlers) HandleWorkspaceSearch(ctx context.Context, c *app.RequestContext) {
	scope, ok := requireProjectScope(c)
	if !ok {
		return
	}
	query := c.Query("q")
	limit := book.DefaultSearchLimit
	if rawLimit := c.Query("limit"); rawLimit != "" {
		parsed, err := strconv.Atoi(rawLimit)
		if err != nil || parsed < 0 {
			writeErrorKey(c, consts.StatusBadRequest, "api.workspace.limitInvalid")
			return
		}
		limit = parsed
	}
	opts := book.SearchOptions{Regex: isTruthyQueryFlag(c.Query("regex"))}

	results, err := h.app.SearchProjectWorkspace(ctx, scope.ProjectID, query, limit, opts)
	if err != nil {
		if errors.Is(err, book.ErrInvalidSearchRegex) {
			writeErrorKey(c, consts.StatusBadRequest, "api.workspace.invalidRegex", "detail", err.Error())
			return
		}
		writeErrorKey(c, consts.StatusInternalServerError, "api.workspace.searchFailed", "detail", err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, map[string]any{"results": results})
}

// HandleWorkspaceReplace performs a recoverable CAS replacement in one Project.
func (h *Handlers) HandleWorkspaceReplace(ctx context.Context, c *app.RequestContext) {
	scope, ok := requireProjectScope(c)
	if !ok {
		return
	}
	var req struct {
		Query       string `json:"query"`
		Replacement string `json:"replacement"`
		Regex       bool   `json:"regex"`
	}
	if err := c.BindJSON(&req); err != nil || strings.TrimSpace(req.Query) == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.workspace.queryRequired")
		return
	}
	result, err := h.app.ReplaceProjectWorkspace(ctx, scope.ProjectID, projectfilesapp.ReplaceRequest{
		Query: req.Query, Replacement: req.Replacement, Regex: req.Regex,
	})
	if err != nil {
		key := "api.workspace.replaceFailed"
		if errors.Is(err, book.ErrInvalidSearchRegex) {
			key = "api.workspace.invalidRegex"
		} else if errors.Is(err, book.ErrRegexMatchesEmpty) {
			key = "api.workspace.regexMatchesEmpty"
		}
		var changeErr *workspacechange.Error
		if errors.As(err, &changeErr) {
			writeWorkspaceChangeError(c, err)
			return
		}
		status := consts.StatusInternalServerError
		if key != "api.workspace.replaceFailed" {
			status = consts.StatusBadRequest
		}
		writeErrorKey(c, status, key, "detail", err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, result)
}

func isTruthyQueryFlag(raw string) bool {
	return raw == "1" || raw == "true"
}

// handleWorkspaceSwitch POST /api/workspace/switch — 切换工作目录。
func (h *Handlers) HandleWorkspaceSwitch(ctx context.Context, c *app.RequestContext) {
	var req struct {
		Path string `json:"path"`
	}
	if err := c.BindJSON(&req); err != nil || req.Path == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.pathRequired")
		return
	}

	workspace, err := h.app.SwitchWorkspace(ctx, req.Path)
	if err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, map[string]string{
		"workspace": workspace,
		"message":   messageKey(c, "api.workspace.switched", "workspace", workspace),
	})
}

// handleWorkspaceCurrent GET /api/workspace/current — 获取当前工作目录。
func (h *Handlers) HandleWorkspaceCurrent(ctx context.Context, c *app.RequestContext) {
	hasState, _ := h.app.Status()
	writeJSON(c, consts.StatusOK, map[string]interface{}{
		"workspace":  h.app.Workspace(),
		"project_id": h.app.ProjectID(),
		"has_state":  hasState,
	})
}

func fileReadStatus(err error) int {
	if os.IsNotExist(err) {
		return consts.StatusNotFound
	}
	if isForbiddenFileError(err) {
		return consts.StatusForbidden
	}
	return consts.StatusBadRequest
}

func isForbiddenFileError(err error) bool {
	msg := err.Error()
	return msg == "路径不能为空" ||
		msg == "不允许使用绝对路径" ||
		msg == "路径不在 workspace 范围内" ||
		msg == "不允许操作隐藏文件或隐藏目录"
}
