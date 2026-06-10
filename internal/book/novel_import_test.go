package book

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPreviewNovelImportSplitsMarkdownAndChineseChapters(t *testing.T) {
	data := []byte(`# 序章

开场。

第一章 风起

第一章正文。

## 第二章 雨落

第二章正文。`)

	preview, err := PreviewNovelImport("长夜.md", data)
	if err != nil {
		t.Fatalf("PreviewNovelImport failed: %v", err)
	}
	if preview.Title != "长夜" {
		t.Fatalf("title = %q", preview.Title)
	}
	if preview.ChapterCount != 3 {
		t.Fatalf("chapter count = %d, chapters=%#v", preview.ChapterCount, preview.Chapters)
	}
	if preview.Chapters[0].Title != "序章" || preview.Chapters[1].Title != "第一章 风起" || preview.Chapters[2].Title != "第二章 雨落" {
		t.Fatalf("unexpected chapter titles: %#v", preview.Chapters)
	}
}

func TestImportNovelToWorkspaceWritesChapters(t *testing.T) {
	dir := t.TempDir()
	if err := NewState(dir).InitWorkspace(); err != nil {
		t.Fatalf("InitWorkspace failed: %v", err)
	}
	data := []byte("第一章 开始\n\n内容一\n\n第二章 继续\n\n内容二")

	preview, paths, err := ImportNovelToWorkspace(dir, "测试.txt", data)
	if err != nil {
		t.Fatalf("ImportNovelToWorkspace failed: %v", err)
	}
	if preview.ChapterCount != 2 || len(paths) != 2 {
		t.Fatalf("unexpected import result preview=%#v paths=%#v", preview, paths)
	}
	for _, rel := range paths {
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(rel))); err != nil {
			t.Fatalf("missing imported chapter %s: %v", rel, err)
		}
	}
}

func TestPreviewNovelImportRejectsUnsupportedFiles(t *testing.T) {
	if _, err := PreviewNovelImport("novel.pdf", []byte("正文")); err == nil {
		t.Fatalf("expected unsupported file error")
	}
	if _, err := PreviewNovelImport("novel.txt", []byte{0xff, 0xfe}); err == nil {
		t.Fatalf("expected utf-8 error")
	}
}
