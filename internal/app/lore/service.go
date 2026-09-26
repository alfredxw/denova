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
		imageasset.DiscardUnlinkedLore(ctx, store, runtime.BookService, generated)
		return booklore.Item{}, err
	}
	updated, err := store.AppendImage(item.ID, &generated)
	if err != nil {
		imageasset.DiscardUnlinkedLore(ctx, store, runtime.BookService, generated)
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
	previousPaths := map[string]bool{}
	for _, material := range item.ResolvedMaterials {
		previousPaths[material.Path] = true
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
	result, err := service.images.GenerateProjectWithAgent(ctx, projectID, imageapp.AgentGenerateRequest{
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
	hasNewImage := false
	for _, material := range item.ResolvedMaterials {
		if strings.HasPrefix(material.MIMEType, "image/") && !previousPaths[material.Path] {
			hasNewImage = true
		}
	}
	if !hasNewImage {
		err := result.MissingImageError()
		slog.WarnContext(ctx, "[lore-image] Image Agent completed without a new lore image",
			"project_id", projectID, "item_id", item.ID, "command_id", request.CommandID, "error", err)
		return booklore.Item{}, err
	}
	return item, nil
}

func (service *Service) UploadItemMaterial(ctx context.Context, projectID, id, filename string, data []byte) (booklore.Item, error) {
	var item booklore.Item
	_, err := service.withStore(ctx, projectID, func(store *booklore.Store) error {
		var err error
		item, err = store.UploadMaterial(ctx, id, filename, data)
		return err
	})
	return item, err
}
func (service *Service) MaterialAssets(ctx context.Context, projectID string) ([]booklore.Asset, error) {
	var assets []booklore.Asset
	_, err := service.withStore(ctx, projectID, func(store *booklore.Store) error {
		var err error
		assets, err = store.Assets()
		return err
	})
	return assets, err
}
func (service *Service) MutateMaterial(ctx context.Context, projectID, id string, mutation booklore.MaterialMutation) (booklore.Item, error) {
	var item booklore.Item
	_, err := service.withStore(ctx, projectID, func(store *booklore.Store) error {
		var err error
		if mutation.Op == "remote" || mutation.Op == "localize" {
			item, err = store.RemoteMaterial(ctx, id, mutation)
		} else {
			item, err = store.MutateMaterial(id, mutation)
		}
		return err
	})
	if err != nil {
		slog.WarnContext(ctx, "[lore-material] mutation failed", "project_id", projectID, "item_id", id, "operation", mutation.Op, "error", err)
	}
	return item, err
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
