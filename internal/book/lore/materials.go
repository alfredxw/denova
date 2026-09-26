package lore

import (
	"fmt"
	"path"
	"strings"
	"time"

	"denova/internal/portablepath"
	"github.com/google/uuid"
)

// Materials distinguishes untouched legacy data (nil) from an explicitly empty collection.
type Materials struct {
	Entries      []MaterialEntry `json:"entries"`
	CoverAssetID string          `json:"cover_asset_id,omitempty"`
}
type MaterialEntry struct {
	AssetID     string `json:"asset_id"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
}
type AssetSource struct {
	Kind     string `json:"kind"`
	MetaPath string `json:"meta_path,omitempty"`
}
type Asset struct {
	ID           string      `json:"id"`
	Path         string      `json:"path"`
	OriginalName string      `json:"original_name"`
	MIMEType     string      `json:"mime_type"`
	SizeBytes    int         `json:"size_bytes"`
	CreatedAt    string      `json:"created_at,omitempty"`
	Source       AssetSource `json:"source"`
}

// Material is the resolved read view shared by UI, tools and resource exchange.
type Material struct {
	Asset
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// MaterialMutation changes only the requested association or cover, under the
// same collection lock as text edits. It never accepts a stale item snapshot.
type MaterialMutation struct {
	Op          string `json:"op"`
	AssetID     string `json:"asset_id,omitempty"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
}

