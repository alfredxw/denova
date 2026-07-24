package fsdurability

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestSyncDirectory(t *testing.T) {
	directory := t.TempDir()
	if err := SyncDirectory(directory); err != nil {
		t.Fatalf("sync directory: %v", err)
	}
}

func TestSyncRootDirectory(t *testing.T) {
	directory := t.TempDir()
	nested := filepath.Join(directory, "nested")
	if err := os.Mkdir(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()

	if err := SyncRootDirectory(root, "nested"); err != nil {
		t.Fatalf("sync root directory: %v", err)
	}
}

func TestSyncDirectoryReportsMissingPath(t *testing.T) {
	err := SyncDirectory(filepath.Join(t.TempDir(), "missing"))
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing directory error = %v, want os.ErrNotExist", err)
	}
}

func TestSyncRootDirectoryReportsMissingPath(t *testing.T) {
	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()

	err = SyncRootDirectory(root, "missing")
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing root directory error = %v, want os.ErrNotExist", err)
	}
}
