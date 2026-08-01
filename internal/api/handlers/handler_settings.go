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

// HandleSettingsGet returns persisted layers and their resolved runtime view.
func (h *Handlers) HandleSettingsGet(ctx context.Context, c *app.RequestContext) {
	layered, err := h.app.Settings()
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
	layered, err := h.app.PatchSettings(layer, body.Changes, body.BaseRevision)
	if err != nil {
		switch {
		case errors.Is(err, config.ErrSettingsRevisionConflict):
			writeErrorKey(c, consts.StatusConflict, "api.settings.revisionConflict")
		case errors.Is(err, appsvc.ErrNoWorkspaceOpen):
			writeErrorKey(c, consts.StatusBadRequest, "api.settings.workspaceMissing")
		case errors.Is(err, config.ErrInvalidTerminalCommand),
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
