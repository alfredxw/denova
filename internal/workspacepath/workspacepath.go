package workspacepath

import (
	"encoding/json"
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
// Existing .denova wins for new workspaces. When both names exist because a
// legacy workspace was opened during the Denova rename, old .nova state is kept
// active if .denova only contains generated or ephemeral files.
func DirName(workspace string) string {
	return dirNameFor(workspace)
}

func dirNameFor(workspace string, elem ...string) string {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return DataDirName
	}
	currentDir := filepath.Join(workspace, DataDirName)
	legacyDir := filepath.Join(workspace, LegacyDataDirName)
	currentExists := isDir(currentDir)
	legacyExists := isDir(legacyDir)
	if currentExists && legacyExists {
		if len(elem) > 0 && preferLegacyTarget(currentDir, legacyDir, elem...) {
			return LegacyDataDirName
		}
		if hasWorkspaceData(legacyDir, true) && !hasWorkspaceData(currentDir, true) {
			return LegacyDataDirName
		}
		return DataDirName
	}
	if currentExists {
		return DataDirName
	}
	if legacyExists {
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
	parts := append([]string{filepath.Join(workspace, dirNameFor(workspace, elem...))}, elem...)
	return filepath.Join(parts...)
}

// Rel joins elem under the active workspace-private data directory name.
func Rel(workspace string, elem ...string) string {
	parts := append([]string{dirNameFor(workspace, elem...)}, elem...)
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

func preferLegacyTarget(currentDir, legacyDir string, elem ...string) bool {
	currentTarget := filepath.Join(append([]string{currentDir}, elem...)...)
	legacyTarget := filepath.Join(append([]string{legacyDir}, elem...)...)
	legacyInfo, err := os.Stat(legacyTarget)
	if err != nil {
		return false
	}
	currentInfo, err := os.Stat(currentTarget)
	if os.IsNotExist(err) {
		return true
	}
	if err != nil {
		return false
	}
	return hasWorkspaceDataWithInfo(legacyTarget, legacyInfo, false) &&
		!hasWorkspaceDataWithInfo(currentTarget, currentInfo, false)
}

func hasWorkspaceData(path string, ignoreEphemeral bool) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return hasWorkspaceDataWithInfo(path, info, ignoreEphemeral)
}

func hasWorkspaceDataWithInfo(path string, info os.FileInfo, ignoreEphemeral bool) bool {
	if !info.IsDir() {
		return fileHasWorkspaceData(path, info)
	}
	found := false
	_ = filepath.WalkDir(path, func(child string, entry os.DirEntry, err error) error {
		if err != nil || found {
			return nil
		}
		if child == path {
			return nil
		}
		if ignoreEphemeral && entry.IsDir() && isEphemeralRoot(path, child) {
			return filepath.SkipDir
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return nil
		}
		if fileHasWorkspaceData(child, info) {
			found = true
		}
		return nil
	})
	return found
}

func isEphemeralRoot(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	switch filepath.ToSlash(rel) {
	case "runs", "checkpoints":
		return true
	default:
		return false
	}
}

func fileHasWorkspaceData(path string, info os.FileInfo) bool {
	if info.Size() == 0 || filepath.Base(path) == ".DS_Store" {
		return false
	}
	return !isEmptyLoreItemsFile(path, info.Size())
}

func isEmptyLoreItemsFile(path string, size int64) bool {
	if filepath.Base(path) != "items.json" || filepath.Base(filepath.Dir(path)) != "lore" {
		return false
	}
	if size > 256*1024 {
		return false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	if strings.TrimSpace(string(data)) == "" {
		return true
	}
	var collection struct {
		Items json.RawMessage `json:"items"`
	}
	if err := json.Unmarshal(data, &collection); err == nil && collection.Items != nil {
		rawItems := strings.TrimSpace(string(collection.Items))
		if rawItems == "" || rawItems == "null" {
			return true
		}
		var items []json.RawMessage
		if err := json.Unmarshal(collection.Items, &items); err == nil {
			return len(items) == 0
		}
		return false
	}
	var items []json.RawMessage
	if err := json.Unmarshal(data, &items); err == nil {
		return len(items) == 0
	}
	return false
}
