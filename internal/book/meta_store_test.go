package book

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMetaStoreWriteAndRead(t *testing.T) {
	root := t.TempDir()
	bookDir := filepath.Join(root, "book")
	if err := os.MkdirAll(bookDir, 0o755); err != nil {
		t.Fatal(err)
	}

	store := NewMetaStore(filepath.Join(root, "denova"))
	written, err := store.Write(bookDir, BookMeta{
		Title: "Test Book", Author: "Author", Description: "Description",
	})
	if err != nil {
		t.Fatal(err)
	}
	if written.CreatedAt == "" || written.UpdatedAt == "" {
		t.Fatalf("timestamps were not populated: %#v", written)
	}
	if _, err := os.Stat(store.metaPath(bookDir)); err != nil {
		t.Fatalf("metadata file is unavailable: %v", err)
	}

	read, err := store.Read(bookDir)
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
	if _, err := store.Write(bookDir, BookMeta{Title: "Current", Author: "Current Author"}); err != nil {
		t.Fatal(err)
	}
	read, err := store.Read(bookDir)
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

	read, err := NewMetaStore(filepath.Join(root, "denova")).Read(bookDir)
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

	read, err := NewMetaStore(filepath.Join(root, "denova")).Read(bookDir)
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
	first, err := store.Write(bookDir, BookMeta{Title: "First"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.Write(bookDir, BookMeta{Title: "Second"})
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
