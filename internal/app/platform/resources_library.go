package platform

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"

	booklore "denova/internal/book/lore"
	"denova/internal/platform"
	"denova/internal/portablepath"
	"denova/internal/revisionfile"
)

func (host Resources) WriteLibraryItem(ctx context.Context, projectID string, input platform.LibraryWrite) (platform.LibraryItem, error) {
	var result platform.LibraryItem
	operation, err := host.host.AcquireProject(ctx, projectID)
	if err != nil {
		return result, err
	}
	defer operation.Release()
	_, err = host.host.WithLoreStore(operation.Context(), projectID, func(store *booklore.Store) error {
		old, readErr := store.Get(input.Item.ID)
		exists := readErr == nil
		if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
			return readErr
		}
		item := input.Item
		item.Type, item.Name = booklore.NormalizeType(item.Type), strings.TrimSpace(item.Name)
		item.Content, item.BriefDescription = strings.TrimSpace(item.Content), strings.TrimSpace(item.BriefDescription)
		if exists && old.Name == item.Name && old.Type == item.Type && old.Content == item.Content && (item.BriefDescription == "" || old.BriefDescription == item.BriefDescription) && slices.Equal(old.Tags, item.Tags) && slices.Equal(old.Keywords, item.Keywords) && old.Enabled == item.Enabled {
			result = publicLibraryItem(old)
			return nil
		}
		if exists && (input.BaseRevision == "" || input.BaseRevision != old.UpdatedAt) || !exists && input.BaseRevision != "" {
			return &platform.Error{Code: "DOCUMENT_CONFLICT", MessageKey: "platform.errors.DOCUMENT_CONFLICT", Diagnostic: "Library item changed since it was read"}
		}
		mutation := booklore.ItemInput{ID: item.ID, Enabled: &item.Enabled, Type: item.Type, Name: item.Name, Tags: item.Tags, BriefDescription: item.BriefDescription, Keywords: item.Keywords, Content: item.Content, BaseRevision: input.BaseRevision}
		var saved booklore.Item
		var saveErr error
		if exists {
			data, err := json.MarshalIndent(old, "", "  ")
			if err != nil {
				return err
			}
			name := revisionfile.Revision(data)[7:] + ".json"
			backup := filepath.Join(operation.Layout().StoreRoot, "backups", "extension-library", name)
			if _, err := revisionfile.ReplaceIfRevision(operation.Context(), backup, revisionfile.MissingRevision, data, revisionfile.Options{FileMode: 0600, DirectoryMode: 0700}); err != nil && !errors.Is(err, revisionfile.ErrRevisionConflict) {
				return err
			}
			mutation.Importance, mutation.LoadMode = old.Importance, old.LoadMode
			saved, saveErr = store.Update(item.ID, mutation)
		} else {
			mutation.Provenance = &booklore.Provenance{Kind: "extension", SourceName: input.SourceName, SourceRecordID: input.SourceID, SourceHash: input.SourceHash}
			saved, saveErr = store.Create(mutation)
		}
		if errors.Is(saveErr, booklore.ErrRevisionConflict) {
			return &platform.Error{Code: "DOCUMENT_CONFLICT", MessageKey: "platform.errors.DOCUMENT_CONFLICT", Diagnostic: "Library item changed during save"}
		}
		if saveErr != nil {
			return saveErr
		}
		result = publicLibraryItem(saved)
		return nil
	})
	return result, err
}

func publicLibraryItem(item booklore.Item) platform.LibraryItem {
	result := platform.LibraryItem{ID: item.ID, Type: item.Type, Name: item.Name, Enabled: item.Enabled, Tags: item.Tags, BriefDescription: item.BriefDescription, Keywords: item.Keywords, Content: item.Content, UpdatedAt: item.UpdatedAt}
	if item.Image != nil && strings.HasPrefix(item.Image.ImagePath, "assets/") && portablepath.Validate(item.Image.ImagePath) == nil {
		result.Image = &platform.AssetRef{Kind: "project", Path: item.Image.ImagePath}
	}
	return result
}