func assetFromImage(image *Image) Asset {
	mime := image.MIMEType
	if mime == "" {
		mime = "image/png"
		if ext := strings.ToLower(path.Ext(image.ImagePath)); ext == ".jpg" || ext == ".jpeg" {
			mime = "image/jpeg"
		}
	}
	kind := "generated"
	if image.Provider == "user_upload" {
		kind = "upload"
	}
	id := ""
	if strings.HasPrefix(image.ImagePath, "assets/lore/media/asset_") {
		id = path.Base(path.Dir(image.ImagePath))
	}
	return Asset{ID: id, Path: image.ImagePath, OriginalName: path.Base(image.ImagePath), MIMEType: mime, SizeBytes: image.SizeBytes, CreatedAt: image.CreatedAt, Source: AssetSource{Kind: kind, MetaPath: image.MetaPath}}
}
func legacyMaterial(item Item) Material {
	asset := assetFromImage(item.Image)
	// This locator is never a persisted AssetID; the path detects stale selections.
	asset.ID = "legacy:" + item.ID + ":" + asset.Path
	return Material{Asset: asset, Name: asset.OriginalName, Description: item.Image.AltText}
}
func resolveItem(item Item, assets []Asset) Item {
	item.ResolvedMaterials = make([]Material, 0)
	if item.Materials == nil {
		if item.Image != nil {
			item.ResolvedMaterials = append(item.ResolvedMaterials, legacyMaterial(item))
		}
		return item
	}
	item.Image = nil
	byID := make(map[string]Asset, len(assets))
	for _, a := range assets {
		byID[a.ID] = a
	}
	for _, entry := range item.Materials.Entries {
		asset := byID[entry.AssetID]
		name := entry.Name
		if name == "" {
			name = asset.OriginalName
		}
		item.ResolvedMaterials = append(item.ResolvedMaterials, Material{Asset: asset, Name: name, Description: entry.Description})
		if entry.AssetID == item.Materials.CoverAssetID {
			item.Image = &Image{ImagePath: asset.Path, MetaPath: asset.Source.MetaPath, AltText: firstNonEmptyLoreValue(entry.Description, name), MIMEType: asset.MIMEType, SizeBytes: asset.SizeBytes, CreatedAt: asset.CreatedAt}
		}
	}
	return item
}
func validateMaterials(c Collection) error {
	byID := map[string]Asset{}
	paths := map[string]bool{}
	for _, a := range c.Assets {
		if a.ID == "" || strings.HasPrefix(a.ID, "legacy:") || byID[a.ID].ID != "" {
			return fmt.Errorf("invalid or duplicate lore asset ID: %q", a.ID)
		}
		if err := portablepath.Validate(a.Path); err != nil {
			return err
		}
		if !strings.HasPrefix(a.Path, "assets/") || paths[portablepath.FoldKey(a.Path)] {
			return fmt.Errorf("invalid or duplicate lore asset path: %s", a.Path)
		}
		if a.Source.MetaPath != "" {
			if err := portablepath.Validate(a.Source.MetaPath); err != nil {
				return err
			}
			if !strings.HasPrefix(a.Source.MetaPath, "assets/") {
				return fmt.Errorf("invalid asset source path")
			}
		}
		if !strings.HasPrefix(a.MIMEType, "image/") && !strings.HasPrefix(a.MIMEType, "audio/") {
			return fmt.Errorf("unsupported lore media type: %s", a.MIMEType)
		}
		if a.SizeBytes < 0 {
			return fmt.Errorf("negative lore asset size")
		}
		byID[a.ID] = a
		paths[portablepath.FoldKey(a.Path)] = true
	}
	for _, item := range c.Items {
		if item.Materials == nil {
			continue
		}
		if item.Materials.Entries == nil {
			return fmt.Errorf("lore %s materials.entries must be an array", item.ID)
		}
		seen := map[string]bool{}
		for _, entry := range item.Materials.Entries {
			if byID[entry.AssetID].ID == "" || seen[entry.AssetID] {
				return fmt.Errorf("lore %s has a missing or duplicate asset: %s", item.ID, entry.AssetID)
			}
			seen[entry.AssetID] = true
		}
		cover := item.Materials.CoverAssetID
		if cover != "" && (!seen[cover] || !strings.HasPrefix(byID[cover].MIMEType, "image/")) {
			return fmt.Errorf("lore %s cover must reference a linked image", item.ID)
		}
	}
	return nil
}
func registerAsset(c *Collection, a Asset) Asset {
	for _, existing := range c.Assets {
		if existing.Path == a.Path {
			return existing
		}
	}
	if a.ID == "" || strings.HasPrefix(a.ID, "legacy:") {
		a.ID = "asset_" + uuid.NewString()
	}
	c.Assets = append(c.Assets, a)
	return a
}
func promoteMaterials(c *Collection, item *Item) (oldID, newID string) {
	if item.Materials != nil {
		return "", ""
	}
	item.Materials = &Materials{Entries: []MaterialEntry{}}
	if item.Image == nil {
		return "", ""
	}
	legacy := legacyMaterial(*item)
	a := registerAsset(c, legacy.Asset)
	item.Materials.Entries = append(item.Materials.Entries, MaterialEntry{AssetID: a.ID, Name: legacy.Name, Description: legacy.Description})
	item.Materials.CoverAssetID = a.ID
	return legacy.ID, a.ID
}
func (s *Store) mutateMaterials(id string, change func(*Collection, *Item) error) (Item, error) {
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()
	c, err := s.loadOrCreate()
	if err != nil {
		return Item{}, err
	}
	index := loreItemIndex(c.Items, id)
	if index < 0 {
		return Item{}, fmt.Errorf("lore item not found: %s", id)
	}
	item := &c.Items[index]
	if err := change(&c, item); err != nil {
		return Item{}, err
	}
	item.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if err := s.save(c); err != nil {
		return Item{}, err
	}
	return resolveItem(*item, c.Assets), nil
}

