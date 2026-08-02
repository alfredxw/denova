package loreapp

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"

	"denova/config"
	agentrun "denova/internal/agents/run"
	appsettings "denova/internal/app/settings"
	"denova/internal/app/task"
	booklore "denova/internal/book/lore"
	imageasset "denova/internal/image/asset"
	imagepreset "denova/internal/image/preset"
)

func (service *Service) Items() ([]booklore.Item, error) {
	var items []booklore.Item
	_, err := service.withStore("", func(store *booklore.Store) error {
		var listErr error
		items, listErr = store.ListAll()
		return listErr
	})
	return items, err
}

func (service *Service) CreateItem(input booklore.ItemInput) (booklore.Item, error) {
	var item booklore.Item
	_, err := service.withStore("", func(store *booklore.Store) error {
		var createErr error
		item, createErr = store.Create(input)
		return createErr
	})
	return item, err
}

func (service *Service) UpdateItem(id string, input booklore.ItemInput) (booklore.Item, error) {
	var item booklore.Item
	_, err := service.withStore("", func(store *booklore.Store) error {
		var updateErr error
		item, updateErr = store.Update(id, input)
		return updateErr
	})
	return item, err
}

func (service *Service) DeleteItem(id string) error {
	_, err := service.withStore("", func(store *booklore.Store) error { return store.Delete(id) })
	return err
}

func (service *Service) ClearItemImage(id string) (booklore.Item, error) {
	var item booklore.Item
	_, err := service.withStore("", func(store *booklore.Store) error {
		var updateErr error
		item, updateErr = store.SetImage(id, nil)
		return updateErr
	})
	return item, err
}

func (service *Service) GenerateItemImage(ctx context.Context, expectedWorkspace, id string, request ItemImageGenerateRequest) (booklore.Item, error) {
	if service == nil || service.images == nil {
		return booklore.Item{}, ErrNoWorkspace
	}
	runtime, err := service.images.AcquireRuntime(ctx, expectedWorkspace)
	if err != nil {
		return booklore.Item{}, err
	}
	defer runtime.Release()

	cfg := runtime.Config
	if layered, loadErr := config.LoadLayeredWithStartupConfigAt(
		cfg.DataDir(), runtime.Workspace, config.ProjectConfigPath(cfg.ProjectStateDir),
	); loadErr == nil {
		appsettings.ApplyLayered(&cfg, layered)
	} else {
		slog.ErrorContext(ctx, fmt.Sprintf("[lore-image] load layered settings failed workspace=%s err=%v", runtime.Workspace, loadErr))
	}
	store := booklore.NewStore(runtime.Workspace)
	item, err := store.ReadAny(id)
	if err != nil {
		return booklore.Item{}, err
	}
	preset, err := resolveImagePreset(cfg, request.ImagePresetID)
	if err != nil {
		return booklore.Item{}, err
	}
	generated, err := imageasset.NewService().GenerateLore(runtime.Context(), &cfg, runtime.BookService, imageasset.LoreGenerateRequest{
		Item:              item,
		Instruction:       request.Instruction,
		ImagePresetID:     preset.ID,
		ImagePresetPrompt: preset.PromptForTargets(imagepreset.TargetToolRequest),
		ProfileID:         request.ProfileID,
	})
	if err != nil {
		return booklore.Item{}, err
	}
	if err := runtime.Context().Err(); err != nil {
		return booklore.Item{}, err
	}
	updated, err := store.SetImage(item.ID, &generated)
	if err != nil {
		return booklore.Item{}, err
	}
	slog.InfoContext(ctx, fmt.Sprintf("[lore-image] generated item_id=%s path=%s", updated.ID, generated.ImagePath))
	return updated, nil
}

func (service *Service) GenerateItemImageForWorkspace(ctx context.Context, expectedWorkspace, id string, request ItemImageGenerateRequest) (booklore.Item, error) {
	if service == nil || service.host == nil {
		return booklore.Item{}, ErrNoWorkspace
	}
	if _, err := service.host.ValidateLoreWorkspace(expectedWorkspace); err != nil {
		return booklore.Item{}, err
	}
	return service.GenerateItemImage(ctx, expectedWorkspace, id, request)
}

