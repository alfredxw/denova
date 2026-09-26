package lore

import (
	"context"
	"denova/internal/revisionfile"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
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
	snapshot, err := revisionfile.Read(context.Background(), path)
	if err == nil && snapshot.Exists {
		data := snapshot.Content
		// Version 1 remains readable regardless of where a user copied it; every
		// subsequent typed save upgrades the same collection to version 2.
		collection, decodeErr := decodeLoreCollectionJSON(data)
		if decodeErr != nil {
			return Collection{}, fmt.Errorf("解析 Lore items 失败 path=%s: %w", path, decodeErr)
		}
		return collection, nil
	}
	if err != nil {
		return Collection{}, err
	}
	return Collection{Version: loreItemsVersion, Items: []Item{}}, nil
}

func (s *Store) save(collection Collection) error {
	collection.Version = loreItemsVersion
	normalized := make([]Item, 0, len(collection.Items))
	for _, item := range collection.Items {
		item.ResolvedMaterials = nil
		normalized = append(normalized, normalizeLoreItem(item))
	}
	collection.Items = normalized
	if err := validateLoreItemIdentities(collection.Items); err != nil {
		return fmt.Errorf("拒绝保存无效 Lore collection: %w", err)
	}
	if err := validateMaterials(collection); err != nil {
		return err
	}
	path := s.itemsPath()
	data, err := json.MarshalIndent(collection, "", "  ")
	if err != nil {
		return err
	}
	_, err = revisionfile.ReplaceIfRevision(context.Background(), path, "", append(data, '\n'), revisionfile.Options{})
	return err
}

func (s *Store) itemsPath() string {
	return ItemsPath(s.workspace)
}

func (s *Store) hasItem(items []Item, id string) bool {
	return loreItemIndex(items, id) >= 0
}

// WithMutationLock coordinates an application-level resource transaction with
// normal Lore mutations. The callback must not call this Store's mutators.
func (s *Store) WithMutationLock(operation func() error) error {
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()
	return operation()
}
