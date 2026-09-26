package resourceexchange

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"

	"denova/internal/book/lore"
	"github.com/google/uuid"
)

// Resource packages keep association text and file references; runtime provider
// configuration and host paths are deliberately not part of the portable payload.
type portableMaterials struct {
	Entries   []portableMaterial `json:"entries"`
	CoverPath string             `json:"cover_asset_path,omitempty"`
}
type portableMaterial struct {
	AssetPath    string `json:"asset_path"`
	OriginalName string `json:"original_name,omitempty"`
	Name         string `json:"name,omitempty"`
	Description  string `json:"description,omitempty"`
}

func (s *Service) exportLoreMaterials(ctx context.Context, ref LocalRef, item lore.Item, raw []byte) (map[string][]byte, error) {
	files := map[string][]byte{}
	payload := portableMaterials{Entries: []portableMaterial{}}
	for _, material := range item.ResolvedMaterials {
		snapshot, err := s.snapshot(ctx, FileTarget{ProjectID: ref.ProjectID, Path: material.Path})
		if err != nil {
			return nil, err
		}
		if !snapshot.Exists {
			return nil, fmt.Errorf("lore material missing: %s", material.Path)
		}
		name := "media/" + uuid.NewSHA1(uuid.NameSpaceURL, []byte(ref.ProjectID+":"+material.Path)).String() + path.Ext(material.Path)
		files[name] = snapshot.Content
		payload.Entries = append(payload.Entries, portableMaterial{AssetPath: name, OriginalName: material.OriginalName, Name: material.Name, Description: material.Description})
		if item.Image != nil && item.Image.ImagePath == material.Path {
			payload.CoverPath = name
		}
	}
	var body map[string]json.RawMessage
	if err := json.Unmarshal(raw, &body); err != nil {
		return nil, err
	}
	delete(body, "image")
	body["materials"], _ = json.Marshal(payload)
	encoded, err := json.MarshalIndent(body, "", "  ")
	if err != nil {
		return nil, err
	}
	files["resource.json"] = encoded
	return files, nil
}
func importLoreMaterials(ctx context.Context, dir, previewDir string, resource PreviewResource, local LocalRef, raw []byte, staged map[FileTarget][]byte, extra *[]FileTarget, importedAssets map[FileTarget]lore.Asset) error {
	var payload struct {
		Image     *portableImage     `json:"image"`
		Materials *portableMaterials `json:"materials"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return err
	}
	materials := payload.Materials
	if materials == nil && payload.Image != nil {
		materials = &portableMaterials{Entries: []portableMaterial{{AssetPath: payload.Image.AssetPath, Description: payload.Image.AltText}}, CoverPath: payload.Image.AssetPath}
	}
	if materials == nil {
		return nil
	}
	if materials.Entries == nil {
		return fmt.Errorf("material entries must be an array")
	}
	store := lore.NewStore(dir)
	item, err := store.ReadAny(local.ID)
	if err != nil {
		return err
	}
	// The temporary store is isolated; the outer exchange transaction commits the
	// final collection and every staged file together.
	for _, m := range item.ResolvedMaterials {
		if _, err = store.MutateMaterial(local.ID, lore.MaterialMutation{Op: "remove", AssetID: m.ID}); err != nil {
			return err
		}
	}
	if _, err = store.MutateMaterial(local.ID, lore.MaterialMutation{Op: "cover"}); err != nil {
		return err
	}
	seen := map[string]bool{}
	coverFound := materials.CoverPath == ""
	for _, m := range materials.Entries {
		if seen[m.AssetPath] {
			return fmt.Errorf("duplicate material path")
		}
		seen[m.AssetPath] = true
		data, err := resourceAsset(previewDir, resource, m.AssetPath)
		if err != nil {
			return err
		}
		// Reuse the same package file across items without conflating distinct
		// assets that happen to have identical bytes or different descriptions.
		key := FileTarget{ProjectID: local.ProjectID, Path: path.Join(resource.Root, m.AssetPath)}
		asset := importedAssets[key]
		if asset.ID == "" {
			filename := m.OriginalName
			if filename == "" {
				filename = path.Base(m.AssetPath)
			}
			uploaded, err := store.UploadMaterial(ctx, local.ID, filename, data)
			if err != nil {
				return err
			}
			asset = uploaded.ResolvedMaterials[len(uploaded.ResolvedMaterials)-1].Asset
			content, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(asset.Path)))
			if err != nil {
				return err
			}
			target := FileTarget{ProjectID: local.ProjectID, Path: asset.Path}
			staged[target] = content
			*extra = append(*extra, target)
		} else if _, err = store.AttachAsset(local.ID, asset, lore.MaterialEntry{}); err != nil {
			return err
		}
		importedAssets[key] = asset
		if _, err = store.MutateMaterial(local.ID, lore.MaterialMutation{Op: "update", AssetID: asset.ID, Name: m.Name, Description: m.Description}); err != nil {
			return err
		}
		if m.AssetPath == materials.CoverPath {
			coverFound = true
			if _, err = store.MutateMaterial(local.ID, lore.MaterialMutation{Op: "cover", AssetID: asset.ID}); err != nil {
				return err
			}
		}
	}
	if !coverFound {
		return fmt.Errorf("cover references an unlisted material")
	}
	return nil
}
