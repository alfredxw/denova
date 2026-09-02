package project

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStoreDirNameBaseProducesPortableReadableNames(t *testing.T) {
	tests := map[string]string{
		" My Project ": "My-Project",
		"我的/小说":        "我的-小说",
		"✨":            "Project",
		"CON":          "Project-CON",
		"COM¹":         "Project-COM¹",
		"Cafe\u0301":   "Café",
	}
	for input, want := range tests {
		if got := storeDirNameBase(input); got != want {
			t.Errorf("storeDirNameBase(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestStoreDirNameValidationUsesCaseInsensitiveCollisionKeys(t *testing.T) {
	projects := []Record{
		{ID: "project-one", Type: TypeGeneral, StoreDirName: "Book"},
		{ID: "project-two", Type: TypeGeneral, StoreDirName: "book"},
	}
	if err := validateStoreDirNames(projects); err == nil {
		t.Fatal("case-insensitive Store directory collision was accepted")
	}
}

func TestProjectIDStoreDirectoryMigrationNeverMergesExistingStores(t *testing.T) {
	denovaDir := t.TempDir()
	registry := NewRegistry(denovaDir)
	record := Record{ID: "project-one", Type: TypeGeneral, StoreDirName: "Book"}
	source := filepath.Join(denovaDir, storeDirectoryName, record.ID)
	destination := filepath.Join(denovaDir, storeDirectoryName, record.StoreDirName)
	for _, path := range []string{source, destination} {
		if err := os.MkdirAll(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(source, "source.txt"), []byte("source"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(destination, "destination.txt"), []byte("destination"), 0o600); err != nil {
		t.Fatal(err)
	}

	err := registry.migrateProjectIDStoreDirectories([]Record{record})
	if err == nil || !strings.Contains(err.Error(), "destination already exists") {
		t.Fatalf("Store migration conflict error = %v", err)
	}
	for _, path := range []string{filepath.Join(source, "source.txt"), filepath.Join(destination, "destination.txt")} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("Store migration conflict changed %s: %v", path, err)
		}
	}
}
