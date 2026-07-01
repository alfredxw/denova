package workspacepath

import (
	"os"
	"path/filepath"
	"strings"
)

const (
	// DataDirName is the current workspace-private data directory.
	DataDirName = ".denova"
	// LegacyDataDirName is the pre-rename workspace-private data directory.
	LegacyDataDirName = ".nova"
)

// DirName returns the active workspace-private directory name.
// Existing .denova wins; existing .nova is reused for legacy workspaces so new
// writes do not split one book's private state across two directories.
func DirName(workspace string) string {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return DataDirName
	}
	if isDir(filepath.Join(workspace, DataDirName)) {
		return DataDirName
	}
	if isDir(filepath.Join(workspace, LegacyDataDirName)) {
		return LegacyDataDirName
	}
	return DataDirName
}

// Dir returns the absolute or workspace-relative active data directory path.
func Dir(workspace string) string {
	return filepath.Join(workspace, DirName(workspace))
}

// Path joins elem under the active workspace-private data directory.
func Path(workspace string, elem ...string) string {
	parts := append([]string{Dir(workspace)}, elem...)
	return filepath.Join(parts...)
}

// Rel joins elem under the active workspace-private data directory name.
func Rel(workspace string, elem ...string) string {
	parts := append([]string{DirName(workspace)}, elem...)
	return filepath.ToSlash(filepath.Join(parts...))
}

// CurrentRel joins elem under the current Denova data directory name.
func CurrentRel(elem ...string) string {
	parts := append([]string{DataDirName}, elem...)
	return filepath.ToSlash(filepath.Join(parts...))
}

// LegacyRel joins elem under the legacy Nova data directory name.
func LegacyRel(elem ...string) string {
	parts := append([]string{LegacyDataDirName}, elem...)
	return filepath.ToSlash(filepath.Join(parts...))
}

// IsInternalRel reports whether rel points inside either workspace-private data
// directory name. It is intended for workspace file APIs and version filters.
func IsInternalRel(rel string) bool {
	clean := filepath.ToSlash(filepath.Clean(strings.TrimSpace(rel)))
	return clean == DataDirName || strings.HasPrefix(clean, DataDirName+"/") ||
		clean == LegacyDataDirName || strings.HasPrefix(clean, LegacyDataDirName+"/")
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
