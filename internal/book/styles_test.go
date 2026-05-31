package book

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestUserStyleDir(t *testing.T) {
	workspace := t.TempDir()
	got := UserStyleDir(workspace)
	want := filepath.Join(workspace, "styles")
	if got != want {
		t.Fatalf("用户级风格目录不符合预期: want=%s got=%s", want, got)
	}
}

func TestServiceStyleFiles(t *testing.T) {
	workspace := t.TempDir()
	styleRoot := filepath.Join(t.TempDir(), "styles")
	service := NewServiceWithStyleRoot(workspace, styleRoot)
	mustWriteFile(t, styleRoot, "古龙.md", "短句")
	mustWriteFile(t, styleRoot, "番茄.txt", "快节奏")
	mustWriteFile(t, styleRoot, "武侠/冷峻.md", "冷峻")
	mustWriteFile(t, styleRoot, "ignore.doc", "ignore")
	mustWriteFile(t, styleRoot, ".hidden.md", "hidden")
	mustWriteFile(t, styleRoot, ".secret/secret.md", "hidden")

	files, err := service.StyleFiles()
	if err != nil {
		t.Fatalf("获取风格文件失败: %v", err)
	}

	want := []string{"古龙.md", "武侠/冷峻.md", "番茄.txt"}
	if !reflect.DeepEqual(files, want) {
		t.Fatalf("风格文件不符合预期: want=%v got=%v", want, files)
	}
}

func TestServiceReadStyleFile(t *testing.T) {
	workspace := t.TempDir()
	styleRoot := filepath.Join(t.TempDir(), "styles")
	service := NewServiceWithStyleRoot(workspace, styleRoot)
	mustWriteFile(t, styleRoot, "古龙.md", "短句留白")
	mustWriteFile(t, styleRoot, "番茄.txt", "强冲突快节奏")

	content, err := service.ReadStyleFile("古龙.md")
	if err != nil {
		t.Fatalf("读取合法风格文件失败: %v", err)
	}
	if content != "短句留白" {
		t.Fatalf("读取内容不符合预期: %q", content)
	}
	txtContent, err := service.ReadStyleFile("番茄.txt")
	if err != nil {
		t.Fatalf("读取 TXT 风格文件失败: %v", err)
	}
	if txtContent != "强冲突快节奏" {
		t.Fatalf("读取 TXT 内容不符合预期: %q", txtContent)
	}

	tests := []struct {
		name string
		path string
	}{
		{name: "拒绝越界路径", path: "../outline.md"},
		{name: "拒绝绝对路径", path: filepath.Join(styleRoot, "古龙.md")},
		{name: "拒绝不支持的扩展名", path: "notes.doc"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := service.ReadStyleFile(tt.path); err == nil {
				t.Fatalf("期望读取失败")
			}
		})
	}
}

func mustWriteFile(t *testing.T, workspace, relPath, content string) {
	t.Helper()
	absPath := filepath.Join(workspace, filepath.FromSlash(relPath))
	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(absPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
