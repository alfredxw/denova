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

func TestChapterDisplayTitleAndIndexSupportMultipleFilenameStyles(t *testing.T) {
	tests := []struct {
		name        string
		wantIndex   int
		wantDisplay string
	}{
		{name: "ch0001-开局.md", wantIndex: 1, wantDisplay: "0001 开局"},
		{name: "001-开局.md", wantIndex: 1, wantDisplay: "001 开局"},
		{name: "第一章-缘起.md", wantIndex: 1, wantDisplay: "第一章 缘起"},
		{name: "第12章-归来.md", wantIndex: 12, wantDisplay: "第12章 归来"},
		{name: "Chapter-2-Flight.md", wantIndex: 2, wantDisplay: "Chapter 2 Flight"},
		{name: "Chapter XII Return.md", wantIndex: 12, wantDisplay: "Chapter XII Return"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := chapterIndex(tt.name); got != tt.wantIndex {
				t.Fatalf("chapterIndex(%q) = %d, want %d", tt.name, got, tt.wantIndex)
			}
			if got := chapterDisplayTitle(tt.name); got != tt.wantDisplay {
				t.Fatalf("chapterDisplayTitle(%q) = %q, want %q", tt.name, got, tt.wantDisplay)
			}
		})
	}
}
