package book

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestServiceExportTextAssemblesReadableManuscript(t *testing.T) {
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

	result, err := NewService(root).ExportText(BookMeta{Title: "星河边境", Author: "Denova"})
	if err != nil {
		t.Fatal(err)
	}

	if result.ChapterCount != 2 {
		t.Fatalf("chapter count = %d", result.ChapterCount)
	}
	wantOrder := []string{
		"星河边境",
		"作者: Denova",
		"第一卷 风起",
		"第一章 开局",
		"天亮了。",
		"第二章 追光",
		"林川踏入雨夜。",
	}
	last := -1
	for _, item := range wantOrder {
		index := strings.Index(result.Content, item)
		if index == -1 {
			t.Fatalf("export missing %q:\n%s", item, result.Content)
		}
		if index <= last {
			t.Fatalf("export order mismatch around %q:\n%s", item, result.Content)
		}
		last = index
	}
	if strings.Contains(result.Content, "# 第二章 追光") || strings.Contains(result.Content, "第三章 空章") {
		t.Fatalf("export should remove duplicate headings and skip empty chapters:\n%s", result.Content)
	}
}

func TestServiceExportTextRequiresNonEmptyChapters(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "chapters"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "chapters", "ch00001-空章.md"), nil, 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := NewService(root).ExportText(BookMeta{Title: "空书"})
	if !errors.Is(err, ErrNoExportableChapters) {
		t.Fatalf("err = %v, want ErrNoExportableChapters", err)
	}
}
