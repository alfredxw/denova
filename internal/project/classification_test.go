package project

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectTypeUsesBookWorkspaceMarkers(t *testing.T) {
	general := filepath.Join(t.TempDir(), "code-project")
	book := filepath.Join(t.TempDir(), "novel-project")
	if err := os.MkdirAll(general, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(book, "chapters"), 0o755); err != nil {
		t.Fatal(err)
	}

	if got, err := DetectType(general); err != nil || got != TypeGeneral {
		t.Fatalf("general detection = %q, %v", got, err)
	}
	if got, err := DetectType(book); err != nil || got != TypeBook {
		t.Fatalf("book detection = %q, %v", got, err)
	}
}
