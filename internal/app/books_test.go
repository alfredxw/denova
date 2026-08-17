package app

import (
	"os"
	"path/filepath"
	"testing"

	"denova/internal/book"
	projectdomain "denova/internal/project"
)

func TestBooksIncludesCoverUpdatedAt(t *testing.T) {
	dataDir := t.TempDir()
	bookDir := filepath.Join(dataDir, "alpha")
	if err := os.MkdirAll(filepath.Join(bookDir, ".denova"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(bookDir, "assets", "image"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bookDir, "assets", "image", "cover.png"), []byte("cover"), 0o644); err != nil {
		t.Fatal(err)
	}
	application := &App{
		projectRegistry: projectdomain.NewRegistry(dataDir),
		bookMetaStore:   book.NewMetaStore(dataDir),
	}

	books := application.BookAssets().Books()
	if len(books) != 1 {
		t.Fatalf("Books = %#v, want one Book", books)
	}
	if books[0].CoverUpdatedAt == "" {
		t.Fatalf("Book cover timestamp is empty: %#v", books[0])
	}
}

func TestUpdateBookInfoCanClearAuthor(t *testing.T) {
	root := t.TempDir()
	bookDir := filepath.Join(root, "book")
	if err := os.MkdirAll(bookDir, 0o755); err != nil {
		t.Fatal(err)
	}
	store := book.NewMetaStore(filepath.Join(root, "denova"))
	if _, err := store.Write(bookDir, book.BookMeta{Title: "Book", Author: "Author"}); err != nil {
		t.Fatal(err)
	}
	application := &App{bookMetaStore: store}

	updated, err := application.BookAssets().UpdateInfo(bookDir, "Book", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if updated.Author != "" {
		t.Fatalf("Author was not cleared: %#v", updated)
	}
}
