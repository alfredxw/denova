package project

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"denova/internal/portablepath"
)

// CreateDirectoryRequest creates a new General Project. An omitted parent uses
// the managed projects directory; an explicit parent must already exist.
type CreateDirectoryRequest struct {
	Name            string `json:"name"`
	ParentDirectory string `json:"parent_directory,omitempty"`
}

func (registry *Registry) CreateDirectory(request CreateDirectoryRequest) (Record, error) {
	name, err := portablepath.NormalizeComponent(request.Name)
	if err != nil {
		return Record{}, err
	}
	parent := request.ParentDirectory
	if parent == "" {
		parent = filepath.Join(registry.denovaDir, "projects")
		if err := os.MkdirAll(parent, 0o700); err != nil {
			return Record{}, err
		}
	} else if !filepath.IsAbs(parent) {
		return Record{}, fmt.Errorf("project parent must be an absolute directory")
	}
	if err := portablepath.CheckNoCollision(parent, name); err != nil {
		return Record{}, err
	}
	root, err := os.OpenRoot(parent)
	if err != nil {
		return Record{}, err
	}
	defer root.Close()
	if err := root.Mkdir(name, 0o700); err != nil {
		return Record{}, err
	}
	path := filepath.Join(parent, name)
	record, err := registry.Add(path, TypeGeneral, name)
	if err != nil {
		// Remove only the empty directory this operation created.
		if cleanupErr := root.Remove(name); cleanupErr != nil {
			slog.Warn("project_directory_cleanup_failed", "path", path, "error", cleanupErr)
		}
		return Record{}, err
	}
	if _, err := registry.EnsureStore(record); err != nil {
		return record, err
	}
	slog.Info("project_directory_created", "project", record.ID, "location", record.Location.Path)
	return record, nil
}
