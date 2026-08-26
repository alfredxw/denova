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
		Endpoint config.ImageAPIEndpointSettings `json:"endpoint"`
		Profile  config.ImageAPIProfileSettings  `json:"profile"`
	}
	if err := decodeStrictJSONRequest(c.Request.Body(), &body); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
		return
	}
	result, err := h.app.Images().Ping(ctx, body.Endpoint, body.Profile)
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

// HandleComfyUIWorkflowDiscovery lists saved UI workflows and their executable
// snapshot status for an inline, potentially unsaved image profile.
func (h *Handlers) HandleComfyUIWorkflowDiscovery(ctx context.Context, c *app.RequestContext) {
	var body struct {
		Endpoint config.ImageAPIEndpointSettings `json:"endpoint"`
		Profile  config.ImageAPIProfileSettings  `json:"profile"`
	}
	if err := decodeStrictJSONRequest(c.Request.Body(), &body); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
		return
	}
	result, err := h.app.Images().DiscoverComfyUIWorkflows(ctx, body.Endpoint, body.Profile)
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

// HandleComfyUIWorkflowLoad imports the latest fresh successful API snapshot
// for one saved workflow selected from discovery.
func (h *Handlers) HandleComfyUIWorkflowLoad(ctx context.Context, c *app.RequestContext) {
	var body struct {
		Endpoint config.ImageAPIEndpointSettings `json:"endpoint"`
		Profile  config.ImageAPIProfileSettings  `json:"profile"`
		Path     string                          `json:"path"`
	}
	if err := decodeStrictJSONRequest(c.Request.Body(), &body); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
		return
	}
	result, err := h.app.Images().LoadComfyUIWorkflow(ctx, body.Endpoint, body.Profile, body.Path)
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
