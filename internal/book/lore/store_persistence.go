package lore

import (
	"context"
	"denova/internal/localfs"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
)

func (s *Store) Ensure() error {
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()

	sourcePath, legacy := s.readableItemsPath()
	collection, err := s.loadOrCreate()
	if err != nil {
		return err
	}
	if _, err := os.Stat(s.itemsPath()); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := s.save(collection); err != nil {
		return err
	}
	if legacy {
		slog.InfoContext(context.Background(), fmt.Sprintf("[lore-store] migrated legacy Lore collection source=%s target=%s", sourcePath, s.itemsPath()))
	}
	return nil
}

func (s *Store) loadOrCreate() (Collection, error) {
	path, _ := s.readableItemsPath()
	data, err := os.ReadFile(path)
	if err == nil {
		// Version 1 remains readable regardless of where a user copied it; every
		// subsequent typed save upgrades the same collection to version 2.
		collection, decodeErr := decodeLoreCollectionJSON(data)
		if decodeErr != nil {
			return Collection{}, fmt.Errorf("解析 Lore items 失败 path=%s: %w", path, decodeErr)
		}
		return collection, nil
	}
	if !os.IsNotExist(err) {
		return Collection{}, err
	}
	return Collection{Version: loreItemsVersion, Items: []Item{}}, nil
}

func (s *Store) save(collection Collection) error {
	collection.Version = loreItemsVersion
	normalized := make([]Item, 0, len(collection.Items))
	for _, item := range collection.Items {
		normalized = append(normalized, normalizeLoreItem(item))
	}
	collection.Items = normalized
	if err := validateLoreItemIdentities(collection.Items); err != nil {
		return fmt.Errorf("拒绝保存无效 Lore collection: %w", err)
	}
	path := s.itemsPath()
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(collection, "", "  ")
	if err != nil {
		return err
	}
	temp, err := os.CreateTemp(dir, ".items-*.tmp")
	if err != nil {
		return fmt.Errorf("创建资料库临时文件失败 path=%s: %w", path, err)
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	closeTemp := func() {
		if closeErr := temp.Close(); closeErr != nil {
			slog.ErrorContext(context.Background(), fmt.Sprintf("[lore-store] close temp file failed path=%s err=%v", tempPath, closeErr))
		}
	}
	if err := temp.Chmod(0o644); err != nil {
		closeTemp()
		return fmt.Errorf("设置资料库临时文件权限失败 path=%s: %w", tempPath, err)
	}
	if _, err := temp.Write(append(data, '\n')); err != nil {
		closeTemp()
		return fmt.Errorf("写入资料库临时文件失败 path=%s: %w", tempPath, err)
	}
	if err := temp.Sync(); err != nil {
		closeTemp()
		return fmt.Errorf("同步资料库临时文件失败 path=%s: %w", tempPath, err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("关闭资料库临时文件失败 path=%s: %w", tempPath, err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("原子替换资料库文件失败 path=%s: %w", path, err)
	}
	if err := localfs.SyncDirectory(dir); err != nil {
		slog.ErrorContext(context.Background(), fmt.Sprintf("[lore-store] directory durability failed path=%s err=%v", dir, err))
	}
	return nil
}

func (s *Store) itemsPath() string {
	return ItemsPath(s.workspace)
}

func (s *Store) hasItem(items []Item, id string) bool {
	return loreItemIndex(items, id) >= 0
}
