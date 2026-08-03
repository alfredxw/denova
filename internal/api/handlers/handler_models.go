package handlers

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	"denova/config"
	modelsapp "denova/internal/app/models"
)

// HandleModelCatalog returns executable protocols and optional provider
// presets. Custom endpoints use the compatible provider plus a protocol.
func (h *Handlers) HandleModelCatalog(_ context.Context, c *app.RequestContext) {
	catalog, err := h.app.Models().Catalog()
	if err != nil {
		writeError(c, consts.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, catalog)
}

// HandleModelList discovers optional upstream model suggestions for an inline,
// potentially unsaved profile. Custom model text remains valid independently.
func (h *Handlers) HandleModelList(ctx context.Context, c *app.RequestContext) {
	var body struct {
		Profile config.ModelProfileSettings `json:"profile"`
	}
	if err := decodeStrictJSONRequest(c.Request.Body(), &body); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
		return
	}
	result, err := h.app.Models().List(ctx, body.Profile)
	if err != nil {
		if modelsapp.IsProviderRequestError(err) {
			writeError(c, consts.StatusUnprocessableEntity, err.Error())
			return
		}
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, result)
}

// HandleModelPing validates an inline, potentially unsaved model profile with
// a minimal real generation request.
func (h *Handlers) HandleModelPing(ctx context.Context, c *app.RequestContext) {
	var body struct {
		Profile config.ModelProfileSettings `json:"profile"`
	}
	if err := decodeStrictJSONRequest(c.Request.Body(), &body); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
		return
	}
	result, err := h.app.Models().Ping(ctx, body.Profile)
	if err != nil {
		if modelsapp.IsProviderRequestError(err) {
			writeError(c, consts.StatusUnprocessableEntity, err.Error())
			return
		}
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, result)
}
