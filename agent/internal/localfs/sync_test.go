package localfs

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestSyncDirectory(t *testing.T) {
	if err := SyncDirectory(t.TempDir()); err != nil {
		t.Fatalf("sync directory: %v", err)
	}
}

func TestSyncDirectoryReportsMissingPath(t *testing.T) {
	err := SyncDirectory(filepath.Join(t.TempDir(), "missing"))
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing directory error = %v, want os.ErrNotExist", err)
	}
}
