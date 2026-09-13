package project

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCreateDirectoryProjectIsPortableAndNeverOverwrites(t *testing.T) {
	root := t.TempDir()
	registry := NewRegistry(root)
	record, err := registry.CreateDirectory(CreateDirectoryRequest{Name: "Garden"})
	if err != nil {
		t.Fatal(err)
	}
	if record.Type != TypeGeneral || record.Location.Kind != LocationManaged || record.Location.Path != "projects/Garden" {
		t.Fatalf("unexpected project: %#v", record)
	}
	file := filepath.Join(record.WorkspacePath, "notes.md")
	if err := os.WriteFile(file, []byte("Keep this source"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.CreateDirectory(CreateDirectoryRequest{Name: "Garden"}); err == nil {
		t.Fatal("reused an existing directory")
	}
	if content, err := os.ReadFile(file); err != nil || string(content) != "Keep this source" {
		t.Fatalf("source changed: %s %v", content, err)
	}
	if _, err := registry.CreateDirectory(CreateDirectoryRequest{Name: "../escape"}); err == nil {
		t.Fatal("accepted traversal")
	}
	moved := filepath.Join(t.TempDir(), "moved")
	if err := os.Rename(root, moved); err != nil {
		t.Fatal(err)
	}
	loaded, _, err := NewRegistry(moved).Resolve(record.ID, true)
	if err != nil || loaded.StoreDirName != record.StoreDirName || loaded.Location != record.Location {
		t.Fatalf("project identity changed after relocation: %#v %v", loaded, err)
	}
}
