package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestBookRegistryTouchListAndCurrent(t *testing.T) {
	root := t.TempDir()
	bookA := filepath.Join(root, "book-a")
	bookB := filepath.Join(root, "book-b")
	if err := os.MkdirAll(bookA, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(bookB, 0o755); err != nil {
		t.Fatal(err)
	}

	registry := &BookRegistry{path: filepath.Join(root, "books.json")}
	if err := registry.Touch(bookA); err != nil {
		t.Fatalf("记录 bookA 失败: %v", err)
	}
	if err := registry.Touch(bookB); err != nil {
		t.Fatalf("记录 bookB 失败: %v", err)
	}

	if got := registry.Current(); got != bookB {
		t.Fatalf("当前书籍不符合预期: want=%s got=%s", bookB, got)
	}
	books := registry.List()
	if len(books) != 2 {
		t.Fatalf("书籍记录数量不符合预期: %d", len(books))
	}
	if books[0].Path != bookB || books[1].Path != bookA {
		t.Fatalf("书籍记录排序不符合预期: %#v", books)
	}
}

func TestBookRegistryListScansNovaDirBooks(t *testing.T) {
	root := t.TempDir()
	bookA := filepath.Join(root, "zeta")
	bookB := filepath.Join(root, "alpha")
	missingBook := filepath.Join(root, "missing")
	for _, dir := range []string{
		filepath.Join(bookA, ".nova"),
		filepath.Join(bookB, "chapters"),
		filepath.Join(root, "book_meta"),
		filepath.Join(root, "styles"),
		filepath.Join(root, "notes"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	registry := &BookRegistry{path: filepath.Join(root, "books.json"), novaDir: root}
	if err := registry.save(bookRegistryData{
		Books: []BookRecord{
			{Path: missingBook, LastOpenedAt: "2026-01-03T00:00:00Z"},
			{Path: bookA, LastOpenedAt: "2026-01-02T00:00:00Z"},
		},
	}); err != nil {
		t.Fatalf("写入注册表失败: %v", err)
	}

	books := registry.List()
	if len(books) != 2 {
		t.Fatalf("书籍数量不符合预期: %#v", books)
	}
	if books[0].Path != bookB || books[1].Path != bookA {
		t.Fatalf("书籍应来自 Nova 目录并按名称排序: %#v", books)
	}
	if books[1].LastOpenedAt != "2026-01-02T00:00:00Z" {
		t.Fatalf("应保留已有打开时间用于兼容展示: %#v", books[1])
	}
}

func TestBookRegistryRemove(t *testing.T) {
	root := t.TempDir()
	bookA := filepath.Join(root, "book-a")
	bookB := filepath.Join(root, "book-b")
	if err := os.MkdirAll(bookA, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(bookB, 0o755); err != nil {
		t.Fatal(err)
	}

	registry := &BookRegistry{path: filepath.Join(root, "books.json")}
	if err := registry.Touch(bookA); err != nil {
		t.Fatal(err)
	}
	if err := registry.Touch(bookB); err != nil {
		t.Fatal(err)
	}
	if err := registry.Remove(bookB); err != nil {
		t.Fatalf("移除记录失败: %v", err)
	}

	if got := registry.Current(); got != bookA {
		t.Fatalf("移除当前书籍后应回退到上一条记录: want=%s got=%s", bookA, got)
	}
	books := registry.List()
	if len(books) != 1 || books[0].Path != bookA {
		t.Fatalf("移除后的书籍列表不符合预期: %#v", books)
	}
}

func TestBookRegistryRemoveHidesScannedNovaBook(t *testing.T) {
	root := t.TempDir()
	bookA := filepath.Join(root, "book-a")
	bookB := filepath.Join(root, "book-b")
	for _, dir := range []string{
		filepath.Join(bookA, ".nova"),
		filepath.Join(bookB, ".nova"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	registry := &BookRegistry{path: filepath.Join(root, "books.json"), novaDir: root}
	if err := registry.Remove(bookB); err != nil {
		t.Fatalf("软删除书籍失败: %v", err)
	}

	books := registry.List()
	if len(books) != 1 || books[0].Path != bookA {
		t.Fatalf("软删除后扫描列表应隐藏目标书籍: %#v", books)
	}
	if _, err := os.Stat(bookB); err != nil {
		t.Fatalf("软删除不应删除磁盘目录: %v", err)
	}
}

func TestBookRegistryReorderScannedNovaBooks(t *testing.T) {
	root := t.TempDir()
	bookA := filepath.Join(root, "alpha")
	bookB := filepath.Join(root, "zeta")
	for _, dir := range []string{
		filepath.Join(bookA, ".nova"),
		filepath.Join(bookB, ".nova"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	registry := &BookRegistry{path: filepath.Join(root, "books.json"), novaDir: root}
	if err := registry.Reorder([]string{bookB, bookA}); err != nil {
		t.Fatalf("保存排序失败: %v", err)
	}

	books := registry.List()
	if len(books) != 2 || books[0].Path != bookB || books[1].Path != bookA {
		t.Fatalf("书籍列表应遵循自定义排序: %#v", books)
	}

	if err := registry.Touch(bookA); err != nil {
		t.Fatalf("打开书籍失败: %v", err)
	}
	books = registry.List()
	if len(books) != 2 || books[0].Path != bookB || books[1].Path != bookA {
		t.Fatalf("打开书籍不应打乱自定义排序: %#v", books)
	}
}

func TestNewBookRegistryUsesNovaDir(t *testing.T) {
	novaDir := t.TempDir()
	registry := NewBookRegistry(novaDir)
	want := filepath.Join(novaDir, "books.json")
	if registry.path != want {
		t.Fatalf("注册表路径不符合预期: want=%s got=%s", want, registry.path)
	}
}

func TestBookRegistryLoadsLegacyPathAndMigratesOnSave(t *testing.T) {
	root := t.TempDir()
	bookDir := filepath.Join(root, "book")
	if err := os.MkdirAll(bookDir, 0o755); err != nil {
		t.Fatal(err)
	}

	legacyPath := filepath.Join(root, "legacy-books.json")
	newPath := filepath.Join(root, "nova", "books.json")
	legacyData := bookRegistryData{
		Current: bookDir,
		Books: []BookRecord{{
			Name:         "旧书",
			Path:         bookDir,
			LastOpenedAt: "2026-01-01T00:00:00Z",
		}},
	}
	raw, err := json.Marshal(legacyData)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacyPath, raw, 0o644); err != nil {
		t.Fatal(err)
	}

	registry := &BookRegistry{path: newPath, legacyPath: legacyPath}
	if got := registry.Current(); got != bookDir {
		t.Fatalf("未能读取旧注册表当前书籍: want=%s got=%s", bookDir, got)
	}
	if err := registry.Touch(bookDir); err != nil {
		t.Fatalf("保存迁移后的注册表失败: %v", err)
	}
	if _, err := os.Stat(newPath); err != nil {
		t.Fatalf("新注册表未写入: %v", err)
	}
}
