package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"

	"denova/config"
	"denova/internal/agent"
	"denova/internal/book"
	"denova/internal/imagepreset"
	"denova/internal/loreimage"
)

// LoreAppService 负责资料库 CRUD。
type LoreAppService struct {
	app *App
}

type LoreItemImageGenerateRequest struct {
	Instruction   string `json:"instruction,omitempty"`
	ImagePresetID string `json:"image_preset_id,omitempty"`
	ProfileID     string `json:"profile_id,omitempty"`
}

type LoreImagesGenerateRequest struct {
	ItemIDs           []string `json:"item_ids"`
	Instruction       string   `json:"instruction,omitempty"`
	OverwriteExisting bool     `json:"overwrite_existing,omitempty"`
	ImagePresetID     string   `json:"image_preset_id,omitempty"`
	ProfileID         string   `json:"profile_id,omitempty"`
}

type LoreImageProgressEvent struct {
	ItemID  string         `json:"item_id"`
	Index   int            `json:"index"`
	Total   int            `json:"total"`
	Status  string         `json:"status"`
	Message string         `json:"message,omitempty"`
	Item    *book.LoreItem `json:"item,omitempty"`
}

var ErrLoreImageTaskRunning = errors.New("已有资料项图片生成任务正在运行")

func (a *App) LoreItems() ([]book.LoreItem, error) {
	return a.lore().LoreItems()
}

func (s *LoreAppService) LoreItems() ([]book.LoreItem, error) {
	state := s.bookState()
	if state == nil {
		return nil, ErrNoWorkspace
	}
	return book.NewLoreStore(state.Workspace()).ListAll()
}

func (a *App) CreateLoreItem(input book.LoreItemInput) (book.LoreItem, error) {
	return a.lore().CreateLoreItem(input)
}

func (s *LoreAppService) CreateLoreItem(input book.LoreItemInput) (book.LoreItem, error) {
	state := s.bookState()
	if state == nil {
		return book.LoreItem{}, ErrNoWorkspace
	}
	return book.NewLoreStore(state.Workspace()).Create(input)
}

func (a *App) UpdateLoreItem(id string, input book.LoreItemInput) (book.LoreItem, error) {
	return a.lore().UpdateLoreItem(id, input)
}

func (s *LoreAppService) UpdateLoreItem(id string, input book.LoreItemInput) (book.LoreItem, error) {
	state := s.bookState()
	if state == nil {
		return book.LoreItem{}, ErrNoWorkspace
	}
	return book.NewLoreStore(state.Workspace()).Update(id, input)
}

func (a *App) DeleteLoreItem(id string) error {
	return a.lore().DeleteLoreItem(id)
}

func (s *LoreAppService) DeleteLoreItem(id string) error {
	state := s.bookState()
	if state == nil {
		return ErrNoWorkspace
	}
	return book.NewLoreStore(state.Workspace()).Delete(id)
}

func (a *App) GenerateLoreItemImage(ctx context.Context, id string, request LoreItemImageGenerateRequest) (book.LoreItem, error) {
	return a.lore().GenerateLoreItemImage(ctx, id, request)
}

func (s *LoreAppService) GenerateLoreItemImage(ctx context.Context, id string, request LoreItemImageGenerateRequest) (book.LoreItem, error) {
	return s.generateLoreItemImage(ctx, id, request)
}

func (a *App) ClearLoreItemImage(id string) (book.LoreItem, error) {
	return a.lore().ClearLoreItemImage(id)
}

func (s *LoreAppService) ClearLoreItemImage(id string) (book.LoreItem, error) {
	state := s.bookState()
	if state == nil {
		return book.LoreItem{}, ErrNoWorkspace
	}
	return book.NewLoreStore(state.Workspace()).SetImage(id, nil)
}

func (a *App) StartLoreImagesGenerateTask(request LoreImagesGenerateRequest) (*Task, error) {
	return a.lore().StartLoreImagesGenerateTask(request)
}

func (s *LoreAppService) StartLoreImagesGenerateTask(request LoreImagesGenerateRequest) (*Task, error) {
	request.ItemIDs = dedupeLoreImageItemIDs(request.ItemIDs)
	if len(request.ItemIDs) == 0 {
		return nil, fmt.Errorf("请选择需要生成图片的资料项")
	}
	a := s.app
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.activeLoreImageTask != nil {
		if !a.activeLoreImageTask.Finished() {
			return nil, ErrLoreImageTaskRunning
		}
		a.activeLoreImageTask = nil
	}

	task := NewTask(func(ctx context.Context, task *Task, emit func(agent.Event)) {
		defer s.clearLoreImageTask(task)
		log.Printf("[lore-image] batch begin task_id=%s items=%d overwrite=%v", task.ID(), len(request.ItemIDs), request.OverwriteExisting)
		emit(agent.Event{Type: "thinking", Data: map[string]string{"content": "正在准备批量生成资料项图片。"}})
		generated, skipped, failed := s.runLoreImagesGenerateBatch(ctx, request, emit)
		if ctx.Err() != nil {
			emit(agent.Event{Type: "aborted", Data: map[string]string{"message": "资料项图片生成已中止"}})
			return
		}
		emit(agent.Event{Type: "done", Data: map[string]any{
			"status":    "ok",
			"total":     len(request.ItemIDs),
			"generated": generated,
			"skipped":   skipped,
			"failed":    failed,
		}})
		log.Printf("[lore-image] batch done task_id=%s generated=%d skipped=%d failed=%d", task.ID(), generated, skipped, failed)
	})

	a.activeLoreImageTask = task
	return task, nil
}

func (s *LoreAppService) clearLoreImageTask(task *Task) {
	a := s.app
	a.mu.Lock()
	if a.activeLoreImageTask == task {
		a.activeLoreImageTask = nil
	}
	a.mu.Unlock()
}

