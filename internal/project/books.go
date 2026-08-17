package project

import (
	"path/filepath"
	"strings"
)

// ContentDirectoryName is the default parent for project content stored under
// the Denova data directory. User-selected external parents remain unchanged.
const ContentDirectoryName = "projects"

// Books returns available, non-archived Book projects in the registry's shared
// order. Missing Books remain visible through List so callers can relink them,
// but they are not valid workspace-switch targets.
func (registry *Registry) Books() ([]Record, error) {
	records, err := registry.List(false)
	if err != nil {
		return nil, err
	}
	books := make([]Record, 0, len(records))
	for _, record := range records {
		if record.Type == TypeBook && record.Status == StatusAvailable {
			books = append(books, record)
		}
	}
	return books, nil
}

// ReorderBooks replaces only Book slots in the unified Project order. General
// Projects keep their relative positions, so the bookshelf and Project picker
// can share one durable ordering authority without corrupting each other.
func (registry *Registry) ReorderBooks(paths []string) error {
	registry.mu.Lock()
	defer registry.mu.Unlock()

	data, _, err := registry.loadAndDiscoverLocked()
	if err != nil {
		return err
	}
	records := orderedRecords(data, false)
	bookByPath := make(map[string]string, len(records))
	for _, record := range records {
		if record.Type == TypeBook {
			bookByPath[filepath.Clean(record.WorkspacePath)] = record.ID
		}
	}

	bookIDs := make([]string, 0, len(bookByPath))
	seen := make(map[string]bool, len(bookByPath))
	for _, path := range paths {
		canonical, canonicalErr := canonicalDirectory(path, false)
		if canonicalErr != nil {
			continue
		}
		id := bookByPath[canonical]
		if id != "" && !seen[id] {
			seen[id] = true
			bookIDs = append(bookIDs, id)
		}
	}
	for _, record := range records {
		if record.Type == TypeBook && !seen[record.ID] {
			seen[record.ID] = true
			bookIDs = append(bookIDs, record.ID)
		}
	}

	order := make([]string, 0, len(records))
	nextBook := 0
	for _, record := range records {
		if record.Type != TypeBook {
			order = append(order, record.ID)
			continue
		}
		order = append(order, bookIDs[nextBook])
		nextBook++
	}
	data.Order = order
	data.SortMode = SortManual
	return registry.saveLocked(data)
}

// BookCreationParent resolves the parent used when creating a Book. Selecting
// the Denova data directory places content beneath its dedicated projects
// directory; selecting any other directory preserves the user's choice.
func BookCreationParent(parentDir, denovaDir string) (string, error) {
	parent, err := filepath.Abs(parentDir)
	if err != nil {
		return "", err
	}
	denovaDir = strings.TrimSpace(denovaDir)
	if denovaDir == "" {
		return parent, nil
	}
	dataRoot, err := filepath.Abs(denovaDir)
	if err == nil && parent == dataRoot {
		return filepath.Join(parent, ContentDirectoryName), nil
	}
	return parent, nil
}
