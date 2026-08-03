package book

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestServiceManuscriptGroupsChaptersByVolume(t *testing.T) {
	root := t.TempDir()
	chapterDir := filepath.Join(root, "chapters", "v00001-第一卷-风起")
	if err := os.MkdirAll(chapterDir, 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"ch00002-第二章-追光.md": "# 第二章 追光\n\n林川踏入雨夜。",
		"ch00001-第一章-开局.md": "第一章 开局\n\n天亮了。",
		"ch00003-第三章-空章.md": "",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(chapterDir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	manuscript, err := NewService(root).Manuscript(BookMeta{Title: "星河边境", Author: "Denova"})
	if err != nil {
		t.Fatal(err)
	}

	if manuscript.Title != "星河边境" || manuscript.Author != "Denova" {
		t.Fatalf("metadata mismatch: %#v", manuscript)
	}
	if manuscript.ChapterCount() != 2 {
		t.Fatalf("chapter count = %d", manuscript.ChapterCount())
	}
	if len(manuscript.Volumes) != 1 {
		t.Fatalf("volume count = %d, want 1", len(manuscript.Volumes))
	}
	volume := manuscript.Volumes[0]
	if volume.Title != "第一卷 风起" {
		t.Fatalf("volume title = %q", volume.Title)
	}
	if len(volume.Chapters) != 2 {
		t.Fatalf("volume chapter count = %d", len(volume.Chapters))
	}
	if volume.Chapters[0].Title != "第一章 开局" || volume.Chapters[0].Body != "天亮了。" {
		t.Fatalf("first chapter mismatch: %#v", volume.Chapters[0])
	}
	// The duplicate leading heading ("# 第二章 追光") must be stripped from the body.
	if volume.Chapters[1].Title != "第二章 追光" || volume.Chapters[1].Body != "林川踏入雨夜。" {
		t.Fatalf("second chapter mismatch: %#v", volume.Chapters[1])
	}
}

func TestServiceManuscriptKeepsUnnamedVolumeUntitled(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "chapters"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "chapters", "ch00001-开端.md"), []byte("开端\n\n正文。"), 0o644); err != nil {
		t.Fatal(err)
	}

	manuscript, err := NewService(root).Manuscript(BookMeta{Title: "无卷书"})
	if err != nil {
		t.Fatal(err)
	}
	if len(manuscript.Volumes) != 1 {
		t.Fatalf("volume count = %d, want 1", len(manuscript.Volumes))
	}
	if manuscript.Volumes[0].Title != "" {
		t.Fatalf("unnamed volume should have empty title, got %q", manuscript.Volumes[0].Title)
	}
}

func TestServiceManuscriptRequiresNonEmptyChapters(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "chapters"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "chapters", "ch00001-空章.md"), nil, 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := NewService(root).Manuscript(BookMeta{Title: "空书"})
	if !errors.Is(err, ErrNoExportableChapters) {
		t.Fatalf("err = %v, want ErrNoExportableChapters", err)
	}
}
