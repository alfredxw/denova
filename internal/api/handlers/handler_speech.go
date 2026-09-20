package handlers

import (
	"context"
	"errors"
	"log/slog"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	"denova/config"
	appsettings "denova/internal/app/settings"
	"denova/internal/speech"
)

// HandleSpeech uses saved user settings, or an explicit unsaved preview. Audio
// is ephemeral and never enters a Project Store or the model context.
func (h *Handlers) HandleSpeech(ctx context.Context, c *app.RequestContext) {
	var body struct {
		ID      string                 `json:"id"`
		Input   string                 `json:"input"`
		Preview *config.SpeechSettings `json:"preview,omitempty"`
	}
	if err := decodeStrictJSONRequest(c.Request.Body(), &body); err != nil {
		writeSpeechError(c, consts.StatusBadRequest, speech.InvalidInput)
		return
	}
	settings := body.Preview
	if settings == nil {
		layered, err := h.app.SettingsService().Snapshot(appsettings.Global())
		if err != nil {
			writeSpeechError(c, consts.StatusInternalServerError, speech.Service)
			return
		}
		settings = layered.Effective.Speech
	}
	if settings == nil {
		writeSpeechError(c, consts.StatusBadRequest, speech.Unconfigured)
		return
	}
	data, err := h.speechRequests.Synthesize(ctx, body.ID, *settings, body.Input)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			slog.Debug("Speech synthesis cancelled", "speech_request_id", body.ID)
			c.Status(consts.StatusNoContent)
			return
		}
		key := speech.ErrorKey(err)
		slog.Warn("Speech synthesis failed", "reason", key)
		writeSpeechError(c, consts.StatusBadGateway, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	slog.Debug("Speech synthesis completed", "speech_request_id", body.ID, "audio_bytes", len(data))
	c.Data(consts.StatusOK, "audio/mpeg", data)
}

func (h *Handlers) HandleSpeechCancel(_ context.Context, c *app.RequestContext) {
	h.speechRequests.Cancel(c.Param("id"))
	c.Status(consts.StatusNoContent)
}

func writeSpeechError(c *app.RequestContext, status int, err error) {
	key := speech.ErrorKey(err)
	c.JSON(status, map[string]string{"error": messageKey(c, key), "code": key})
}
