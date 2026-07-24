package skills

import (
	"fmt"
	"path/filepath"
	"strings"

	"denova/internal/workspacepath"
)

// NewDirectories returns the canonical skill search path for Denova.
func NewDirectories(builtinDir, novaDir, workspace string) []Directory {
	dirs := make([]Directory, 0, 3)
	if path := normalizePath(builtinDir); path != "" {
		dirs = append(dirs, Directory{Scope: ScopeBuiltin, Path: path, Writable: false})
	}
	if path := normalizePath(filepath.Join(novaDir, "skills")); novaDir != "" && path != "" {
		dirs = append(dirs, Directory{Scope: ScopeUser, Path: path, Writable: true})
	}
	if path := normalizePath(workspacepath.Path(workspace, "skills")); workspace != "" && path != "" {
		dirs = append(dirs, Directory{Scope: ScopeWorkspace, Path: path, Writable: true})
	}
	return dirs
}

func directoryForScope(dirs []Directory, scope Scope) (Directory, error) {
	for _, dir := range dedupeDirectories(dirs) {
		if dir.Scope == scope {
			return dir, nil
		}
	}
	return Directory{}, fmt.Errorf("skill scope not configured: %s", scope)
}

func writableDirectoryForScope(dirs []Directory, scope Scope) (Directory, error) {
	dir, err := directoryForScope(dirs, scope)
	if err != nil {
		return Directory{}, err
	}
	if !dir.Writable {
		return Directory{}, fmt.Errorf("skill scope is read-only: %s", scope)
	}
	return dir, nil
}

func scopeInfos(dirs []Directory) []ScopeInfo {
	out := make([]ScopeInfo, 0, len(dirs))
	for _, dir := range dirs {
		out = append(out, ScopeInfo{Scope: dir.Scope, Path: dir.Path, Writable: dir.Writable})
	}
	return out
}

func dedupeDirectories(dirs []Directory) []Directory {
	seen := map[string]bool{}
	out := make([]Directory, 0, len(dirs))
	for _, dir := range dirs {
		if dir.Path == "" {
			continue
		}
		path := normalizePath(dir.Path)
		key := string(dir.Scope) + "\x00" + path
		if seen[key] {
			continue
		}
		seen[key] = true
		dir.Path = path
		out = append(out, dir)
	}
	return out
}

func normalizePath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if abs, err := filepath.Abs(path); err == nil {
		return filepath.Clean(abs)
	}
	return filepath.Clean(path)
}

func scopeRank(scope Scope) int {
	switch scope {
	case ScopeWorkspace:
		return 3
	case ScopeUser:
		return 2
	case ScopeBuiltin:
		return 1
	default:
		return 0
	}
}
