package app

import (
	"context"
	"fmt"
	"strings"

	appsettings "denova/internal/app/settings"
	booklore "denova/internal/book/lore"
	imagegen "denova/internal/image/generation"
	"denova/internal/platform"
	"denova/internal/portablepath"
)

// platformResourceHost exposes existing library and image services through
// stable Project leases. No foreground workspace or extension-supplied path
// can select another Project, and model credentials stay inside the host.
type platformResourceHost struct{ app *App }

func (host platformResourceHost) LibraryItems(ctx context.Context, projectID string) ([]platform.LibraryItem, error) {
	result := []platform.LibraryItem{}
	_, err := (loreHost{app: host.app}).WithLoreStore(ctx, projectID, func(store *booklore.Store) error {
		items, err := store.ListAll()
		if err != nil {
			return err
		}
		for _, item := range items {
			entry := platform.LibraryItem{
				ID: item.ID, Type: item.Type, Name: item.Name, Enabled: item.Enabled,
				Tags: item.Tags, BriefDescription: item.BriefDescription, Keywords: item.Keywords,
				Content: item.Content, UpdatedAt: item.UpdatedAt,
			}
			if item.Image != nil && strings.HasPrefix(item.Image.ImagePath, "assets/") && portablepath.Validate(item.Image.ImagePath) == nil {
				entry.Image = &platform.AssetRef{Kind: "project", Path: item.Image.ImagePath}
			}
			result = append(result, entry)
		}
		return ctx.Err()
	})
	return result, err
}

func (host platformResourceHost) ReadAsset(ctx context.Context, projectID, name string) ([]byte, error) {
	if !strings.HasPrefix(name, "assets/") || portablepath.Validate(name) != nil {
		return nil, fmt.Errorf("asset path must be a portable path inside assets/")
	}
	operation, err := host.app.AcquireProjectOperation(ctx, projectID)
	if err != nil {
		return nil, err
	}
	defer operation.Release()
	if err := operation.Context().Err(); err != nil {
		return nil, err
	}
	return platform.ReadRasterAsset(operation.Layout().ContentRoot, name)
}

func (host platformResourceHost) GenerateImage(ctx context.Context, projectID, profileID string, request platform.ImageRequest) (platform.ImageBytes, error) {
	operation, err := host.app.AcquireProjectOperation(ctx, projectID)
	if err != nil {
		return platform.ImageBytes{}, err
	}
	defer operation.Release()
	layout := operation.Layout()
	cfg, err := appsettings.RefreshProject((imageHost{app: host.app}).ImageConfigSnapshot(), layout.ContentRoot, layout.StoreRoot)
	if err != nil {
		return platform.ImageBytes{}, fmt.Errorf("resolve extension image Project settings: %w", err)
	}
	cfg.ProjectID = projectID
	// The shared provider service handles image profile resolution, credentials,
	// provider defaults and downloads. Assets belong to the extension, so this
	// operation does not require a Book Project or mutate library bindings.
	result, err := imagegen.NewService().Generate(operation.Context(), &cfg, imagegen.GenerateRequest{
		ProfileID: profileID, Prompt: request.Prompt, N: 1, Size: request.Size,
		AspectRatio: request.AspectRatio, Quality: request.Quality,
	})
	if err != nil {
		return platform.ImageBytes{}, err
	}
	if err := operation.Context().Err(); err != nil {
		return platform.ImageBytes{}, err
	}
	if len(result.Images) == 0 {
		return platform.ImageBytes{}, fmt.Errorf("image provider returned no images")
	}
	return platform.ImageBytes{Data: result.Images[0].Data, RevisedPrompt: result.Images[0].RevisedPrompt}, nil
}
