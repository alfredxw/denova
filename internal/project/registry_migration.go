package project

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const projectIDStateDirectoryRegistryVersion = 2

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
		switch data.Version {
		case registryVersion:
			normalizeRegistryData(&data)
			if err := validateStateDirNames(data.Projects); err != nil {
				return registryData{}, false, err
			}
			if err := registry.migrateProjectIDStateDirectoriesOnce(data.Projects); err != nil {
				return registryData{}, false, err
			}
			return data, false, nil
		case projectIDStateDirectoryRegistryVersion:
			normalizeRegistryData(&data)
			if err := registry.assignReadableStateDirNames(&data); err != nil {
				return registryData{}, false, err
			}
			// Persist the mapping before moving state. If a move fails, the next
			// load reads the new mapping and safely resumes the remaining moves.
			if err := registry.saveLocked(data); err != nil {
				return registryData{}, false, fmt.Errorf("persist readable project state directories: %w", err)
			}
			if err := registry.migrateProjectIDStateDirectoriesOnce(data.Projects); err != nil {
				return registryData{}, false, err
			}
			return data, false, nil
		default:
			return registryData{}, false, fmt.Errorf(
				"project registry version %d does not match supported version %d",
				data.Version,
				registryVersion,
			)
		}
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

func (registry *Registry) importLegacyBooksLocked(data *registryData) (bool, error) {
	legacy, found, err := registry.readLegacyBookRegistryLocked()
	if err != nil || !found {
		return false, err
	}
	now := time.Now().UTC()
	hiddenPaths := legacyHiddenProjectPaths(legacy.Hidden)
	pathToID := make(map[string]string, len(legacy.Books)+len(legacy.Hidden))
	activeIDs := make(map[string]bool, len(legacy.Books))
	for _, book := range legacy.Books {
		canonical, canonicalErr := canonicalDirectory(book.Path, false)
		if canonicalErr != nil || canonical == "" || pathToID[canonical] != "" {
			continue
		}
		record, recordErr := registry.newRecord(data, canonical, TypeBook, book.Name, now)
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
		record, recordErr := registry.newRecord(data, canonical, TypeBook, filepath.Base(canonical), now)
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

func legacyHiddenProjectPaths(paths []string) map[string]struct{} {
	hidden := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		if canonical, err := canonicalDirectory(path, false); err == nil && canonical != "" {
			hidden[canonical] = struct{}{}
		}
	}
	return hidden
}

func projectDirectoryExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func timePointer(value time.Time) *time.Time {
	return &value
}