func (a *App) AbortLoreImagesGenerateTask() {
	a.mu.RLock()
	task := a.activeLoreImageTask
	a.mu.RUnlock()
	if task != nil {
		task.Abort()
	}
}

func (s *LoreAppService) runLoreImagesGenerateBatch(ctx context.Context, request LoreImagesGenerateRequest, emit func(agent.Event)) (generated, skipped, failed int) {
	ids := request.ItemIDs
	total := len(ids)
	if total == 0 {
		emit(agent.Event{Type: "error", Data: map[string]string{"message": "请选择需要生成图片的资料项"}})
		return 0, 0, 1
	}
	store, _, _, err := s.loreImageRuntimeSnapshot()
	if err != nil {
		emit(agent.Event{Type: "error", Data: map[string]string{"message": err.Error()}})
		return 0, 0, total
	}
	for index, id := range ids {
		if ctx.Err() != nil {
			return generated, skipped, failed
		}
		position := index + 1
		item, err := store.ReadAny(id)
		if err != nil {
			failed++
			emitLoreImageProgress(emit, id, position, total, "error", err.Error(), nil)
			continue
		}
		if item.Image != nil && item.Image.ImagePath != "" && !request.OverwriteExisting {
			skipped++
			emitLoreImageProgress(emit, item.ID, position, total, "skipped", "已有图片，已跳过", &item)
			continue
		}
		emitLoreImageProgress(emit, item.ID, position, total, "running", "正在生成图片", &item)
		updated, err := s.generateLoreItemImage(ctx, item.ID, LoreItemImageGenerateRequest{
			Instruction:   request.Instruction,
			ImagePresetID: request.ImagePresetID,
			ProfileID:     request.ProfileID,
		})
		if err != nil {
			failed++
			emitLoreImageProgress(emit, item.ID, position, total, "error", err.Error(), nil)
			continue
		}
		generated++
		emitLoreImageProgress(emit, updated.ID, position, total, "success", "图片已生成", &updated)
		emit(agent.Event{Type: "lore_image_result", Data: map[string]any{"item_id": updated.ID, "item": updated}})
	}
	return generated, skipped, failed
}

func (s *LoreAppService) generateLoreItemImage(ctx context.Context, id string, request LoreItemImageGenerateRequest) (book.LoreItem, error) {
	store, cfg, bookService, err := s.loreImageRuntimeSnapshot()
	if err != nil {
		return book.LoreItem{}, err
	}
	item, err := store.ReadAny(id)
	if err != nil {
		return book.LoreItem{}, err
	}
	preset, err := resolveLoreImagePreset(cfg, request.ImagePresetID)
	if err != nil {
		return book.LoreItem{}, err
	}
	image, err := loreimage.NewService().Generate(ctx, &cfg, bookService, loreimage.GenerateRequest{
		Item:              item,
		Instruction:       request.Instruction,
		ImagePresetID:     preset.ID,
		ImagePresetPrompt: preset.PromptForTargets(imagepreset.TargetToolRequest),
		ProfileID:         request.ProfileID,
	})
	if err != nil {
		return book.LoreItem{}, err
	}
	if err := ctx.Err(); err != nil {
		return book.LoreItem{}, err
	}
	updated, err := store.SetImage(item.ID, &image)
	if err != nil {
		return book.LoreItem{}, err
	}
	log.Printf("[lore-image] generated item_id=%s path=%s", updated.ID, image.ImagePath)
	return updated, nil
}

func (s *LoreAppService) loreImageRuntimeSnapshot() (*book.LoreStore, config.Config, *book.Service, error) {
	a := s.app
	a.mu.RLock()
	if a.workspace == "" || a.bookService == nil || a.bookState == nil {
		a.mu.RUnlock()
		return nil, config.Config{}, nil, ErrNoWorkspace
	}
	if a.cfg == nil {
		a.mu.RUnlock()
		return nil, config.Config{}, nil, fmt.Errorf("运行配置未初始化")
	}
	cfg := *a.cfg
	workspace := a.workspace
	bookService := a.bookService
	novaDir := cfg.NovaDir
	a.mu.RUnlock()

	cfg.Workspace = workspace
	if layered, err := config.LoadLayeredWithStartupConfig(novaDir, workspace); err == nil {
		applyLayeredSettingsToConfig(&cfg, layered)
	} else {
		log.Printf("[lore-image] 加载分层配置失败 workspace=%s err=%v", workspace, err)
	}
	return book.NewLoreStore(workspace), cfg, bookService, nil
}

func resolveLoreImagePreset(cfg config.Config, requestedID string) (imagepreset.Preset, error) {
	presetID := imagepreset.NormalizeID(requestedID)
	if presetID == "" {
		presetID = imagepreset.NormalizeID(cfg.IDEImagePresetID)
	}
	if presetID == "" {
		presetID = imagepreset.DefaultID
	}
	if strings.TrimSpace(cfg.NovaDir) == "" {
		return imagepreset.DefaultPreset(), nil
	}
	return imagepreset.NewLibrary(cfg.NovaDir).Get(presetID)
}

func dedupeLoreImageItemIDs(ids []string) []string {
	out := make([]string, 0, len(ids))
	seen := map[string]bool{}
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}

func emitLoreImageProgress(emit func(agent.Event), itemID string, index, total int, status, message string, item *book.LoreItem) {
	emit(agent.Event{Type: "lore_image_progress", Data: LoreImageProgressEvent{
		ItemID:  itemID,
		Index:   index,
		Total:   total,
		Status:  status,
		Message: message,
		Item:    item,
	}})
}

func (s *LoreAppService) bookState() *book.State {
	a := s.app
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.bookState
}
