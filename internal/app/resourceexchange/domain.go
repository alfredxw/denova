package resourceexchange

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"path"
	"path/filepath"
	"strings"

	"denova/internal/app/resourcecatalog"
	"denova/internal/book"
	"denova/internal/book/lore"
	imageasset "denova/internal/image/asset"
	imagepreset "denova/internal/image/preset"
	"denova/internal/interactive"
	"denova/internal/interactive/teller"
	"denova/internal/style"
)

const openingPath = "setting/interactive-openings.json"
const coverPath = "assets/image/cover.png"

type opening struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Content string `json:"content"`
}
type openings struct {
	Version int       `json:"version"`
	Presets []opening `json:"presets"`
}

// stageDefinition delegates validation and normalization to the existing domain
// libraries. Only their created document is returned; builtin initialization in
// the scratch directory never becomes part of the installation.
func stageDefinition(resource PreviewResource, id string, raw []byte, refs map[string]string) (string, []byte, error) {
	dir, err := os.MkdirTemp("", "denova-definition-")
	if err != nil {
		return "", nil, err
	}
	defer os.RemoveAll(dir)
	catalog := resourcecatalog.NewService(dir, nil)
	var file string
	switch resource.Kind {
	case "preset.narrative":
		var value teller.Definition
		if err = json.Unmarshal(raw, &value); err != nil {
			break
		}
		value.ID = id
		rewrite := func(values []string) error {
			for i, ref := range values {
				mapped, ok := refs["style.reference:"+ref]
				if !ok {
					return fmt.Errorf("unresolved style reference %s", ref)
				}
				values[i] = style.StoragePath(mapped)
			}
			return nil
		}
		if err = rewrite(value.StyleRefs); err != nil {
			break
		}
		for i := range value.StyleRules {
			if err = rewrite(value.StyleRules[i].StyleRefs); err != nil {
				break
			}
		}
		if err != nil {
			break
		}
		var created teller.Definition
		created, err = catalog.CreateTeller(value)
		file = created.Path
	case "preset.image":
		var value imagepreset.Preset
		if err = json.Unmarshal(raw, &value); err != nil {
			break
		}
		value.ID = id
		var created imagepreset.Preset
		created, err = catalog.CreateImagePreset(value)
		file = created.Path
	case "preset.game_planning":
		var value interactive.GamePlanningTemplate
		if err = json.Unmarshal(raw, &value); err != nil {
			break
		}
		value.ID = id
		var created interactive.GamePlanningTemplate
		created, err = catalog.CreateGamePlanningTemplate(value)
		file = created.Path
	case "preset.events":
		var value interactive.EventPackageModule
		if err = json.Unmarshal(raw, &value); err != nil {
			break
		}
		value.ID = id
		var created interactive.EventPackageModule
		created, err = catalog.CreateEventPackage(value)
		file = created.Path
	case "preset.rules":
		var value interactive.RuleSystemModule
		if err = json.Unmarshal(raw, &value); err != nil {
			break
		}
		value.ID = id
		var reference struct {
			ActorStateRef string `json:"actor_state_ref"`
		}
		if err = json.Unmarshal(raw, &reference); err != nil {
			break
		}
		if reference.ActorStateRef != "" {
			value.ActorStateID = reference.ActorStateRef
		}
		if value.ActorStateID != "" {
			mapped, ok := refs["preset.actor_state:"+value.ActorStateID]
			if !ok {
				return "", nil, fmt.Errorf("unresolved actor state %s", value.ActorStateID)
			}
			value.ActorStateID = mapped
		}
		var created interactive.RuleSystemModule
		created, err = catalog.CreateRuleSystem(value)
		file = created.Path
	case "preset.actor_state":
		var value interactive.ActorStateModule
		if err = json.Unmarshal(raw, &value); err != nil {
			break
		}
		value.ID = id
		var created interactive.ActorStateModule
		created, err = catalog.CreateActorState(value)
		file = created.Path
	case "style.reference":
		var created style.Reference
		created, err = style.NewLibrary(dir).Create(style.WriteRequest{Name: strings.TrimSuffix(resource.Name, path.Ext(resource.Name)), Filename: id, Content: string(raw)})
		file = created.Path
	default:
		return "", nil, fmt.Errorf("unsupported definition kind %s", resource.Kind)
	}
	if err != nil {
		return "", nil, err
	}
	if !filepath.IsAbs(file) {
		file = filepath.Join(dir, file)
	}
	content, err := os.ReadFile(file)
	if err != nil {
		return "", nil, err
	}
	relative, err := filepath.Rel(dir, file)
	return filepath.ToSlash(relative), content, err
}

