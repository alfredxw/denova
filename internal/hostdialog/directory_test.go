package hostdialog

import (
	"path/filepath"
	"testing"
)

func TestNearestExistingDirectoryUsesParentOfMissingProject(t *testing.T) {
	root := t.TempDir()
	missing := filepath.Join(root, "moved", "project")
	if got := nearestExistingDirectory(missing); got != root {
		t.Fatalf("nearestExistingDirectory(%q) = %q, want %q", missing, got, root)
	}
}

func TestNearestExistingDirectoryRejectsRelativePath(t *testing.T) {
	if got := nearestExistingDirectory("relative/project"); got != "" {
		t.Fatalf("relative path should not seed a host dialog: %q", got)
	}
}
