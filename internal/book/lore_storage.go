package book

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"denova/internal/workspacepath"
)

const (
	// LoreItemsRelativePath is the canonical user-visible Lore collection.
	// Individual entries remain records in this file instead of becoming a
	// high-cardinality directory of tiny Markdown files.
	LoreItemsRelativePath = "setting/lore/items.json"
)

// LoreItemsPath returns the canonical public storage path for a book.
func LoreItemsPath(workspace string) string {
	return filepath.Join(workspace, filepath.FromSlash(LoreItemsRelativePath))
}

func decodeLoreCollectionJSON(data []byte) (LoreCollection, error) {
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(data, &envelope); err != nil {
		return LoreCollection{}, fmt.Errorf("Lore JSON 无效 / invalid Lore JSON: %w", err)
	}
	versionRaw, hasVersion := envelope["version"]
	itemsRaw, hasItems := envelope["items"]
	if !hasVersion || !hasItems {
		return LoreCollection{}, errors.New("Lore JSON 必须包含 version 和 items / must contain version and items")
	}
	var version int
	if err := json.Unmarshal(versionRaw, &version); err != nil {
		return LoreCollection{}, errors.New("Lore version 必须是整数 / must be an integer")
	}
	if version < 1 || version > loreItemsVersion {
		return LoreCollection{}, fmt.Errorf("不支持 Lore version %d / unsupported Lore version %d", version, version)
	}
	if string(itemsRaw) == "null" {
		return LoreCollection{}, errors.New("Lore items 必须是数组 / must be an array")
	}
	var items []LoreItem
	if err := json.Unmarshal(itemsRaw, &items); err != nil {
		return LoreCollection{}, fmt.Errorf("Lore items 必须是数组 / must be an array: %w", err)
	}
	if err := validateLoreItemIdentities(items); err != nil {
		return LoreCollection{}, err
	}
	return LoreCollection{Version: loreItemsVersion, Items: normalizeLoreItems(items)}, nil
}

func validateLoreItemIdentities(items []LoreItem) error {
	ids := make(map[string]bool, len(items))
	names := make(map[string]bool, len(items))
	for index, item := range items {
		id := normalizeLoreID(item.ID)
		if id == "" {
			return fmt.Errorf("Lore items[%d].id 不能为空 / cannot be empty", index)
		}
		if ids[id] {
			return fmt.Errorf("Lore id 重复 / duplicate Lore id: %s", id)
		}
		ids[id] = true

		name := strings.TrimSpace(item.Name)
		if name == "" {
			return fmt.Errorf("Lore items[%d].name 不能为空 / cannot be empty", index)
		}
		if err := validateLoreReferenceName(name); err != nil {
			return fmt.Errorf("Lore items[%d].name 无效 / is invalid: %w", index, err)
		}
		nameKey := loreNameKey(name)
		if names[nameKey] {
			return fmt.Errorf("Lore name 重复 / duplicate Lore name: %s", name)
		}
		names[nameKey] = true
	}
	return nil
}

func (s *LoreStore) readableItemsPath() (string, bool) {
	canonical := LoreItemsPath(s.workspace)
	if _, err := os.Stat(canonical); err == nil || !os.IsNotExist(err) {
		return canonical, false
	}

	activeLegacy := workspacepath.Path(s.workspace, "lore", "items.json")
	candidates := []string{
		activeLegacy,
		filepath.Join(s.workspace, workspacepath.DataDirName, "lore", "items.json"),
		filepath.Join(s.workspace, workspacepath.LegacyDataDirName, "lore", "items.json"),
	}
	seen := map[string]bool{}
	for _, candidate := range candidates {
		candidate = filepath.Clean(candidate)
		if seen[candidate] {
			continue
		}
		seen[candidate] = true
		if _, err := os.Stat(candidate); err == nil || !os.IsNotExist(err) {
			return candidate, true
		}
	}
	return canonical, false
}
