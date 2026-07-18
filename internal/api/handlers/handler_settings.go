package handlers

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	"denova/config"
	appsvc "denova/internal/app"
)

// handleSettingsGet GET /api/settings — 返回用户设置、工作区 Agent 定制及生效快照。
func (h *Handlers) HandleSettingsGet(ctx context.Context, c *app.RequestContext) {
	layered, err := h.app.Settings()
	if err != nil {
		writeError(c, consts.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, layered)
}

// handleSettingsUserUpdate PUT /api/settings/user — 持久化用户级配置。
func (h *Handlers) HandleSettingsUserUpdate(ctx context.Context, c *app.RequestContext) {
	body, baseRevision, err := bindSettingsUpdate(c)
	if err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
		return
	}
	layered, err := h.app.UpdateUserSettings(body, baseRevision)
	if err != nil {
		if errors.Is(err, config.ErrSettingsRevisionConflict) {
			writeErrorKey(c, consts.StatusConflict, "api.settings.revisionConflict")
			return
		}
		if key := settingsErrorKey(err); key != "" {
			writeErrorKey(c, consts.StatusBadRequest, key)
			return
		}
		writeError(c, consts.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, layered)
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

// handleSettingsWorkspaceUpdate PUT /api/settings/workspace — 持久化工作区级 Agent 定制。
func (h *Handlers) HandleSettingsWorkspaceUpdate(ctx context.Context, c *app.RequestContext) {
	body, baseRevision, err := bindSettingsUpdate(c)
	if err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
		return
	}
	layered, err := h.app.UpdateWorkspaceSettings(body, baseRevision)
	if err != nil {
		if errors.Is(err, config.ErrSettingsRevisionConflict) {
			writeErrorKey(c, consts.StatusConflict, "api.settings.revisionConflict")
			return
		}
		if errors.Is(err, appsvc.ErrNoWorkspaceOpen) {
			writeErrorKey(c, consts.StatusBadRequest, "api.settings.workspaceMissing")
			return
		}
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, layered)
}

func bindSettingsUpdate(c *app.RequestContext) (config.Settings, string, error) {
	raw := c.Request.Body()
	var envelope struct {
		Settings     *config.Settings `json:"settings"`
		BaseRevision string           `json:"base_revision"`
	}
	if err := json.Unmarshal(raw, &envelope); err == nil && envelope.Settings != nil {
		return *envelope.Settings, envelope.BaseRevision, nil
	}
	var body config.Settings
	if err := json.Unmarshal(raw, &body); err != nil {
		return config.Settings{}, "", err
	}
	return body, "", nil
}
