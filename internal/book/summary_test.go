package book

import (
	"os"
	"path/filepath"
	"testing"
)

func TestServiceSummaryCountsChapters(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "chapters"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "chapters", "ch02-第二章.md"), []byte("第二章\n\n三个人出发。"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "chapters", "ch01-开局.md"), []byte("第一章\n\n天亮了。"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "book.json"), []byte(`{"title":"无限狩猎","author":"Nova"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	summary, err := NewService(root).Summary()
	if err != nil {
		t.Fatal(err)
	}

	if summary.Title != "无限狩猎" {
		t.Fatalf("title = %q", summary.Title)
	}
	if summary.ChapterCount != 2 {
		t.Fatalf("chapter count = %d", summary.ChapterCount)
	}
	if summary.Chapters[0].Path != "chapters/ch01-开局.md" {
		t.Fatalf("first chapter = %q", summary.Chapters[0].Path)
	}
	if summary.Chapters[0].DisplayTitle != "01 开局" {
		t.Fatalf("display title = %q", summary.Chapters[0].DisplayTitle)
	}
	if summary.TotalWords == 0 {
		t.Fatal("expected non-zero total words")
	}
}
