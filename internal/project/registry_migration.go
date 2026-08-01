package project

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"
)

type legacyBookRecord struct {
	Name         string `json:"name"`
	Path         string `json:"path"`
	LastOpenedAt string `json:"last_opened_at"`
}

type legacyBookRegistry struct {
	Current  string             `json:"current"`
	Books    []legacyBookRecord `json:"books"`
	SortMode string             `json:"sort_mode"`
	Order    []string           `json:"order"`
	Hidden   []string           `json:"hidden"`
}

func (registry *Registry) loadOrMigrateLocked() (registryData, bool, error) {
	var data registryData
	raw, err := os.ReadFile(registry.path)
	if err == nil {
		if err := json.Unmarshal(raw, &data); err != nil {
			return registryData{}, false, fmt.Errorf("decode project registry: %w", err)
		}
		if data.Version > registryVersion {
			return registryData{}, false, fmt.Errorf(
				"project registry version %d is newer than supported version %d",
				data.Version,
				registryVersion,
			)
		}
		changed := false
		if data.Version < registryVersion {
			if err := registry.backupRegistryLocked(raw, data.Version); err != nil {
				return registryData{}, false, fmt.Errorf("back up project registry: %w", err)
			}
			archived, err := registry.upgradeRegistryLocked(&data)
			if err != nil {
				return registryData{}, false, err
			}
			slog.InfoContext(context.Background(), fmt.Sprintf(
				"[project/registry_migration.go] upgraded Project registry path=%s version=%d archived_stale_legacy=%d",
				registry.path,
				registryVersion,
				archived,
			))
			changed = true
		}
		normalizeRegistryData(&data)
		return data, changed, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return registryData{}, false, err
	}
	data = registryData{Version: registryVersion, SortMode: SortRecent, Projects: []Record{}}
	migrated, err := registry.importLegacyBooksLocked(&data)
	if err != nil {
		return registryData{}, false, err
	}
	return data, migrated, nil
}

// upgradeRegistryLocked keeps user-created missing Projects visible while
// archiving stale entries that can be proven to originate in books.json.
func (registry *Registry) upgradeRegistryLocked(data *registryData) (int, error) {
	if data.Version < 0 {
		return 0, fmt.Errorf("invalid project registry version %d", data.Version)
	}
	archived := 0
	if data.Version < 2 {
		count, err := registry.archiveStaleLegacyProjectsLocked(data)
		if err != nil {
			return 0, err
		}
		archived += count
		data.Version = 2
	}
	return archived, nil
}

func (registry *Registry) archiveStaleLegacyProjectsLocked(data *registryData) (int, error) {
	legacy, found, err := registry.readLegacyBookRegistryLocked()
	if err != nil || !found {
		return 0, err
	}
	bookPaths, hiddenPaths := legacyProjectPathSets(legacy)
	now := time.Now().UTC()
	archived := 0
	for index := range data.Projects {
		record := &data.Projects[index]
		if record.Type != TypeBook || record.ArchivedAt != nil {
			continue
		}
		canonical, canonicalErr := canonicalDirectory(record.WorkspacePath, false)
		if canonicalErr != nil {
			continue
		}
		_, wasLegacyBook := bookPaths[canonical]
		_, wasHidden := hiddenPaths[canonical]
		if !wasHidden && (!wasLegacyBook || projectDirectoryExists(canonical)) {
			continue
		}
		record.ArchivedAt = timePointer(now)
		record.UpdatedAt = now
		if data.CurrentBookID == record.ID {
			data.CurrentBookID = ""
		}
		archived++
	}
	return archived, nil
}

