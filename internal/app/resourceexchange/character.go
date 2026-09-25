package resourceexchange

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"denova/internal/book/character"
	"denova/internal/book/lore"
	"denova/internal/revisionfile"
)

// Character conversion uses the existing adapter in an isolated workspace. The
// converted resources enter exactly the same frozen preview as native bundles.
func (s *Service) previewCharacter(ctx context.Context, source Source, data []byte) (Preview, error) {
	return s.PreviewCharacter(ctx, source, data, character.ImportOptions{ClassificationMode: lore.ClassificationModeHeuristic})
}

// PreviewCharacter freezes the result of optional semantic classification once.
// Applying or reloading the plan never invokes the classifier again.
func (s *Service) PreviewCharacter(ctx context.Context, source Source, data []byte, options character.ImportOptions) (Preview, error) {
	dir, err := os.MkdirTemp("", "denova-card-")
	if err != nil {
		return Preview{}, err
	}
	defer os.RemoveAll(dir)
	result, err := character.NewService(dir).ImportTavernCard(source.Filename, data, options)
	if err != nil {
		return Preview{}, err
	}
	items, err := lore.NewStore(dir).ListAll()
	if err != nil {
		return Preview{}, err
	}
	manifest := Manifest{Format: "denova.resource-pack", SchemaVersion: 1, Package: PackageInfo{ID: "character-card", Name: result.Name}}
	files := map[string][]byte{}
	for i, item := range items {
		// Provider/profile and local paths are not portable card data.
		item.Image, item.Provenance = nil, nil
		raw, err := portableJSON("lore.item", item)
		if err != nil {
			return Preview{}, err
		}
		id := fmt.Sprintf("lore-%d", i+1)
		name := "lore/" + id + ".json"
		files[name] = raw
		manifest.Resources = append(manifest.Resources, Resource{ID: id, Kind: "lore.item", Path: name})
	}
	if result.OpeningPresetCount > 0 {
		raw, err := os.ReadFile(filepath.Join(dir, "setting", "interactive-openings.json"))
		if err != nil {
			return Preview{}, err
		}
		var collection struct {
			Presets []json.RawMessage `json:"presets"`
		}
		if err := json.Unmarshal(raw, &collection); err != nil {
			return Preview{}, err
		}
		for i, raw := range collection.Presets {
			id := fmt.Sprintf("opening-%d", i+1)
			name := "openings/" + id + ".json"
			var openingData map[string]any
			if err := json.Unmarshal(raw, &openingData); err != nil {
				return Preview{}, err
			}
			portable, err := portableJSON("game.opening", openingData)
			if err != nil {
				return Preview{}, err
			}
			files[name] = portable
			manifest.Resources = append(manifest.Resources, Resource{ID: id, Kind: "game.opening", Path: name})
		}
	}
	if result.CoverPath != "" {
		raw, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(result.CoverPath)))
		if err != nil {
			return Preview{}, err
		}
		files["cover.png"] = raw
		files["cover.json"] = []byte(`{"asset_path":"cover.png"}`)
		manifest.Resources = append(manifest.Resources, Resource{ID: "cover", Kind: "project.cover", Path: "cover.json", Assets: []string{"cover.png"}})
	}
	raw, err := json.Marshal(manifest)
	if err != nil {
		return Preview{}, err
	}
	files["denova-pack.json"] = raw
	preview, err := s.previewFiles(ctx, Source{Kind: "file", Filename: filepath.Base(source.Filename)}, files)
	if err != nil {
		return Preview{}, err
	}
	result.Workspace, result.TargetPath, result.ProjectID, result.ItemIDs = "", "", "", nil
	preview.Character = &result
	directory, _ := s.previewPath(preview.ID)
	raw, err = json.Marshal(preview)
	if err != nil {
		return Preview{}, err
	}
	_, err = revisionfile.ReplaceIfRevision(ctx, filepath.Join(directory, "preview.json"), "", raw, revisionfile.Options{})
	return preview, err
}
