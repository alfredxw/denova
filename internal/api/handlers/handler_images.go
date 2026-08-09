package handlers

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	"denova/config"
	novaApp "denova/internal/app"
	imageapp "denova/internal/app/image"
	imagegen "denova/internal/image/generation"
)

// HandleImagePing validates an inline, potentially unsaved image profile with
// one minimal real generation request and does not persist the returned image.
func (h *Handlers) HandleImagePing(ctx context.Context, c *app.RequestContext) {
	var body struct {
		Profile config.ImageAPIProfileSettings `json:"profile"`
	}
	if err := decodeStrictJSONRequest(c.Request.Body(), &body); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
		return
	}
	result, err := h.app.Images().Ping(ctx, body.Profile)
	if err != nil {
		if imageapp.IsProviderRequestError(err) {
			writeError(c, consts.StatusUnprocessableEntity, err.Error())
			return
		}
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, result)
}

func (h *Handlers) HandleImageGenerate(ctx context.Context, c *app.RequestContext) {
	var req imagegen.GenerateRequest
	if err := c.BindJSON(&req); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
		return
	}
	result, err := h.app.Images().Generate(ctx, req)
	if err != nil {
		if err == novaApp.ErrNoWorkspace {
			writeErrorKey(c, consts.StatusBadRequest, "api.settings.workspaceMissing")
			return
		}
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, result)
}
