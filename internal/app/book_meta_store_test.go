package app

import (
	"os"
	"path/filepath"
	"testing"

	"denova/internal/book"
)

func TestBookMetaStoreWriteAndReadFromNovaDir(t *testing.T) {
	root := t.TempDir()
	bookDir := filepath.Join(root, "book")
	if err := os.MkdirAll(bookDir, 0o755); err != nil {
		t.Fatal(err)
	}

	store := NewBookMetaStore(filepath.Join(root, "nova"))
	meta, err := store.Write(bookDir, book.BookMeta{
		Title:       "测试书",
		Author:      "作者",
		Description: "简介",
	})
	if err != nil {
		t.Fatalf("写入书籍元信息失败: %v", err)
	}
	if meta.CreatedAt == "" || meta.UpdatedAt == "" {
		t.Fatalf("时间字段未自动填充: %#v", meta)
	}
	if _, err := os.Stat(store.metaPath(bookDir)); err != nil {
		t.Fatalf("用户目录元信息文件不存在: %v", err)
	}

	got, err := store.Read(bookDir)
	if err != nil {
		t.Fatalf("读取书籍元信息失败: %v", err)
	}
	if got.Title != "测试书" || got.Author != "作者" || got.Description != "简介" {
		t.Fatalf("读取结果不符合预期: %#v", got)
	}
}

func TestBookMetaStorePrefersNovaDirOverLegacyBookJSON(t *testing.T) {
	root := t.TempDir()
	bookDir := filepath.Join(root, "book")
	if err := os.MkdirAll(bookDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := book.NewState(bookDir).WriteBookMeta(book.BookMeta{Title: "旧标题", Author: "旧作者"}); err != nil {
		t.Fatal(err)
	}

	store := NewBookMetaStore(filepath.Join(root, "nova"))
	if _, err := store.Write(bookDir, book.BookMeta{Title: "新标题", Author: "新作者"}); err != nil {
		t.Fatal(err)
	}
	got, err := store.Read(bookDir)
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "新标题" || got.Author != "新作者" {
		t.Fatalf("未优先读取用户目录元信息: %#v", got)
	}
}

func TestBookMetaStoreReadsLegacyBookJSON(t *testing.T) {
	root := t.TempDir()
	bookDir := filepath.Join(root, "book")
	if err := os.MkdirAll(bookDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := book.NewState(bookDir).WriteBookMeta(book.BookMeta{Title: "旧书", Author: "旧作者"}); err != nil {
		t.Fatal(err)
	}

	store := NewBookMetaStore(filepath.Join(root, "nova"))
	got, err := store.Read(bookDir)
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "旧书" || got.Author != "旧作者" {
		t.Fatalf("未兼容读取旧 book.json: %#v", got)
	}
}

func TestBookMetaStoreDefaultsToDirectoryName(t *testing.T) {
	root := t.TempDir()
	bookDir := filepath.Join(root, "book-name")
	if err := os.MkdirAll(bookDir, 0o755); err != nil {
		t.Fatal(err)
	}

	store := NewBookMetaStore(filepath.Join(root, "nova"))
	got, err := store.Read(bookDir)
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "book-name" {
		t.Fatalf("默认标题不符合预期: %#v", got)
	}
}

func TestBookMetaStorePreservesCreatedAtOnUpdate(t *testing.T) {
	root := t.TempDir()
	bookDir := filepath.Join(root, "book")
	if err := os.MkdirAll(bookDir, 0o755); err != nil {
		t.Fatal(err)
	}

	store := NewBookMetaStore(filepath.Join(root, "nova"))
	first, err := store.Write(bookDir, book.BookMeta{Title: "初版"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.Write(bookDir, book.BookMeta{Title: "新版"})
	if err != nil {
		t.Fatal(err)
	}
	if second.CreatedAt != first.CreatedAt {
		t.Fatalf("CreatedAt 应保持不变: first=%s second=%s", first.CreatedAt, second.CreatedAt)
	}
	if second.UpdatedAt == "" {
		t.Fatalf("UpdatedAt 未填充: %#v", second)
	}
}
