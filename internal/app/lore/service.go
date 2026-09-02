package loreapp

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"denova/config"
	imageapp "denova/internal/app/image"
	appsettings "denova/internal/app/settings"
	booklore "denova/internal/book/lore"
	imageasset "denova/internal/image/asset"
	imagepreset "denova/internal/image/preset"
)

func (service *Service) Items(ctx context.Context, projectID string) ([]booklore.Item, error) {
	var items []booklore.Item
	_, err := service.withStore(ctx, projectID, func(store *booklore.Store) error {
		var listErr error
		items, listErr = store.ListAll()
		return listErr
	})
	return items, err
}

func (service *Service) CreateItem(ctx context.Context, projectID string, input booklore.ItemInput) (booklore.Item, error) {
	var item booklore.Item
	_, err := service.withStore(ctx, projectID, func(store *booklore.Store) error {
		var createErr error
		item, createErr = store.Create(input)
		return createErr
	})
	return item, err
}

func (service *Service) UpdateItem(ctx context.Context, projectID, id string, input booklore.ItemInput) (booklore.Item, error) {
	var item booklore.Item
	_, err := service.withStore(ctx, projectID, func(store *booklore.Store) error {
		var updateErr error
		item, updateErr = store.Update(id, input)
		return updateErr
	})
	return item, err
}

func (service *Service) DeleteItem(ctx context.Context, projectID, id string) error {
	_, err := service.withStore(ctx, projectID, func(store *booklore.Store) error { return store.Delete(id) })
	return err
}

func (service *Service) ClearItemImage(ctx context.Context, projectID, id string) (booklore.Item, error) {
	var item booklore.Item
	_, err := service.withStore(ctx, projectID, func(store *booklore.Store) error {
		var updateErr error
		item, updateErr = store.SetImage(id, nil)
		return updateErr
	})
	return item, err
}

func (service *Service) GenerateItemImage(ctx context.Context, projectID, id string, request ItemImageGenerateRequest) (booklore.Item, error) {
	if strings.TrimSpace(request.Mode) == "agent" {
		return service.generateItemImageWithAgent(ctx, projectID, id, request)
	}
	if service == nil || service.images == nil {
		return booklore.Item{}, ErrNoWorkspace
	}
	runtime, err := service.images.AcquireProjectRuntime(ctx, projectID)
	if err != nil {
		return booklore.Item{}, err
	}
	defer runtime.Release()

	cfg := runtime.Config
	if layered, loadErr := config.LoadLayeredWithStartupConfigAt(
		cfg.DataDir(), runtime.Workspace, config.ProjectConfigPath(cfg.ProjectStoreDir),
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
	preset := imagepreset.Preset{}
	if strings.TrimSpace(request.Prompt) == "" {
		preset, err = resolveImagePreset(cfg, request.ImagePresetID)
		if err != nil {
			return booklore.Item{}, err
		}
	}
	generated, err := imageasset.NewService().GenerateLore(runtime.Context(), &cfg, runtime.BookService, imageasset.LoreGenerateRequest{
		Item:              item,
		Prompt:            request.Prompt,
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

func (service *Service) generateItemImageWithAgent(ctx context.Context, projectID, id string, request ItemImageGenerateRequest) (booklore.Item, error) {
	if service == nil || service.images == nil {
		return booklore.Item{}, ErrNoWorkspace
	}
	var item booklore.Item
	if _, err := service.withStore(ctx, projectID, func(store *booklore.Store) error {
		var readErr error
		item, readErr = store.ReadAny(id)
		return readErr
	}); err != nil {
		return booklore.Item{}, err
	}
	previousImagePath := ""
	if item.Image != nil {
		previousImagePath = item.Image.ImagePath
	}
	source, err := json.Marshal(struct {
		ItemID            string   `json:"item_id"`
		Type              string   `json:"type"`
		Name              string   `json:"name"`
		Tags              []string `json:"tags,omitempty"`
		BriefDescription  string   `json:"brief_description,omitempty"`
		Content           string   `json:"content,omitempty"`
		AdditionalRequest string   `json:"additional_user_requirements,omitempty"`
	}{
		ItemID: item.ID, Type: item.Type, Name: item.Name, Tags: item.Tags,
		BriefDescription: item.BriefDescription, Content: item.Content,
		AdditionalRequest: strings.TrimSpace(request.Instruction),
	})
	if err != nil {
		return booklore.Item{}, fmt.Errorf("encode lore image source context: %w", err)
	}
	_, err = service.images.GenerateProjectWithAgent(ctx, projectID, imageapp.AgentGenerateRequest{
		CommandID: request.CommandID, Purpose: "lore_item", LoreItemID: item.ID,
		SourceContext: string(source), ImagePresetID: request.ImagePresetID,
		SystemPrompt: "Generate exactly one recognizable visual reference for this lore item. Do not edit lore content and do not generate text, titles, watermarks, logos, UI panels, or QR codes.",
		AltText:      "Lore image: " + item.Name,
	})
	if err != nil {
		return booklore.Item{}, err
	}
	if _, err := service.withStore(ctx, projectID, func(store *booklore.Store) error {
		var readErr error
		item, readErr = store.ReadAny(id)
		return readErr
	}); err != nil {
		return booklore.Item{}, err
	}
	if item.Image == nil || strings.TrimSpace(item.Image.ImagePath) == "" || item.Image.ImagePath == previousImagePath {
		return booklore.Item{}, fmt.Errorf("image Agent did not generate a new lore image")
	}
	return item, nil
}

func (service *Service) UploadItemImage(ctx context.Context, projectID, id, filename string, data []byte) (booklore.Item, error) {
	if service == nil || service.images == nil {
		return booklore.Item{}, ErrNoWorkspace
	}
	runtime, err := service.images.AcquireProjectRuntime(ctx, projectID)
	if err != nil {
		return booklore.Item{}, err
	}
	defer runtime.Release()

	store := booklore.NewStore(runtime.Workspace)
	item, err := store.ReadAny(id)
	if err != nil {
		return booklore.Item{}, err
	}
	uploaded, err := imageasset.NewService().UploadLore(runtime.Context(), runtime.BookService, imageasset.LoreUploadRequest{
		Item:     item,
		Filename: filename,
		Data:     data,
	})
	if err != nil {
		return booklore.Item{}, err
	}
	if err := runtime.Context().Err(); err != nil {
		return booklore.Item{}, err
	}
	updated, err := store.SetImage(item.ID, &uploaded)
	if err != nil {
		return booklore.Item{}, err
	}
	slog.InfoContext(ctx, fmt.Sprintf("[lore-image] uploaded item_id=%s filename=%q path=%s", updated.ID, filename, uploaded.ImagePath))
	return updated, nil
}

func (service *Service) withStore(ctx context.Context, projectID string, action func(*booklore.Store) error) (string, error) {
	if service == nil || service.host == nil {
		return "", ErrNoWorkspace
	}
	return service.host.WithLoreStore(ctx, projectID, action)
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
