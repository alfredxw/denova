// Package lore owns Project-scoped lore classification and image generation.
// Stable Project resolution and generation fencing remain host responsibilities
// so this package never depends on foreground navigation state or the root App.
package loreapp

import (
	"context"
	"errors"
	"sync"

	imageapp "denova/internal/app/image"
	"denova/internal/app/task"
	booklore "denova/internal/book/lore"
)

var (
	ErrNoWorkspace      = errors.New("no workspace is selected")
	ErrImageTaskRunning = errors.New("a lore image generation task is already running / 已有资料项图片生成任务正在运行")
)

type ItemImageGenerateRequest struct {
	Instruction   string `json:"instruction,omitempty"`
	ImagePresetID string `json:"image_preset_id,omitempty"`
	ProfileID     string `json:"profile_id,omitempty"`
}

type ImagesGenerateRequest struct {
	ItemIDs           []string `json:"item_ids"`
	Instruction       string   `json:"instruction,omitempty"`
	OverwriteExisting bool     `json:"overwrite_existing,omitempty"`
	ImagePresetID     string   `json:"image_preset_id,omitempty"`
	ProfileID         string   `json:"profile_id,omitempty"`
}

type ImageProgressEvent struct {
	ItemID  string         `json:"item_id"`
	Index   int            `json:"index"`
	Total   int            `json:"total"`
	Status  string         `json:"status"`
	Message string         `json:"message,omitempty"`
	Item    *booklore.Item `json:"item,omitempty"`
}

// Host is the narrow lifecycle boundary required by lore operations. It owns
// workspace identity and task leases; Service owns lore behavior and state.
type Host interface {
	WithLoreStore(context.Context, string, func(*booklore.Store) error) (string, error)
	RegisterLoreTask(task *task.Task, projectID string) (string, error)
	UnregisterLoreTask(task *task.Task)
	ClassifyLoreItems(context.Context, string, []booklore.ClassificationInput) ([]booklore.ClassificationSuggestion, error)
}

type activeImageTask struct {
	task      *task.Task
	projectID string
	workspace string
}

type Service struct {
	host   Host
	images *imageapp.Service

	activeMu sync.Mutex
	active   map[string]*activeImageTask
}

func NewService(host Host, images *imageapp.Service) *Service {
	return &Service{host: host, images: images, active: make(map[string]*activeImageTask)}
}
