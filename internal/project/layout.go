package project

import (
	"fmt"
	"path/filepath"
	"strings"
)

const StateDirectoryName = "project-state"

func joinState(root string, elements ...string) string {
	parts := append([]string{root}, elements...)
	return filepath.Join(parts...)
}

func (registry *Registry) Layout(record Record) (Layout, error) {
	if registry == nil || strings.TrimSpace(registry.denovaDir) == "" {
		return Layout{}, fmt.Errorf("Denova data directory is required")
	}
	if err := ValidateID(record.ID); err != nil {
		return Layout{}, err
	}
	if err := validateStateDirName(record.StateDirName); err != nil {
		return Layout{}, err
	}
	return Layout{
		ProjectID:   record.ID,
		Type:        record.Type,
		ContentRoot: record.WorkspacePath,
		StateRoot:   filepath.Join(registry.denovaDir, StateDirectoryName, record.StateDirName),
	}, nil
}

func ValidateID(id string) error {
	id = strings.TrimSpace(id)
	if id == "" || len(id) > 128 {
		return fmt.Errorf("invalid project ID")
	}
	for _, char := range id {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') || char == '-' || char == '_' {
			continue
		}
		return fmt.Errorf("invalid project ID %q", id)
	}
	return nil
}