// AttachAsset registers a ready file and its association atomically. Callers own
// file creation and must remove only their uncommitted files on failure.
func (s *Store) AttachAsset(id string, asset Asset, entry MaterialEntry) (Item, error) {
	return s.mutateMaterials(id, func(c *Collection, item *Item) error {
		promoteMaterials(c, item)
		asset = registerAsset(c, asset)
		for _, e := range item.Materials.Entries {
			if e.AssetID == asset.ID {
				return nil
			}
		}
		entry.AssetID = asset.ID
		item.Materials.Entries = append(item.Materials.Entries, entry)
		return nil
	})
}
func (s *Store) AppendImage(id string, image *Image) (Item, error) {
	if image == nil {
		return Item{}, fmt.Errorf("image is required")
	}
	return s.AttachAsset(id, assetFromImage(image), MaterialEntry{Name: image.AltText})
}
func (s *Store) MutateMaterial(id string, m MaterialMutation) (Item, error) {
	return s.mutateMaterials(id, func(c *Collection, item *Item) error {
		// Resolve a legacy source before promoting the target (which may be itself).
		var source *Asset
		if m.Op == "link" {
			for _, a := range c.Assets {
				if a.ID == m.AssetID {
					copy := a
					source = &copy
					break
				}
			}
			if source == nil {
				for _, other := range c.Items {
					if other.Materials == nil && other.Image != nil {
						v := legacyMaterial(other)
						if v.ID == m.AssetID {
							source = &v.Asset
							break
						}
					}
				}
			}
			if source == nil {
				return fmt.Errorf("asset not found: %s", m.AssetID)
			}
		}
		oldID, newID := promoteMaterials(c, item)
		if oldID != "" && m.AssetID == oldID {
			m.AssetID = newID
		}
		entries := item.Materials.Entries
		index := -1
		for i, e := range entries {
			if e.AssetID == m.AssetID {
				index = i
				break
			}
		}
		switch m.Op {
		case "link":
			a := registerAsset(c, *source)
			for _, e := range entries {
				if e.AssetID == a.ID {
					return nil
				}
			}
			item.Materials.Entries = append(entries, MaterialEntry{AssetID: a.ID, Name: strings.TrimSpace(m.Name), Description: strings.TrimSpace(m.Description)})
		case "update":
			if index < 0 {
				return fmt.Errorf("material not linked: %s", m.AssetID)
			}
			entries[index].Name = strings.TrimSpace(m.Name)
			entries[index].Description = strings.TrimSpace(m.Description)
		case "remove":
			if index < 0 {
				return fmt.Errorf("material not linked: %s", m.AssetID)
			}
			item.Materials.Entries = append(entries[:index], entries[index+1:]...)
			if item.Materials.CoverAssetID == m.AssetID {
				item.Materials.CoverAssetID = ""
			}
		case "cover":
			if m.AssetID != "" && index < 0 {
				return fmt.Errorf("cover is not linked: %s", m.AssetID)
			}
			item.Materials.CoverAssetID = m.AssetID
		default:
			return fmt.Errorf("unknown material operation: %s", m.Op)
		}
		return nil
	})
}

// Assets includes unlinked files and read-only legacy projections for reuse.
func (s *Store) Assets() ([]Asset, error) {
	c, err := s.loadOrCreate()
	if err != nil {
		return nil, err
	}
	out := append([]Asset{}, c.Assets...)
	seen := map[string]bool{}
	for _, a := range out {
		seen[a.Path] = true
	}
	for _, item := range c.Items {
		if item.Materials == nil && item.Image != nil {
			a := legacyMaterial(item).Asset
			if !seen[a.Path] {
				out = append(out, a)
				seen[a.Path] = true
			}
		}
	}
	return out, nil
}

// MaterialFilePaths resolves version dependencies without rewriting legacy data.
func MaterialFilePaths(data []byte) ([]string, error) {
	c, err := decodeLoreCollectionJSON(data)
	if err != nil {
		return nil, err
	}
	paths := []string{}
	seen := map[string]bool{}
	add := func(a Asset) {
		for _, name := range []string{a.Path, a.Source.MetaPath} {
			if name != "" && !seen[name] {
				paths = append(paths, name)
				seen[name] = true
			}
		}
	}
	for _, a := range c.Assets {
		add(a)
	}
	for _, item := range c.Items {
		if item.Materials == nil && item.Image != nil {
			add(assetFromImage(item.Image))
		}
	}
	return paths, nil
}

// IsManagedMaterialPath identifies immutable media owned by the lore library,
// including the released single-image directory. Restores retain newer files.
func IsManagedMaterialPath(name string) bool {
	return strings.HasPrefix(name, "assets/lore/media/") || strings.HasPrefix(name, "assets/lore/images/")
}
