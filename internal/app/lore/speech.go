package loreapp

import (
	"context"
	"errors"
	"log/slog"
	"strings"

	"denova/config"
	booklore "denova/internal/book/lore"
	"denova/internal/speech"
)

type ItemSpeechGenerateRequest struct {
	Name string `json:"name"`
	Text string `json:"text"`
}

// GenerateItemSpeech uses the saved global voice and appends an immutable MP3.
// Text is also the initial material description, never a change to the lore body.
func (service *Service) GenerateItemSpeech(ctx context.Context, projectID, id string, request ItemSpeechGenerateRequest, settings config.SpeechSettings) (booklore.Item, error) {
	var item booklore.Item
	_, err := service.withStore(ctx, projectID, func(store *booklore.Store) error {
		if _, err := store.ReadAny(id); err != nil {
			return err
		}
		text := strings.TrimSpace(request.Text)
		data, err := speech.Synthesize(ctx, settings, text)
		if err != nil {
			return err
		}
		item, err = store.SaveMaterial(ctx, id, booklore.MaterialFile{
			Filename: "speech.mp3", Data: data, Source: booklore.AssetSource{Kind: "generated"},
			Entry: booklore.MaterialEntry{Name: strings.TrimSpace(request.Name), Description: text},
		})
		if errors.Is(err, booklore.ErrMaterialInvalid) {
			return speech.Audio
		}
		return err
	})
	if err != nil {
		slog.WarnContext(ctx, "[lore-speech] generation failed", "project_id", projectID, "item_id", id, "error", err)
	}
	return item, err
}
