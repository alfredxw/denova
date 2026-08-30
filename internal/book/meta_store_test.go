package book

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestMetaStoreMigratesForeignRootAndSurvivesDataRootMove(t *testing.T) {
	parent := t.TempDir()
	firstRoot := filepath.Join(parent, "first")
	secondRoot := filepath.Join(parent, "second")
	contentRoot := filepath.Join(firstRoot, "projects", "portable-book")
	stateRoot := filepath.Join(firstRoot, "project-state", "Portable-Book")
	if err := os.MkdirAll(filepath.Join(firstRoot, "book_meta"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(contentRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	legacy := StoredMeta{
		Path:     `C:\old-root\Projects\Portable-Book`,
		BookMeta: BookMeta{Title: "Portable", Author: "Writer"},
	}
	encoded, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(firstRoot, "book_meta", "legacy.json"), encoded, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := NewMetaStore(firstRoot).MigrateLegacy(contentRoot, stateRoot); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(firstRoot, secondRoot); err != nil {
		t.Fatal(err)
	}
	read, err := NewMetaStore(secondRoot).Read(
		filepath.Join(secondRoot, "projects", "portable-book"),
		filepath.Join(secondRoot, "project-state", "Portable-Book"),
	)
	if err != nil {
		t.Fatal(err)
	}
	if read.Title != "Portable" || read.Author != "Writer" {
		t.Fatalf("relocated metadata = %#v", read)
	}
	finalData, err := os.ReadFile(filepath.Join(secondRoot, "project-state", "Portable-Book", "meta.json"))
	if err != nil {
		t.Fatal(err)
	}
	var final map[string]any
	if err := json.Unmarshal(finalData, &final); err != nil {
		t.Fatal(err)
	}
	if _, exists := final["path"]; exists {
		t.Fatalf("final metadata retained an absolute owner path: %s", finalData)
	}
}

func TestMetaStoreWriteAndRead(t *testing.T) {
	root := t.TempDir()
	bookDir := filepath.Join(root, "book")
	if err := os.MkdirAll(bookDir, 0o755); err != nil {
		t.Fatal(err)
	}

	store := NewMetaStore(filepath.Join(root, "denova"))
	stateRoot := filepath.Join(root, "denova", "project-state", "book")
	written, err := store.Write(bookDir, stateRoot, BookMeta{
		Title: "Test Book", Author: "Author", Description: "Description",
	})
	if err != nil {
		t.Fatal(err)
	}
	if written.CreatedAt == "" || written.UpdatedAt == "" {
		t.Fatalf("timestamps were not populated: %#v", written)
	}
	if _, err := os.Stat(store.metaPath(stateRoot)); err != nil {
		t.Fatalf("metadata file is unavailable: %v", err)
	}

	read, err := store.Read(bookDir, stateRoot)
	if err != nil {
		t.Fatal(err)
	}
	if read.Title != written.Title || read.Author != written.Author || read.Description != written.Description {
		t.Fatalf("metadata round trip = %#v, want %#v", read, written)
	}
}

func TestMetaStorePrefersUserDataOverLegacyBookJSON(t *testing.T) {
	root := t.TempDir()
	bookDir := filepath.Join(root, "book")
	if err := os.MkdirAll(bookDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := NewState(bookDir).WriteBookMeta(BookMeta{Title: "Legacy", Author: "Legacy Author"}); err != nil {
		t.Fatal(err)
	}

	store := NewMetaStore(filepath.Join(root, "denova"))
	stateRoot := filepath.Join(root, "denova", "project-state", "book")
	if _, err := store.Write(bookDir, stateRoot, BookMeta{Title: "Current", Author: "Current Author"}); err != nil {
		t.Fatal(err)
	}
	read, err := store.Read(bookDir, stateRoot)
	if err != nil {
		t.Fatal(err)
	}
	if read.Title != "Current" || read.Author != "Current Author" {
		t.Fatalf("user-data metadata did not take precedence: %#v", read)
	}
}

func TestMetaStoreReadsLegacyBookJSON(t *testing.T) {
	root := t.TempDir()
	bookDir := filepath.Join(root, "book")
	if err := os.MkdirAll(bookDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := NewState(bookDir).WriteBookMeta(BookMeta{Title: "Legacy", Author: "Legacy Author"}); err != nil {
		t.Fatal(err)
	}

	read, err := NewMetaStore(filepath.Join(root, "denova")).Read(bookDir, filepath.Join(root, "state"))
	if err != nil {
		t.Fatal(err)
	}
	if read.Title != "Legacy" || read.Author != "Legacy Author" {
		t.Fatalf("legacy metadata was not loaded: %#v", read)
	}
}

func TestMetaStoreDefaultsTitleToDirectoryName(t *testing.T) {
	root := t.TempDir()
	bookDir := filepath.Join(root, "book-name")
	if err := os.MkdirAll(bookDir, 0o755); err != nil {
		t.Fatal(err)
	}

	read, err := NewMetaStore(filepath.Join(root, "denova")).Read(bookDir, filepath.Join(root, "state"))
	if err != nil {
		t.Fatal(err)
	}
	if read.Title != "book-name" {
		t.Fatalf("default title = %q, want book-name", read.Title)
	}
}

func TestMetaStorePreservesCreatedAtOnUpdate(t *testing.T) {
	root := t.TempDir()
	bookDir := filepath.Join(root, "book")
	if err := os.MkdirAll(bookDir, 0o755); err != nil {
		t.Fatal(err)
	}

	store := NewMetaStore(filepath.Join(root, "denova"))
	stateRoot := filepath.Join(root, "denova", "project-state", "book")
	first, err := store.Write(bookDir, stateRoot, BookMeta{Title: "First"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.Write(bookDir, stateRoot, BookMeta{Title: "Second"})
	if err != nil {
		t.Fatal(err)
	}
	if second.CreatedAt != first.CreatedAt {
		t.Fatalf("CreatedAt changed: first=%s second=%s", first.CreatedAt, second.CreatedAt)
	}
	if second.UpdatedAt == "" {
		t.Fatalf("UpdatedAt was not populated: %#v", second)
	}
}
