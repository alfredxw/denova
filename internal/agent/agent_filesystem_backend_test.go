package agent

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestAgentFilesystemBackendCanonicalizesWorkspace(t *testing.T) {
	workspace := t.TempDir()
	backend, err := newAgentFilesystemBackend(filepath.Join(workspace, "."))
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := filepath.EvalSymlinks(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if backend.Workspace() != filepath.Clean(canonical) {
		t.Fatalf("workspace = %q, want %q", backend.Workspace(), canonical)
	}
}

func TestAgentFilesystemBackendRejectsMissingAndNonDirectoryWorkspace(t *testing.T) {
	if _, err := newAgentFilesystemBackend(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("missing workspace should be rejected")
	}
	file := filepath.Join(t.TempDir(), "file.txt")
	if err := os.WriteFile(file, []byte("text"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := newAgentFilesystemBackend(file); err == nil || !strings.Contains(err.Error(), "not a directory") {
		t.Fatalf("file workspace should be rejected, got %v", err)
	}
}

func TestAgentFilesystemBackendResolvesOnlyWorkspacePaths(t *testing.T) {
	workspace := t.TempDir()
	backend := newTestAgentFilesystemBackend(t, workspace)
	absolute, relative, err := backend.resolvePath("chapters/ch01.md", false)
	if err != nil {
		t.Fatal(err)
	}
	if absolute != filepath.Join(backend.Workspace(), "chapters", "ch01.md") || relative != "chapters/ch01.md" {
		t.Fatalf("unexpected resolution absolute=%q relative=%q", absolute, relative)
	}
	outside := filepath.Join(filepath.Dir(workspace), "outside.txt")
	if _, _, err := backend.resolvePath(outside, false); err == nil || !strings.Contains(err.Error(), "outside the active workspace") {
		t.Fatalf("outside path should be rejected, got %v", err)
	}
	if _, _, err := backend.resolvePath(".", false); err == nil {
		t.Fatal("file operations must not resolve the workspace root as a file")
	}
}

func TestAgentFilesystemBackendRejectsSymlinkEscapeDuringValidation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink permissions vary on Windows")
	}
	workspace := t.TempDir()
	outsideDir := t.TempDir()
	outside := filepath.Join(outsideDir, "outside.txt")
	if err := os.WriteFile(outside, []byte("outside"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(workspace, "escape.txt")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	backend := newTestAgentFilesystemBackend(t, workspace)
	if _, _, _, err := backend.validateExistingPath(link, false); err == nil {
		t.Fatal("os.Root validation must reject a symlink outside the workspace")
	}
}

func newTestAgentFilesystemBackend(t *testing.T, workspaces ...string) *agentFilesystemBackend {
	t.Helper()
	workspace := ""
	if len(workspaces) > 0 {
		workspace = workspaces[0]
	}
	if strings.TrimSpace(workspace) == "" {
		workspace = t.TempDir()
	}
	backend, err := newAgentFilesystemBackend(workspace)
	if err != nil {
		t.Fatal(err)
	}
	return backend
}
