package handlers

import (
	"context"
	"errors"
	"os"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	projectfilesapp "denova/internal/app/projectfiles"
	workspacechange "denova/internal/workspace/change"
)

func (h *Handlers) HandleProjectFilesList(ctx context.Context, c *app.RequestContext) {
	projectID := strings.TrimSpace(c.Param("project_id"))
	if projectID == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.projectFiles.projectIDRequired")
		return
	}
	directory, err := h.app.ProjectFiles().ListDirectory(ctx, projectID, c.Query("path"), isTruthyQueryFlag(c.Query("include_ignored")))
	if err != nil {
		writeProjectFilesError(c, err, "api.projectFiles.listFailed")
		return
	}
	writeJSON(c, consts.StatusOK, directory)
}

func (h *Handlers) HandleProjectFileRead(ctx context.Context, c *app.RequestContext) {
	projectID := strings.TrimSpace(c.Param("project_id"))
	path := strings.TrimSpace(c.Query("path"))
	if projectID == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.projectFiles.projectIDRequired")
		return
	}
	if path == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.workspace.pathMissing")
		return
	}
	document, err := h.app.ProjectFiles().ReadFile(ctx, projectID, path)
	if err != nil {
		writeProjectFilesError(c, err, "api.projectFiles.readFailed")
		return
	}
	writeJSON(c, consts.StatusOK, document)
}

func (h *Handlers) HandleProjectFileAsset(ctx context.Context, c *app.RequestContext) {
	projectID := strings.TrimSpace(c.Param("project_id"))
	path := strings.TrimSpace(c.Query("path"))
	if projectID == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.projectFiles.projectIDRequired")
		return
	}
	if path == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.workspace.pathMissing")
		return
	}
	data, contentType, err := h.app.ProjectFiles().ReadAsset(ctx, projectID, path)
	if err != nil {
		writeProjectFilesError(c, err, "api.projectFiles.readFailed")
		return
	}
	c.Response.Header.Set("Cache-Control", "no-cache")
	c.Data(consts.StatusOK, contentType, data)
}

func (h *Handlers) HandleProjectFileSave(ctx context.Context, c *app.RequestContext) {
	projectID := strings.TrimSpace(c.Param("project_id"))
	if projectID == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.projectFiles.projectIDRequired")
		return
	}
	var request projectfilesapp.SaveRequest
	if err := c.BindJSON(&request); err != nil || strings.TrimSpace(request.Path) == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.workspace.pathContentRequired")
		return
	}
	result, err := h.app.ProjectFiles().SaveFile(ctx, projectID, request)
	if err != nil {
		writeProjectFilesError(c, err, "api.projectFiles.saveFailed")
		return
	}
	writeJSON(c, consts.StatusOK, map[string]any{
		"project_id": result.ProjectID,
		"path":       result.Path,
		"revision":   result.Revision,
		"changed":    result.Changed,
		"message":    messageKey(c, "api.projectFiles.saved"),
	})
}

func (h *Handlers) HandleProjectFileOperations(ctx context.Context, c *app.RequestContext) {
	projectID := strings.TrimSpace(c.Param("project_id"))
	if projectID == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.projectFiles.projectIDRequired")
		return
	}
	var request struct {
		Operations []projectfilesapp.Operation `json:"operations"`
	}
	if err := c.BindJSON(&request); err != nil || len(request.Operations) == 0 {
		writeErrorKey(c, consts.StatusBadRequest, "api.projectFiles.operationsRequired")
		return
	}
	results, err := h.app.ProjectFiles().ApplyOperations(ctx, projectID, request.Operations)
	if err != nil {
		writeProjectFilesError(c, err, "api.projectFiles.operationFailed")
		return
	}
	items := make([]map[string]any, 0, len(results))
	for _, result := range results {
		item := map[string]any{
			"id":   result.ID,
			"kind": result.Kind,
			"ok":   result.OK,
			"path": result.Path,
		}
		if !result.OK {
			item["code"] = result.Code
			item["error"] = projectFileOperationMessage(c, result.Code, result.Error)
		}
		items = append(items, item)
	}
	writeJSON(c, consts.StatusOK, map[string]any{"results": items})
}

func writeProjectFilesError(c *app.RequestContext, err error, fallbackKey string) {
	status := consts.StatusBadRequest
	code := "project_files_error"
	details := map[string]any(nil)
	message := messageKey(c, fallbackKey, "detail", err.Error())
	if os.IsNotExist(err) {
		status = consts.StatusNotFound
		code = workspacechange.ErrorCodeNotFound
		message = messageKey(c, "api.projectFiles.notFound")
	}
	var changeErr *workspacechange.Error
	if errors.As(err, &changeErr) {
		code = changeErr.Code
		details = changeErr.Details
		switch changeErr.Code {
		case workspacechange.ErrorCodeNotFound:
			status = consts.StatusNotFound
			message = messageKey(c, "api.projectFiles.notFound")
		case workspacechange.ErrorCodeRevisionConflict:
			status = consts.StatusConflict
			message = messageKey(c, "api.workspace.fileRevisionConflict")
		case workspacechange.ErrorCodeConflict, workspacechange.ErrorCodeDurabilityPending:
			status = consts.StatusConflict
		case workspacechange.ErrorCodeInvalidEdit:
			status = consts.StatusBadRequest
		default:
			status = consts.StatusInternalServerError
		}
	}
	payload := map[string]any{"error": message, "code": code}
	if len(details) > 0 {
		payload["details"] = details
	}
	writeJSON(c, status, payload)
}

func projectFileOperationMessage(c *app.RequestContext, code, detail string) string {
	switch code {
	case "target_exists":
		return messageKey(c, "api.workspace.targetExists")
	case "symlink_path":
		return messageKey(c, "api.projectFiles.symlinkPath")
	case workspacechange.ErrorCodeNotFound:
		return messageKey(c, "api.projectFiles.notFound")
	default:
		return messageKey(c, "api.projectFiles.operationFailed", "detail", detail)
	}
}
