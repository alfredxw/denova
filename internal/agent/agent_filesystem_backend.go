package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const agentFileReadDefaultLimitLines = 2000

// agentFilesystemBackend is the single workspace-rooted filesystem boundary
// used by model-visible read and search tools. Every operation opens an
// os.Root so symlinks and traversal cannot escape the admitted workspace.
type agentFilesystemBackend struct {
	workspace string
}

func newAgentFilesystemBackend(workspace string) (*agentFilesystemBackend, error) {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return nil, fmt.Errorf("workspace path is required")
	}
	absolute, err := filepath.Abs(workspace)
	if err != nil {
		return nil, fmt.Errorf("resolve workspace path: %w", err)
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return nil, fmt.Errorf("stat workspace: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("workspace path is not a directory: %s", absolute)
	}
	canonical, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return nil, fmt.Errorf("canonicalize workspace path: %w", err)
	}
	return &agentFilesystemBackend{workspace: filepath.Clean(canonical)}, nil
}

func (b *agentFilesystemBackend) Workspace() string {
	if b == nil {
		return ""
	}
	return b.workspace
}

func (b *agentFilesystemBackend) openRoot() (*os.Root, error) {
	if b == nil || strings.TrimSpace(b.workspace) == "" {
		return nil, fmt.Errorf("filesystem backend has no workspace")
	}
	root, err := os.OpenRoot(b.workspace)
	if err != nil {
		return nil, fmt.Errorf("open workspace root: %w", err)
	}
	return root, nil
}

// resolvePath converts an absolute or workspace-relative model input into a
// canonical display path and an os.Root-relative path. Existence and symlink
// containment are checked by the concrete operation through os.Root.
func (b *agentFilesystemBackend) resolvePath(input string, allowRoot bool) (absolute, relative string, err error) {
	if b == nil || strings.TrimSpace(b.workspace) == "" {
		return "", "", fmt.Errorf("filesystem backend has no workspace")
	}
	input = strings.TrimSpace(input)
	if input == "" {
		return "", "", fmt.Errorf("workspace path is required")
	}

	var rel string
	if filepath.IsAbs(input) {
		cleanInput := filepath.Clean(input)
		if canonical, canonicalErr := filepath.EvalSymlinks(cleanInput); canonicalErr == nil {
			cleanInput = canonical
		}
		rel, err = filepath.Rel(b.workspace, cleanInput)
		if err != nil {
			return "", "", fmt.Errorf("resolve workspace-relative path: %w", err)
		}
	} else {
		rel = filepath.Clean(filepath.FromSlash(input))
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", "", fmt.Errorf("path is outside the active workspace: %s", input)
	}
	if rel == "." && !allowRoot {
		return "", "", fmt.Errorf("path must identify an item inside the active workspace")
	}
	return filepath.Join(b.workspace, rel), filepath.ToSlash(rel), nil
}

func (b *agentFilesystemBackend) validateExistingPath(input string, allowRoot bool) (absolute, relative string, info os.FileInfo, err error) {
	absolute, relative, err = b.resolvePath(input, allowRoot)
	if err != nil {
		return "", "", nil, err
	}
	root, err := b.openRoot()
	if err != nil {
		return "", "", nil, err
	}
	defer root.Close()
	info, err = root.Stat(filepath.FromSlash(relative))
	if err != nil {
		if os.IsNotExist(err) {
			return "", "", nil, fmt.Errorf("workspace path not found: %s", input)
		}
		return "", "", nil, fmt.Errorf("inspect workspace path %s: %w", input, err)
	}
	return absolute, relative, info, nil
}
