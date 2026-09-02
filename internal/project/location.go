package project

import (
	"fmt"
	"path"
	"path/filepath"
	"strings"

	"denova/internal/portablepath"
)

func normalizeManagedLocationPath(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || strings.Contains(value, `\`) || path.IsAbs(value) {
		return "", fmt.Errorf("invalid managed Project location %q", value)
	}
	clean := path.Clean(value)
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("managed Project location escapes the data directory: %q", value)
	}
	if clean != "." {
		if err := portablepath.Validate(clean); err != nil {
			return "", fmt.Errorf("managed Project location is not portable: %w", err)
		}
	}
	return clean, nil
}

func (registry *Registry) locationForWorkspace(workspace string) (ProjectLocation, error) {
	workspace, err := canonicalDirectory(workspace, false)
	if err != nil {
		return ProjectLocation{}, err
	}
	root, err := canonicalDirectory(registry.denovaDir, false)
	if err != nil {
		return ProjectLocation{}, err
	}
	relative, err := filepath.Rel(root, workspace)
	if err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		managed, normalizeErr := normalizeManagedLocationPath(filepath.ToSlash(relative))
		if normalizeErr != nil {
			return ProjectLocation{}, normalizeErr
		}
		return ProjectLocation{Kind: LocationManaged, Path: managed}, nil
	}
	return ProjectLocation{Kind: LocationExternal, Path: workspace}, nil
}

func (registry *Registry) resolveLocation(location ProjectLocation) (string, error) {
	switch location.Kind {
	case LocationManaged:
		relative, err := normalizeManagedLocationPath(location.Path)
		if err != nil {
			return "", err
		}
		root, err := filepath.Abs(registry.denovaDir)
		if err != nil {
			return "", err
		}
		return filepath.Clean(filepath.Join(root, filepath.FromSlash(relative))), nil
	case LocationExternal:
		external := strings.TrimSpace(location.Path)
		if external == "" {
			return "", fmt.Errorf("external Project location is required")
		}
		// A foreign absolute path may not be absolute according to this host's
		// filepath rules. Preserve it verbatim so opening the registry cannot
		// corrupt it or reinterpret it below the current working directory.
		if !filepath.IsAbs(external) {
			return external, nil
		}
		return filepath.Clean(external), nil
	default:
		return "", fmt.Errorf("invalid Project location kind %q", location.Kind)
	}
}

func projectLocationKey(location ProjectLocation) (string, error) {
	switch location.Kind {
	case LocationManaged:
		managed, err := normalizeManagedLocationPath(location.Path)
		if err != nil {
			return "", err
		}
		return string(LocationManaged) + "\x00" + storeDirNameKey(managed), nil
	case LocationExternal:
		external := strings.TrimSpace(location.Path)
		if external == "" {
			return "", fmt.Errorf("external Project location is required")
		}
		return string(LocationExternal) + "\x00" + external, nil
	default:
		return "", fmt.Errorf("invalid Project location kind %q", location.Kind)
	}
}

func projectLocationBase(location ProjectLocation) string {
	value := strings.TrimSpace(location.Path)
	value = strings.ReplaceAll(value, `\`, "/")
	base := strings.TrimSpace(path.Base(value))
	if base == "" || base == "." || base == "/" {
		return "Project"
	}
	return base
}

func (registry *Registry) projectRecordAtRuntime(record Record) (Record, error) {
	workspace, err := registry.resolveLocation(record.Location)
	if err != nil {
		return Record{}, fmt.Errorf("project %s: %w", record.ID, err)
	}
	record.WorkspacePath = workspace
	record.Status = ""
	return record, nil
}
