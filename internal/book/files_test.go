package book

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestSafePath(t *testing.T) {
	workspace := t.TempDir()

	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{name: "普通相对路径", path: "chapters/ch01.md"},
		{name: "拒绝绝对路径", path: filepath.Join(workspace, "chapters/ch01.md"), wantErr: true},
		{name: "拒绝越界路径", path: "../outside.md", wantErr: true},
		{name: "拒绝隐藏目录", path: ".nova/session.jsonl", wantErr: true},
		{name: "拒绝隐藏文件", path: "chapters/.secret", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := SafePath(workspace, tt.path)
			if tt.wantErr && err == nil {
				t.Fatalf("期望返回错误")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("不期望返回错误: %v", err)
			}
		})
	}
}

func TestServiceRename(t *testing.T) {
	workspace := t.TempDir()
	service := NewService(workspace)
	if err := service.Create("chapters/ch01.md", "file", "hello"); err != nil {
		t.Fatalf("创建文件失败: %v", err)
	}

	newPath, err := service.Rename("chapters/ch01.md", "ch01-new.md")
	if err != nil {
		t.Fatalf("重命名失败: %v", err)
	}
	if newPath != "chapters/ch01-new.md" {
		t.Fatalf("新路径不符合预期: %s", newPath)
	}

	if _, err := service.Rename("chapters/ch01-new.md", "nested/name.md"); err == nil {
		t.Fatalf("包含路径分隔符的新名称应被拒绝")
	}
}

func TestBuildFileTreeSkipsHiddenFiles(t *testing.T) {
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, ".nova"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(workspace, "chapters"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, ".hidden"), []byte("hidden"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "chapters", "ch01.md"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	tree, err := BuildFileTree(workspace)
	if err != nil {
		t.Fatalf("构建文件树失败: %v", err)
	}
	if len(tree) != 1 || tree[0].Name != "chapters" {
		t.Fatalf("文件树应只包含 chapters，实际: %#v", tree)
	}
	if len(tree[0].Children) != 1 || tree[0].Children[0].Name != "ch01.md" {
		t.Fatalf("chapters 子节点不符合预期: %#v", tree[0].Children)
	}
}

func TestBuildFileTreeSortsDirsBeforeFiles(t *testing.T) {
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, "z_dir"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(workspace, "a_dir"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "b.md"), []byte("b"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "a.md"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}

	tree, err := BuildFileTree(workspace)
	if err != nil {
		t.Fatalf("构建文件树失败: %v", err)
	}
	got := []string{tree[0].Name, tree[1].Name, tree[2].Name, tree[3].Name}
	want := []string{"a_dir", "z_dir", "a.md", "b.md"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("排序不符合预期: want=%v got=%v", want, got)
	}
}

func TestBuildFileTreeSortsChineseChapterOrdinals(t *testing.T) {
	workspace := t.TempDir()
	chapterDir := filepath.Join(workspace, "chapters")
	if err := os.MkdirAll(chapterDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		"第十一章-潮声.md",
		"第一百一十一章-归途.md",
		"第一章-开局.md",
		"第一千一百一十一章-终局.md",
		"序章.md",
		"第十章-交锋.md",
	} {
		if err := os.WriteFile(filepath.Join(chapterDir, name), []byte("正文"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	tree, err := BuildFileTree(workspace)
	if err != nil {
		t.Fatalf("构建文件树失败: %v", err)
	}
	if len(tree) != 1 || tree[0].Name != "chapters" {
		t.Fatalf("文件树根节点不符合预期: %#v", tree)
	}
	got := make([]string, 0, len(tree[0].Children))
	for _, node := range tree[0].Children {
		got = append(got, node.Name)
	}
	want := []string{
		"序章.md",
		"第一章-开局.md",
		"第十章-交锋.md",
		"第十一章-潮声.md",
		"第一百一十一章-归途.md",
		"第一千一百一十一章-终局.md",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("章节文件排序不符合预期: want=%v got=%v", want, got)
	}
}

func TestBuildFileTreeSortsChineseVolumeOrdinals(t *testing.T) {
	workspace := t.TempDir()
	chapterDir := filepath.Join(workspace, "chapters")
	for _, name := range []string{"第十一卷-潮声", "第一百一十一卷-归途", "第一卷-开局", "第十卷-交锋"} {
		if err := os.MkdirAll(filepath.Join(chapterDir, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	tree, err := BuildFileTree(workspace)
	if err != nil {
		t.Fatalf("构建文件树失败: %v", err)
	}
	if len(tree) != 1 || tree[0].Name != "chapters" {
		t.Fatalf("文件树根节点不符合预期: %#v", tree)
	}
	got := make([]string, 0, len(tree[0].Children))
	for _, node := range tree[0].Children {
		got = append(got, node.Name)
	}
	want := []string{"第一卷-开局", "第十卷-交锋", "第十一卷-潮声", "第一百一十一卷-归途"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("分卷目录排序不符合预期: want=%v got=%v", want, got)
	}
}

func TestServiceCreateExisting(t *testing.T) {
	service := NewService(t.TempDir())
	if err := service.Create("chapters/ch01.md", "file", "hello"); err != nil {
		t.Fatalf("创建文件失败: %v", err)
	}
	if err := service.Create("chapters/ch01.md", "file", "hello"); !errors.Is(err, os.ErrExist) {
		t.Fatalf("重复创建应返回 os.ErrExist，实际: %v", err)
	}
}
