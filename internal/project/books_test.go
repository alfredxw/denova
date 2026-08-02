package project

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRegistryBooksReturnsOnlyAvailableBooks(t *testing.T) {
	dataDir := t.TempDir()
	bookPath := filepath.Join(t.TempDir(), "book")
	generalPath := filepath.Join(t.TempDir(), "general")
	missingPath := filepath.Join(t.TempDir(), "missing")
	for _, path := range []string{bookPath, generalPath, missingPath} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	registry := NewRegistry(dataDir)
	book, err := registry.Add(bookPath, TypeBook, "Book")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Add(generalPath, TypeGeneral, "General"); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Add(missingPath, TypeBook, "Missing"); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(missingPath); err != nil {
		t.Fatal(err)
	}

	books, err := registry.Books()
	if err != nil {
		t.Fatal(err)
	}
	if len(books) != 1 || books[0].ID != book.ID {
		t.Fatalf("available Books = %#v, want only %s", books, book.ID)
	}
}

func TestRegistryReorderBooksPreservesGeneralProjectSlots(t *testing.T) {
	dataDir := t.TempDir()
	registry := NewRegistry(dataDir)
	bookAPath := filepath.Join(t.TempDir(), "a-book")
	generalPath := filepath.Join(t.TempDir(), "b-general")
	bookBPath := filepath.Join(t.TempDir(), "c-book")
	for _, path := range []string{bookAPath, generalPath, bookBPath} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	bookA, err := registry.Add(bookAPath, TypeBook, "A book")
	if err != nil {
		t.Fatal(err)
	}
	general, err := registry.Add(generalPath, TypeGeneral, "B general")
	if err != nil {
		t.Fatal(err)
	}
	bookB, err := registry.Add(bookBPath, TypeBook, "C book")
	if err != nil {
		t.Fatal(err)
	}

	if err := registry.ReorderBooks([]string{bookBPath, bookAPath}); err != nil {
		t.Fatal(err)
	}
	ordered, err := registry.List(false)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{bookB.ID, general.ID, bookA.ID}
	if len(ordered) != len(want) {
		t.Fatalf("ordered Projects = %#v", ordered)
	}
	for index, id := range want {
		if ordered[index].ID != id {
			t.Fatalf("ordered Projects = %#v, want IDs %#v", ordered, want)
		}
	}
	if registry.SortMode() != SortManual {
		t.Fatalf("sort mode = %s, want %s", registry.SortMode(), SortManual)
	}
}

func TestBookCreationParentUsesProjectContentDirectoryForDataRoot(t *testing.T) {
	dataDir := t.TempDir()
	parent, err := BookCreationParent(dataDir, dataDir)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(dataDir, ContentDirectoryName); parent != want {
		t.Fatalf("Book parent = %s, want %s", parent, want)
	}

	customParent := filepath.Join(t.TempDir(), "custom")
	parent, err = BookCreationParent(customParent, dataDir)
	if err != nil {
		t.Fatal(err)
	}
	if parent != customParent {
		t.Fatalf("custom Book parent = %s, want %s", parent, customParent)
	}
}