func (s *Service) stageProject(ctx context.Context, previewDir string, extra *[]FileTarget, resource PreviewResource, local LocalRef, raw []byte, staged map[FileTarget][]byte, expected map[FileTarget]string) (FileTarget, error) {
	target := FileTarget{ProjectID: local.ProjectID}
	switch resource.Kind {
	case "lore.item":
		target.Path = lore.ItemsRelativePath
	case "game.opening":
		target.Path = openingPath
	case "project.cover":
		target.Path = coverPath
	default:
		return target, fmt.Errorf("invalid Project resource")
	}
	if _, ok := staged[target]; !ok {
		snapshot, err := s.snapshot(ctx, target)
		if err != nil {
			return target, err
		}
		staged[target], expected[target] = snapshot.Content, snapshot.Revision
	}
	switch resource.Kind {
	case "project.cover":
		var payload portableImage
		if err := decode(raw, &payload); err != nil {
			return target, err
		}
		raw, err := resourceAsset(previewDir, resource, payload.AssetPath)
		if err != nil {
			return target, err
		}
		cfg, format, err := image.DecodeConfig(bytes.NewReader(raw))
		if err != nil || cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > 16384 || cfg.Height > 16384 || format != "png" {
			return target, fmt.Errorf("Project cover must be a bounded PNG image")
		}
		staged[target] = raw
	case "game.opening":
		var value opening
		if err := decode(raw, &value); err != nil {
			return target, err
		}
		value.ID = local.ID
		if strings.TrimSpace(value.Title) == "" || strings.TrimSpace(value.Content) == "" || len(value.Content) > 64*1024 {
			return target, fmt.Errorf("invalid game opening")
		}
		next := openings{Version: 1, Presets: []opening{}}
		if len(staged[target]) > 0 {
			if err := json.Unmarshal(staged[target], &next); err != nil {
				return target, err
			}
		}
		found := false
		for i := range next.Presets {
			if next.Presets[i].ID == value.ID {
				next.Presets[i] = value
				found = true
				break
			}
		}
		if !found {
			next.Presets = append(next.Presets, value)
		}
		content, err := json.MarshalIndent(next, "", "  ")
		if err != nil {
			return target, err
		}
		staged[target] = content
	case "lore.item":
		var input lore.ItemInput
		if err := json.Unmarshal(raw, &input); err != nil {
			return target, err
		}
		input.ID = local.ID
		input.Provenance = nil
		var payload struct {
			Image *portableImage `json:"image"`
		}
		if err := json.Unmarshal(raw, &payload); err != nil {
			return target, err
		}
		input.Image = nil
		dir, err := os.MkdirTemp("", "denova-lore-")
		if err != nil {
			return target, err
		}
		defer os.RemoveAll(dir)
		if len(staged[target]) > 0 {
			if err := writeFiles(dir, map[string][]byte{target.Path: staged[target]}); err != nil {
				return target, err
			}
		}
		store := lore.NewStore(dir)
		items, err := store.ListAll()
		if err != nil {
			return target, err
		}
		exists := false
		for _, item := range items {
			if item.ID == input.ID {
				exists = true
				break
			}
		}
		if exists {
			_, err = store.Update(input.ID, input)
		} else {
			_, err = store.Create(input)
		}
		if err != nil {
			return target, err
		}
		if payload.Image != nil {
			data, err := resourceAsset(previewDir, resource, payload.Image.AssetPath)
			if err != nil {
				return target, err
			}
			item, err := store.ReadAny(input.ID)
			if err != nil {
				return target, err
			}
			imported, err := imageasset.NewService().UploadLore(ctx, book.NewService(dir), imageasset.LoreUploadRequest{Item: item, Filename: path.Base(payload.Image.AssetPath), Data: data})
			if err != nil {
				return target, err
			}
			imported.AltText = payload.Image.AltText
			if _, err := store.SetImage(item.ID, &imported); err != nil {
				return target, err
			}
			for _, name := range []string{imported.ImagePath, imported.MetaPath} {
				content, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(name)))
				if err != nil {
					return target, err
				}
				assetTarget := FileTarget{ProjectID: local.ProjectID, Path: name}
				staged[assetTarget] = content
				*extra = append(*extra, assetTarget)
			}
		}
		staged[target], err = os.ReadFile(lore.ItemsPath(dir))
		if err != nil {
			return target, err
		}
	}
	return target, nil
}
