// Package lore owns Project-scoped lore classification and image generation.
// Stable Project resolution and generation fencing remain host responsibilities
// so this package never depends on foreground navigation state or the root App.
package loreapp

import (
	"context"
	"errors"

	imageapp "denova/internal/app/image"
	booklore "denova/internal/book/lore"
)

var ErrNoWorkspace = errors.New("no workspace is selected")

type ItemImageGenerateRequest struct {
	Mode          string `json:"mode,omitempty"`
	CommandID     string `json:"command_id,omitempty"`
	Prompt        string `json:"prompt,omitempty"`
	Instruction   string `json:"instruction,omitempty"`
	ImagePresetID string `json:"image_preset_id,omitempty"`
	ProfileID     string `json:"profile_id,omitempty"`
}

// Host is the narrow lifecycle boundary required by lore operations. It owns
// workspace identity and task leases; Service owns lore behavior and state.
type Host interface {
	WithLoreStore(context.Context, string, func(*booklore.Store) error) (string, error)
	ClassifyLoreItems(context.Context, string, []booklore.ClassificationInput) ([]booklore.ClassificationSuggestion, error)
}

type Service struct {
	host   Host
	images *imageapp.Service
}

func NewService(host Host, images *imageapp.Service) *Service {
	return &Service{host: host, images: images}
}