func (registry *Registry) importLegacyBooksLocked(data *registryData) (bool, error) {
	legacy, found, err := registry.readLegacyBookRegistryLocked()
	if err != nil || !found {
		return false, err
	}
	now := time.Now().UTC()
	_, hiddenPaths := legacyProjectPathSets(legacy)
	pathToID := make(map[string]string, len(legacy.Books)+len(legacy.Hidden))
	activeIDs := make(map[string]bool, len(legacy.Books))
	for _, book := range legacy.Books {
		canonical, canonicalErr := canonicalDirectory(book.Path, false)
		if canonicalErr != nil || canonical == "" || pathToID[canonical] != "" {
			continue
		}
		record, recordErr := newRecord(canonical, TypeBook, book.Name, now)
		if recordErr != nil {
			return false, recordErr
		}
		if opened, parseErr := time.Parse(time.RFC3339Nano, book.LastOpenedAt); parseErr == nil {
			record.LastOpenedAt = opened.UTC()
		}
		_, hidden := hiddenPaths[canonical]
		if hidden || !projectDirectoryExists(canonical) {
			record.ArchivedAt = timePointer(now)
		} else {
			activeIDs[record.ID] = true
		}
		data.Projects = append(data.Projects, record)
		pathToID[canonical] = record.ID
	}

	// Hidden books were not always present in legacy.Books. Persist an archived
	// tombstone so automatic Book discovery cannot resurrect them.
	for _, path := range legacy.Hidden {
		canonical, canonicalErr := canonicalDirectory(path, false)
		if canonicalErr != nil || canonical == "" || pathToID[canonical] != "" {
			continue
		}
		record, recordErr := newRecord(canonical, TypeBook, filepath.Base(canonical), now)
		if recordErr != nil {
			return false, recordErr
		}
		record.ArchivedAt = timePointer(now)
		data.Projects = append(data.Projects, record)
		pathToID[canonical] = record.ID
	}

	for _, path := range legacy.Order {
		if canonical, canonicalErr := canonicalDirectory(path, false); canonicalErr == nil && pathToID[canonical] != "" {
			data.Order = append(data.Order, pathToID[canonical])
		}
	}
	if canonical, canonicalErr := canonicalDirectory(legacy.Current, false); canonicalErr == nil {
		if id := pathToID[canonical]; activeIDs[id] {
			data.CurrentBookID = id
		}
	}
	data.SortMode = normalizedSortMode(SortMode(legacy.SortMode), len(data.Order) > 0)
	return len(data.Projects) > 0 || len(legacy.Hidden) > 0, nil
}

func (registry *Registry) readLegacyBookRegistryLocked() (legacyBookRegistry, bool, error) {
	raw, err := os.ReadFile(registry.legacyBooksPath)
	if errors.Is(err, os.ErrNotExist) {
		return legacyBookRegistry{}, false, nil
	}
	if err != nil {
		return legacyBookRegistry{}, false, err
	}
	var legacy legacyBookRegistry
	if err := json.Unmarshal(raw, &legacy); err != nil {
		return legacyBookRegistry{}, false, fmt.Errorf("decode legacy book registry: %w", err)
	}
	return legacy, true, nil
}

func legacyProjectPathSets(legacy legacyBookRegistry) (map[string]struct{}, map[string]struct{}) {
	books := make(map[string]struct{}, len(legacy.Books))
	for _, book := range legacy.Books {
		if canonical, err := canonicalDirectory(book.Path, false); err == nil && canonical != "" {
			books[canonical] = struct{}{}
		}
	}
	hidden := make(map[string]struct{}, len(legacy.Hidden))
	for _, path := range legacy.Hidden {
		if canonical, err := canonicalDirectory(path, false); err == nil && canonical != "" {
			hidden[canonical] = struct{}{}
		}
	}
	return books, hidden
}

func (registry *Registry) backupRegistryLocked(raw []byte, version int) error {
	backupPath := filepath.Join(filepath.Dir(registry.path), fmt.Sprintf("projects.v%d.backup.json", version))
	backup, err := os.OpenFile(backupPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, os.ErrExist) {
		existing, readErr := os.ReadFile(backupPath)
		if readErr != nil {
			return readErr
		}
		if !bytes.Equal(existing, raw) {
			return fmt.Errorf("rollback backup already exists with different content: %s", backupPath)
		}
		return nil
	}
	if err != nil {
		return err
	}
	removeBackup := true
	defer func() {
		_ = backup.Close()
		if removeBackup {
			_ = os.Remove(backupPath)
		}
	}()
	if _, err := backup.Write(raw); err != nil {
		return err
	}
	if err := backup.Sync(); err != nil {
		return err
	}
	if err := backup.Close(); err != nil {
		return err
	}
	removeBackup = false
	slog.InfoContext(context.Background(), fmt.Sprintf(
		"[project/registry_migration.go] saved Project registry rollback backup path=%s source_version=%d",
		backupPath,
		version,
	))
	return nil
}

func projectDirectoryExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func timePointer(value time.Time) *time.Time {
	return &value
}
