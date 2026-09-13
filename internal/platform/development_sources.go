package platform

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
)

// DevelopmentSource is a read-only projection for the extension directory.
// Source files remain authoritative; invalid drafts stay visible and repairable.
type DevelopmentSource struct {
	Development
	ProjectName string    `json:"projectName"`
	Manifest    *Manifest `json:"manifest,omitempty"`
	MessageKey  string    `json:"messageKey,omitempty"`
}

func (m *Manager) DevelopmentSources() ([]DevelopmentSource, error) {
	items, err := m.Developments()
	if err != nil {
		return nil, err
	}
	result := make([]DevelopmentSource, 0, len(items))
	for _, item := range items {
		source := DevelopmentSource{Development: item}
		record, layout, err := m.registry.Resolve(item.ProjectID, true)
		if err == nil {
			source.ProjectName = record.Name
			err = validateDevelopmentPath(item.RelativePath)
		}
		if err == nil {
			source.Manifest, err = readSourceManifest(filepath.Join(layout.ContentRoot, filepath.FromSlash(item.RelativePath)), item.Kind)
		}
		if err != nil {
			source.MessageKey = "platform.sourceInvalid"
			slog.Warn("platform_source_read_failed", "project", item.ProjectID, "source", item.RelativePath, "error", err)
		}
		result = append(result, source)
	}
	return result, nil
}

func readSourceManifest(directory string, kind Kind) (*Manifest, error) {
	root, err := os.OpenRoot(directory)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	file, err := root.Open(kind.manifestFile())
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, MaxDefinitionBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > MaxDefinitionBytes {
		return nil, failure("LIMIT_EXCEEDED", "Source manifest exceeds the definition size limit")
	}
	var manifest Manifest
	if err := decodeJSON(data, &manifest); err != nil {
		return nil, err
	}
	return &manifest, nil
}
