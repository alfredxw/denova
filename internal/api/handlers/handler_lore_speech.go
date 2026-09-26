package handlers

import (
	"context"
	"errors"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	loreapp "denova/internal/app/lore"
	appsettings "denova/internal/app/settings"
	"denova/internal/speech"
)

func (h *Handlers) HandleLoreItemSpeechGenerate(ctx context.Context, c *app.RequestContext) {
	scope, ok := requireProjectScope(c)
	if !ok {
		return
	}
	var body loreapp.ItemSpeechGenerateRequest
	if err := decodeStrictJSONRequest(c.Request.Body(), &body); err != nil {
		writeSpeechError(c, consts.StatusBadRequest, speech.InvalidInput)
		return
	}
	layered, err := h.app.SettingsService().Snapshot(appsettings.Global())
	if err != nil {
		writeSpeechError(c, consts.StatusInternalServerError, speech.Service)
		return
	}
	if layered.Effective.Speech == nil {
		writeSpeechError(c, consts.StatusBadRequest, speech.Unconfigured)
		return
	}
	item, err := h.app.Lore().GenerateItemSpeech(ctx, scope.ProjectID, c.Param("id"), body, *layered.Effective.Speech)
	if err != nil {
		var speechErr speech.Error
		if errors.As(err, &speechErr) {
			status := consts.StatusBadGateway
			if speechErr == speech.Unconfigured || speechErr == speech.InvalidInput || speechErr == speech.InvalidURL {
				status = consts.StatusBadRequest
			}
			writeSpeechError(c, status, err)
		} else {
			writeProjectBookError(c, err, "api.projectBook.loreFailed")
		}
		return
	}
	writeJSON(c, consts.StatusOK, item)
}