func (service *Service) StartImagesGenerateTask(ctx context.Context, expectedWorkspace string, request ImagesGenerateRequest) (*task.Task, error) {
	request.ItemIDs = dedupeImageItemIDs(request.ItemIDs)
	if len(request.ItemIDs) == 0 {
		return nil, fmt.Errorf("select at least one lore item / 请选择需要生成图片的资料项")
	}
	if service == nil || service.host == nil {
		return nil, ErrNoWorkspace
	}

	var workspace string
	created, err := task.NewRegisteredWithContext(ctx, func(created *task.Task) error {
		service.activeMu.Lock()
		defer service.activeMu.Unlock()
		if service.active != nil && service.active.task != nil && !service.active.task.Finished() {
			return ErrImageTaskRunning
		}
		actualWorkspace, registerErr := service.host.RegisterLoreTask(created, expectedWorkspace)
		if registerErr != nil {
			return registerErr
		}
		workspace = actualWorkspace
		service.active = &activeImageTask{task: created, workspace: actualWorkspace}
		return nil
	}, func(ctx context.Context, running *task.Task, emit func(agentrun.Event)) {
		defer service.clearImageTask(running)
		slog.InfoContext(ctx, fmt.Sprintf("[lore-image] batch begin task_id=%s items=%d overwrite=%v", running.ID(), len(request.ItemIDs), request.OverwriteExisting))
		emit(agentrun.Event{Type: "thinking", Data: map[string]string{"content": "Preparing lore item images. / 正在准备批量生成资料项图片。"}})
		generated, skipped, failed := service.runImagesGenerateBatch(ctx, workspace, request, emit)
		if ctx.Err() != nil {
			emit(agentrun.Event{Type: "aborted", Data: map[string]string{"message": "Lore image generation was aborted. / 资料项图片生成已中止"}})
			return
		}
		emit(agentrun.Event{Type: "done", Data: map[string]any{
			"status": "ok", "total": len(request.ItemIDs), "generated": generated, "skipped": skipped, "failed": failed,
		}})
		slog.InfoContext(ctx, fmt.Sprintf("[lore-image] batch done task_id=%s generated=%d skipped=%d failed=%d", running.ID(), generated, skipped, failed))
	})
	if err != nil {
		return nil, err
	}
	return created, nil
}

func (service *Service) StartImagesGenerateTaskForWorkspace(ctx context.Context, expectedWorkspace string, request ImagesGenerateRequest) (*task.Task, error) {
	if service == nil || service.host == nil {
		return nil, ErrNoWorkspace
	}
	if _, err := service.host.ValidateLoreWorkspace(expectedWorkspace); err != nil {
		return nil, err
	}
	return service.StartImagesGenerateTask(ctx, expectedWorkspace, request)
}

func (service *Service) AbortImagesGenerateTask(expectedWorkspace string) error {
	if service == nil || service.host == nil {
		return ErrNoWorkspace
	}
	service.activeMu.Lock()
	defer service.activeMu.Unlock()
	workspace, err := service.host.ValidateLoreWorkspace(expectedWorkspace)
	if err != nil {
		return err
	}
	if service.active != nil && sameWorkspace(service.active.workspace, workspace) && service.active.task != nil {
		service.active.task.Abort()
	}
	return nil
}

func (service *Service) clearImageTask(completed *task.Task) {
	if service == nil || completed == nil {
		return
	}
	service.activeMu.Lock()
	if service.active != nil && service.active.task == completed {
		service.active = nil
	}
	service.activeMu.Unlock()
	if service.host != nil {
		service.host.UnregisterLoreTask(completed)
	}
}

func (service *Service) runImagesGenerateBatch(ctx context.Context, workspace string, request ImagesGenerateRequest, emit func(agentrun.Event)) (generated, skipped, failed int) {
	total := len(request.ItemIDs)
	if total == 0 {
		emit(agentrun.Event{Type: "error", Data: map[string]string{"message": "Select at least one lore item. / 请选择需要生成图片的资料项"}})
		return 0, 0, 1
	}
	store := booklore.NewStore(workspace)
	for index, id := range request.ItemIDs {
		if ctx.Err() != nil {
			return generated, skipped, failed
		}
		position := index + 1
		item, err := store.ReadAny(id)
		if err != nil {
			failed++
			emitImageProgress(emit, id, position, total, "error", err.Error(), nil)
			continue
		}
		if item.Image != nil && item.Image.ImagePath != "" && !request.OverwriteExisting {
			skipped++
			emitImageProgress(emit, item.ID, position, total, "skipped", "Existing image kept. / 已有图片，已跳过", &item)
			continue
		}
		emitImageProgress(emit, item.ID, position, total, "running", "Generating image. / 正在生成图片", &item)
		updated, err := service.GenerateItemImage(ctx, workspace, item.ID, ItemImageGenerateRequest{
			Instruction: request.Instruction, ImagePresetID: request.ImagePresetID, ProfileID: request.ProfileID,
		})
		if err != nil {
			failed++
			emitImageProgress(emit, item.ID, position, total, "error", err.Error(), nil)
			continue
		}
		generated++
		emitImageProgress(emit, updated.ID, position, total, "success", "Image generated. / 图片已生成", &updated)
		emit(agentrun.Event{Type: "lore_image_result", Data: map[string]any{"item_id": updated.ID, "item": updated}})
	}
	return generated, skipped, failed
}

func (service *Service) withStore(expectedWorkspace string, action func(*booklore.Store) error) (string, error) {
	if service == nil || service.host == nil {
		return "", ErrNoWorkspace
	}
	return service.host.WithLoreStore(expectedWorkspace, action)
}

func resolveImagePreset(cfg config.Config, requestedID string) (imagepreset.Preset, error) {
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

func dedupeImageItemIDs(ids []string) []string {
	result := make([]string, 0, len(ids))
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	return result
}

func emitImageProgress(emit func(agentrun.Event), itemID string, index, total int, status, message string, item *booklore.Item) {
	emit(agentrun.Event{Type: "lore_image_progress", Data: ImageProgressEvent{
		ItemID: itemID, Index: index, Total: total, Status: status, Message: message, Item: item,
	}})
}

func sameWorkspace(left, right string) bool {
	left = strings.TrimSpace(left)
	right = strings.TrimSpace(right)
	return left != "" && right != "" && filepath.Clean(left) == filepath.Clean(right)
}
