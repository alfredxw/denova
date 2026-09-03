package tools

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// LocalWorkspace owns the canonical Project root shared by file, search, and
// process Adapters. Project-bound operations cross os.Root; read/search paths
// may resolve to explicit external roots whose authorization belongs to the
// host Permission Policy.
type LocalWorkspace struct {
	root              string
	rootIdentity      os.FileInfo
	ripgrepExecutable string
	limits            WorkspaceLimits
}

// WorkspaceOptions configures host-owned dependencies without coupling the
// reusable Agent module to an application's release layout.
type WorkspaceOptions struct {
	Root              string
	RipgrepExecutable string
	Limits            WorkspaceLimits
}

// WorkspaceLimits are host-owned safety and pagination defaults shared by
// local read, glob, grep, and process Adapters. Zero values select the high
// built-in defaults; products can bind them to their context policy.
type WorkspaceLimits struct {
	MaxResultBytes        int
	MaxResultEntries      int
	DefaultReadLines      int
	DefaultDirectoryDepth int
	DefaultDirectoryItems int
	MaxDirectoryDepth     int
}

// OpenWorkspace validates and canonicalizes a local workspace root.
func OpenWorkspace(root string) (*LocalWorkspace, error) {
	return OpenWorkspaceWithOptions(WorkspaceOptions{Root: root})
}

// OpenWorkspaceWithOptions validates the workspace and optional ripgrep path.
func OpenWorkspaceWithOptions(options WorkspaceOptions) (*LocalWorkspace, error) {
	root := strings.TrimSpace(options.Root)
	if root == "" {
		return nil, errors.New("workspace path is required")
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve workspace path: %w", err)
	}
	canonical, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return nil, fmt.Errorf("canonicalize workspace path: %w", err)
	}
	openedRoot, err := os.OpenRoot(canonical)
	if err != nil {
		return nil, fmt.Errorf("open workspace: %w", err)
	}
	defer openedRoot.Close()
	info, err := openedRoot.Stat(".")
	if err != nil {
		return nil, fmt.Errorf("stat workspace: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("workspace path is not a directory: %s", canonical)
	}
	if options.Limits.MaxResultBytes > maxConfiguredResultBytes {
		return nil, fmt.Errorf("workspace result byte limit cannot exceed %d", maxConfiguredResultBytes)
	}
	if options.Limits.MaxResultEntries > maxConfiguredEntries {
		return nil, fmt.Errorf("workspace result entry limit cannot exceed %d", maxConfiguredEntries)
	}
	ripgrepExecutable := "rg"
	if configured := strings.TrimSpace(options.RipgrepExecutable); configured != "" {
		ripgrepExecutable, err = validateExecutable(configured)
		if err != nil {
			return nil, fmt.Errorf("validate ripgrep executable: %w", err)
		}
	}
	return &LocalWorkspace{
		root: filepath.Clean(canonical), rootIdentity: info, ripgrepExecutable: ripgrepExecutable,
		limits: normalizeWorkspaceLimits(options.Limits),
	}, nil
}

// Root returns the canonical workspace path.
func (workspace *LocalWorkspace) Root() string {
	if workspace == nil {
		return ""
	}
	return workspace.root
}

// Limits returns the normalized policy bound to this workspace.
func (workspace *LocalWorkspace) Limits() WorkspaceLimits {
	if workspace == nil {
		return normalizeWorkspaceLimits(WorkspaceLimits{})
	}
	return workspace.limits
}

func normalizeWorkspaceLimits(limits WorkspaceLimits) WorkspaceLimits {
	if limits.MaxResultBytes <= 0 {
		limits.MaxResultBytes = defaultResultBytes
	}
	if limits.MaxResultEntries <= 0 {
		limits.MaxResultEntries = defaultResultEntries
	}
	if limits.DefaultReadLines <= 0 {
		limits.DefaultReadLines = defaultReadLines
	}
	if limits.DefaultReadLines > limits.MaxResultEntries {
		limits.DefaultReadLines = limits.MaxResultEntries
	}
	if limits.DefaultDirectoryDepth <= 0 {
		limits.DefaultDirectoryDepth = defaultDirectoryDepth
	}
	if limits.DefaultDirectoryItems <= 0 {
		limits.DefaultDirectoryItems = defaultDirectoryItems
	}
	if limits.MaxDirectoryDepth <= 0 {
		limits.MaxDirectoryDepth = defaultMaxDirectoryDepth
	}
	if limits.DefaultDirectoryDepth > limits.MaxDirectoryDepth {
		limits.DefaultDirectoryDepth = limits.MaxDirectoryDepth
	}
	if limits.DefaultDirectoryItems > limits.MaxResultEntries {
		limits.DefaultDirectoryItems = limits.MaxResultEntries
	}
	return limits
}

func (workspace *LocalWorkspace) openRoot() (*os.Root, error) {
	if workspace == nil || workspace.root == "" {
		return nil, errors.New("workspace is not configured")
	}
	root, err := os.OpenRoot(workspace.root)
	if err != nil {
		return nil, fmt.Errorf("open workspace root: %w", err)
	}
	info, err := root.Stat(".")
	if err != nil {
		root.Close()
		return nil, fmt.Errorf("verify workspace root identity: %w", err)
	}
	if workspace.rootIdentity == nil || !os.SameFile(workspace.rootIdentity, info) {
		root.Close()
		return nil, errors.New("workspace root identity changed; reopen the workspace before using tools")
	}
	return root, nil
}

func (workspace *LocalWorkspace) verifyRootIdentity() error {
	root, err := workspace.openRoot()
	if err != nil {
		return err
	}
	return root.Close()
}

func (workspace *LocalWorkspace) relative(input string, allowRoot bool) (string, error) {
	if workspace == nil || workspace.root == "" {
		return "", errors.New("workspace is not configured")
	}
	input = strings.TrimSpace(input)
	if input == "" {
		return "", errors.New("workspace path is required")
	}
	var relative string
	var err error
	if filepath.IsAbs(input) {
		relative, err = filepath.Rel(workspace.root, filepath.Clean(input))
		if err != nil {
			return "", fmt.Errorf("resolve workspace-relative path: %w", err)
		}
	} else {
		relative = filepath.Clean(filepath.FromSlash(input))
	}
	if relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return "", fmt.Errorf("path is outside the active workspace: %s", input)
	}
	if relative == "." && !allowRoot {
		return "", errors.New("path must identify an item inside the active workspace")
	}
	return filepath.ToSlash(relative), nil
}

func (workspace *LocalWorkspace) stat(input string, allowRoot bool) (string, os.FileInfo, error) {
	relative, err := workspace.relative(input, allowRoot)
	if err != nil {
		return "", nil, err
	}
	root, err := workspace.openRoot()
	if err != nil {
		return "", nil, err
	}
	defer root.Close()
	info, err := root.Stat(filepath.FromSlash(relative))
	if err != nil {
		return "", nil, fmt.Errorf("inspect workspace path %s: %w", relative, err)
	}
	return relative, info, nil
}

func validateExecutable(input string) (string, error) {
	absolute, err := filepath.Abs(strings.TrimSpace(input))
	if err != nil {
		return "", fmt.Errorf("resolve executable: %w", err)
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return "", fmt.Errorf("stat executable: %w", err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("executable is not a regular file: %s", absolute)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o111 == 0 {
		return "", fmt.Errorf("file is not executable: %s", absolute)
	}
	return absolute, nil
}
