package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	"denova/config"
	"denova/internal/agentprofiles"
	appsvc "denova/internal/app"
	appsettings "denova/internal/app/settings"
)

// HandleSettingsGet returns persisted layers and their resolved runtime view.
func (h *Handlers) HandleSettingsGet(ctx context.Context, c *app.RequestContext) {
	layered, err := h.app.SettingsService().Snapshot(settingsTarget(c))
	if err != nil {
		writeError(c, consts.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, layered)
}

// HandleAgentApprovalRuleDelete atomically revokes one server-generated user
// rule without replacing the surrounding collection from a stale UI snapshot.
func (h *Handlers) HandleAgentApprovalRuleDelete(ctx context.Context, c *app.RequestContext) {
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", "agent approval rule id is required")
		return
	}
	_, layered, err := h.app.SettingsService().RemoveAgentApprovalRule(id)
	if err != nil {
		writeError(c, consts.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, layered)
}

// HandleSettingsPatch applies only fields present in changes. Omitted fields
// remain untouched and JSON null clears an inherited value.
func (h *Handlers) HandleSettingsPatch(ctx context.Context, c *app.RequestContext) {
	var body struct {
		Layer        string          `json:"layer"`
		BaseRevision string          `json:"base_revision,omitempty"`
		Changes      json.RawMessage `json:"changes"`
	}
	if err := decodeStrictJSONRequest(c.Request.Body(), &body); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
		return
	}
	layer, err := config.ParseSettingsLayer(body.Layer)
	if err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
		return
	}
	layered, err := h.app.SettingsService().Patch(settingsTarget(c), layer, body.Changes, body.BaseRevision)
	if err != nil {
		if writeSettingsMutationError(c, err) {
			return
		}

		switch {
		case errors.Is(err, config.ErrSettingsRevisionConflict):
			writeErrorKey(c, consts.StatusConflict, "api.settings.revisionConflict")
		case errors.Is(err, appsvc.ErrNoWorkspaceOpen):
			writeErrorKey(c, consts.StatusBadRequest, "api.settings.workspaceMissing")
		case errors.Is(err, config.ErrInvalidTerminalCommand),
			errors.Is(err, config.ErrInvalidAgentQuickPrompt),
			errors.Is(err, config.ErrInvalidSettingsPatch),
			errors.Is(err, config.ErrUnsupportedSettingsLayer):
			writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
		default:
			if key := settingsErrorKey(err); key != "" {
				writeErrorKey(c, consts.StatusBadRequest, key)
				return
			}
			writeError(c, consts.StatusInternalServerError, err.Error())
		}
		return
	}
	writeJSON(c, consts.StatusOK, layered)
}

func settingsTarget(c *app.RequestContext) appsettings.Target {
	if layout := projectScope(c); layout.ProjectID != "" {
		return appsettings.Project(layout.ProjectID)
	}
	return appsettings.Global()
}

func settingsErrorKey(err error) string {
	switch {
	case errors.Is(err, config.ErrRemoteAccessUsernameRequired):
		return "api.settings.lanUsernameRequired"
	case errors.Is(err, config.ErrRemoteAccessPasswordRequired):
		return "api.settings.lanPasswordRequired"
	default:
		return ""
	}
}

// writeSettingsMutationError exposes per-file outcomes without leaking raw
// storage errors, including when independent files have already been saved.
func writeSettingsMutationError(c *app.RequestContext, err error) bool {
	var partial *agentprofiles.MutationError
	if !errors.As(err, &partial) {
		return false
	}
	status := consts.StatusInternalServerError
	if errors.Is(err, config.ErrInvalidAgentProfile) {
		status = consts.StatusBadRequest
	}
	if errors.Is(err, config.ErrSettingsRevisionConflict) {
		status = consts.StatusConflict
	}
	writeJSON(c, status, map[string]any{
		"error":   messageKey(c, "api.settings.fileSaveFailed", "paths", strings.Join(partial.FailedPaths(), ", ")),
		"code":    "settings_file_save_failed",
		"details": map[string]any{"files": partial.Files},
	})
	return true
}
