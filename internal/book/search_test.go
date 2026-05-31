package book

import (
	"os"
	"path/filepath"
	"testing"
)

func TestServiceSearchFindsTextAndSkipsHidden(t *testing.T) {
	workspace := t.TempDir()
	service := NewService(workspace)
	if err := service.Create("chapters/ch01.md", "file", "第一章\n林川点燃火把\n"); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(workspace, ".nova"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, ".nova", "secret.md"), []byte("林川隐藏记录"), 0o644); err != nil {
		t.Fatal(err)
	}

	results, err := service.Search("林川", 100)
	if err != nil {
		t.Fatalf("搜索失败: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("应只命中可见正文，实际: %#v", results)
	}
	if results[0].Path != "chapters/ch01.md" || results[0].Line != 2 || results[0].Column != 1 {
		t.Fatalf("搜索结果位置不符合预期: %#v", results[0])
	}
}

func TestServiceSearchFindsPathMatch(t *testing.T) {
	service := NewService(t.TempDir())
	if err := service.Create("setting/characters.md", "file", "角色设定"); err != nil {
		t.Fatal(err)
	}

	results, err := service.Search("characters", 100)
	if err != nil {
		t.Fatalf("搜索失败: %v", err)
	}
	if len(results) == 0 || results[0].Path != "setting/characters.md" || results[0].Line != 0 {
		t.Fatalf("应返回路径匹配结果: %#v", results)
	}
}
