package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"denova/config"
	agents "denova/internal/agents"
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
	return a.lore().generateLoreItemImage(ctx, "", id, request)
}

// GenerateLoreItemImageForWorkspace prevents a request prepared for one book
// from following a concurrent workspace switch into another book.
func (a *App) GenerateLoreItemImageForWorkspace(ctx context.Context, expectedWorkspace, id string, request LoreItemImageGenerateRequest) (book.LoreItem, error) {
	if err := a.ValidateWorkspaceIdentity(expectedWorkspace); err != nil {
		return book.LoreItem{}, err
	}
	return a.lore().generateLoreItemImage(ctx, expectedWorkspace, id, request)
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

func (a *App) StartLoreImagesGenerateTask(ctx context.Context, request LoreImagesGenerateRequest) (*Task, error) {
	return a.lore().startLoreImagesGenerateTask(ctx, "", request)
}

// StartLoreImagesGenerateTaskForWorkspace binds the background task to the
// workspace identity carried by the initiating HTTP request.
func (a *App) StartLoreImagesGenerateTaskForWorkspace(ctx context.Context, expectedWorkspace string, request LoreImagesGenerateRequest) (*Task, error) {
	if err := a.ValidateWorkspaceIdentity(expectedWorkspace); err != nil {
		return nil, err
	}
	return a.lore().startLoreImagesGenerateTask(ctx, expectedWorkspace, request)
}

func (s *LoreAppService) startLoreImagesGenerateTask(ctx context.Context, expectedWorkspace string, request LoreImagesGenerateRequest) (*Task, error) {
	request.ItemIDs = dedupeLoreImageItemIDs(request.ItemIDs)
	if len(request.ItemIDs) == 0 {
		return nil, fmt.Errorf("请选择需要生成图片的资料项")
	}
	a := s.app
	a.mu.Lock()
	if a.workspaceTransition {
		a.mu.Unlock()
		return nil, ErrWorkspaceTransition
	}
	workspace := a.workspace
	if strings.TrimSpace(expectedWorkspace) != "" && lifecycleWorkspaceKey(expectedWorkspace) != lifecycleWorkspaceKey(workspace) {
		a.mu.Unlock()
		return nil, fmt.Errorf("%w: expected=%q actual=%q", ErrWorkspaceChanged, expectedWorkspace, workspace)
	}
	if a.activeLoreImageTask != nil {
		if !a.activeLoreImageTask.Finished() {
			a.mu.Unlock()
			return nil, ErrLoreImageTaskRunning
		}
		a.activeLoreImageTask = nil
	}
	a.mu.Unlock()

	task, err := NewRegisteredTaskWithContext(ctx, func(task *Task) error {
		a.mu.Lock()
		defer a.mu.Unlock()
		if a.activeLoreImageTask != nil && !a.activeLoreImageTask.Finished() {
			return ErrLoreImageTaskRunning
		}
		if err := a.registerWorkspaceTaskLocked(task, workspace, true); err != nil {
			return err
		}
		a.activeLoreImageTask = task
		return nil
	}, func(ctx context.Context, task *Task, emit func(agents.Event)) {
		defer s.clearLoreImageTask(task)
		slog.InfoContext(ctx, fmt.Sprintf("[lore-image] batch begin task_id=%s items=%d overwrite=%v", task.ID(), len(request.ItemIDs), request.OverwriteExisting))
		emit(agents.Event{Type: "thinking", Data: map[string]string{"content": "正在准备批量生成资料项图片。"}})
		generated, skipped, failed := s.runLoreImagesGenerateBatch(ctx, workspace, request, emit)
		if ctx.Err() != nil {
			emit(agents.Event{Type: "aborted", Data: map[string]string{"message": "资料项图片生成已中止"}})
			return
		}
		emit(agents.Event{Type: "done", Data: map[string]any{
			"status":    "ok",
			"total":     len(request.ItemIDs),
			"generated": generated,
			"skipped":   skipped,
			"failed":    failed,
		}})
		slog.ErrorContext(ctx, fmt.Sprintf("[lore-image] batch done task_id=%s generated=%d skipped=%d failed=%d", task.ID(), generated, skipped, failed))
	})
	if err != nil {
		return nil, err
	}
	return task, nil
}

func (s *LoreAppService) clearLoreImageTask(task *Task) {
	a := s.app
	a.unregisterWorkspaceTask(task)
	a.mu.Lock()
	if a.activeLoreImageTask == task {
		a.activeLoreImageTask = nil
	}
	a.mu.Unlock()
}

// AbortLoreImagesGenerateTaskForWorkspace atomically binds an abort command to
// the workspace that issued it. Holding the App read lock through Abort keeps a
// concurrent workspace switch or replacement task from slipping between the
// identity check and the command.
func (a *App) AbortLoreImagesGenerateTaskForWorkspace(expectedWorkspace string) error {
	a.mu.RLock()
	defer a.mu.RUnlock()

	actualWorkspace := strings.TrimSpace(a.workspace)
	expectedWorkspace = strings.TrimSpace(expectedWorkspace)
	if actualWorkspace == "" {
		return ErrNoWorkspace
	}
	if expectedWorkspace == "" || lifecycleWorkspaceKey(expectedWorkspace) != lifecycleWorkspaceKey(actualWorkspace) {
		return fmt.Errorf("%w: expected=%q actual=%q", ErrWorkspaceChanged, expectedWorkspace, actualWorkspace)
	}
	if a.activeLoreImageTask != nil {
		a.activeLoreImageTask.Abort()
	}
	return nil
}

func (s *LoreAppService) runLoreImagesGenerateBatch(ctx context.Context, workspace string, request LoreImagesGenerateRequest, emit func(agents.Event)) (generated, skipped, failed int) {
	ids := request.ItemIDs
	total := len(ids)
	if total == 0 {
		emit(agents.Event{Type: "error", Data: map[string]string{"message": "请选择需要生成图片的资料项"}})
		return 0, 0, 1
	}
	store, _, _, err := s.loreImageRuntimeSnapshot(workspace)
	if err != nil {
		emit(agents.Event{Type: "error", Data: map[string]string{"message": err.Error()}})
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
		updated, err := s.generateLoreItemImage(ctx, workspace, item.ID, LoreItemImageGenerateRequest{
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
		emit(agents.Event{Type: "lore_image_result", Data: map[string]any{"item_id": updated.ID, "item": updated}})
	}
	return generated, skipped, failed
}

func (s *LoreAppService) generateLoreItemImage(ctx context.Context, expectedWorkspace, id string, request LoreItemImageGenerateRequest) (book.LoreItem, error) {
	runtime, err := s.app.images().acquireWorkspaceRuntimeFor(ctx, expectedWorkspace)
	if err != nil {
		return book.LoreItem{}, err
	}
	defer runtime.Release()
	cfg := runtime.cfg
	if layered, loadErr := config.LoadLayeredWithStartupConfigAt(
		cfg.DataDir(), runtime.workspace, config.ProjectConfigPath(cfg.ProjectStateDir),
	); loadErr == nil {
		applyLayeredSettingsToConfig(&cfg, layered)
	} else {
		slog.ErrorContext(ctx, fmt.Sprintf("[lore-image] 加载分层配置失败 workspace=%s err=%v", runtime.workspace, loadErr))
	}
	store := book.NewLoreStore(runtime.workspace)
	item, err := store.ReadAny(id)
	if err != nil {
		return book.LoreItem{}, err
	}
	preset, err := resolveLoreImagePreset(cfg, request.ImagePresetID)
	if err != nil {
		return book.LoreItem{}, err
	}
	image, err := loreimage.NewService().Generate(runtime.Context(), &cfg, runtime.bookService, loreimage.GenerateRequest{
		Item:              item,
		Instruction:       request.Instruction,
		ImagePresetID:     preset.ID,
		ImagePresetPrompt: preset.PromptForTargets(imagepreset.TargetToolRequest),
		ProfileID:         request.ProfileID,
	})
	if err != nil {
		return book.LoreItem{}, err
	}
	if err := runtime.Context().Err(); err != nil {
		return book.LoreItem{}, err
	}
	updated, err := store.SetImage(item.ID, &image)
	if err != nil {
		return book.LoreItem{}, err
	}
	slog.InfoContext(ctx, fmt.Sprintf("[lore-image] generated item_id=%s path=%s", updated.ID, image.ImagePath))
	return updated, nil
}

func (s *LoreAppService) loreImageRuntimeSnapshot(expectedWorkspace string) (*book.LoreStore, config.Config, *book.Service, error) {
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
	if strings.TrimSpace(expectedWorkspace) != "" && lifecycleWorkspaceKey(expectedWorkspace) != lifecycleWorkspaceKey(workspace) {
		a.mu.RUnlock()
		return nil, config.Config{}, nil, fmt.Errorf("%w: expected=%q actual=%q", ErrWorkspaceChanged, expectedWorkspace, workspace)
	}
	bookService := a.bookService
	novaDir := cfg.DataDir()
	a.mu.RUnlock()

	cfg.Workspace = workspace
	if layered, err := config.LoadLayeredWithStartupConfigAt(
		novaDir, workspace, config.ProjectConfigPath(cfg.ProjectStateDir),
	); err == nil {
		applyLayeredSettingsToConfig(&cfg, layered)
	} else {
		slog.ErrorContext(context.Background(), fmt.Sprintf("[lore-image] 加载分层配置失败 workspace=%s err=%v", workspace, err))
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
	if strings.TrimSpace(cfg.DataDir()) == "" {
		return imagepreset.DefaultPreset(), nil
	}
	return imagepreset.NewLibrary(cfg.DataDir()).Get(presetID)
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

func emitLoreImageProgress(emit func(agents.Event), itemID string, index, total int, status, message string, item *book.LoreItem) {
	emit(agents.Event{Type: "lore_image_progress", Data: LoreImageProgressEvent{
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
