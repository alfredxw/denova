package book

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestReplacerLiteralIsCaseInsensitive(t *testing.T) {
	replacer, err := NewReplacer("hello", "hi", SearchOptions{})
	if err != nil {
		t.Fatal(err)
	}
	got, count := replacer.ReplaceAll("Hello HELLO hello world")
	if count != 3 || got != "hi hi hi world" {
		t.Fatalf("字面量替换应大小写不敏感替换三处，实际 count=%d content=%q", count, got)
	}
}

func TestReplacerLiteralMatchesSubstring(t *testing.T) {
	replacer, err := NewReplacer("林川", "陆离", SearchOptions{})
	if err != nil {
		t.Fatal(err)
	}
	got, count := replacer.ReplaceAll("林川遇见了小林川，林川点头。")
	if count != 3 || got != "陆离遇见了小陆离，陆离点头。" {
		t.Fatalf("CJK 字面量替换按子串匹配，实际 count=%d content=%q", count, got)
	}
}

func TestReplacerRegexSupportsCaptureGroups(t *testing.T) {
	replacer, err := NewReplacer(`(林川)和(韩月)`, "$2与$1", SearchOptions{Regex: true})
	if err != nil {
		t.Fatal(err)
	}
	got, count := replacer.ReplaceAll("林川和韩月进了城，林川和韩月又出城。")
	if count != 2 || got != "韩月与林川进了城，韩月与林川又出城。" {
		t.Fatalf("正则捕获组替换不符合预期，实际 count=%d content=%q", count, got)
	}
}

func TestReplacerReplacementTemplateEdgeCases(t *testing.T) {
	replacer, err := NewReplacer(`(\w+)@example\.com`, "$1 [at] cost $5 $$ $9", SearchOptions{Regex: true})
	if err != nil {
		t.Fatal(err)
	}
	got, count := replacer.ReplaceAll("contact bob@example.com now")
	if count != 1 || got != "contact bob [at] cost $5 $ $9 now" {
		t.Fatalf("替换模板边界情况不符合预期，实际 count=%d content=%q", count, got)
	}
}

func TestReplacerRegexIsCaseSensitive(t *testing.T) {
	replacer, err := NewReplacer("abc", "x", SearchOptions{Regex: true})
	if err != nil {
		t.Fatal(err)
	}
	got, count := replacer.ReplaceAll("ABC abc")
	if count != 1 || got != "ABC x" {
		t.Fatalf("正则替换应大小写敏感，实际 count=%d content=%q", count, got)
	}
}

func TestReplacerRegexSpanningMultipleLines(t *testing.T) {
	replacer, err := NewReplacer(`\d+`, "N", SearchOptions{Regex: true})
	if err != nil {
		t.Fatal(err)
	}
	got, count := replacer.ReplaceAll("第1章\n第23章\n无数字\n")
	if count != 2 || got != "第N章\n第N章\n无数字\n" {
		t.Fatalf("跨行正则替换不符合预期，实际 count=%d content=%q", count, got)
	}
}

func TestReplacerNoMatchReturnsOriginal(t *testing.T) {
	replacer, err := NewReplacer("不存在", "x", SearchOptions{})
	if err != nil {
		t.Fatal(err)
	}
	content := "正文内容"
	got, count := replacer.ReplaceAll(content)
	if count != 0 || got != content {
		t.Fatalf("无匹配时应原样返回，实际 count=%d content=%q", count, got)
	}
}

func TestNewReplacerRejectsInvalidRegex(t *testing.T) {
	_, err := NewReplacer("(未闭合", "x", SearchOptions{Regex: true})
	if !errors.Is(err, ErrInvalidSearchRegex) {
		t.Fatalf("非法正则应返回 ErrInvalidSearchRegex，实际: %v", err)
	}
}

func TestNewReplacerRejectsEmptyMatchingRegex(t *testing.T) {
	for _, pattern := range []string{"a*", `(?:x)?`, "^"} {
		if _, err := NewReplacer(pattern, "x", SearchOptions{Regex: true}); !errors.Is(err, ErrRegexMatchesEmpty) {
			t.Fatalf("模式 %q 可匹配空字符串，应返回 ErrRegexMatchesEmpty，实际: %v", pattern, err)
		}
	}
}

func TestListReplaceCandidateFilesFiltersLikeSearch(t *testing.T) {
	workspace := t.TempDir()
	service := NewService(workspace)
	if err := service.Create("chapters/ch01.md", "file", "正文"); err != nil {
		t.Fatal(err)
	}
	if err := service.Create("notes.txt", "file", "笔记"); err != nil {
		t.Fatal(err)
	}
	if err := service.Create("cover.png", "file", "binary"); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(workspace, ".hidden"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, ".hidden", "secret.md"), []byte("秘密"), 0o644); err != nil {
		t.Fatal(err)
	}

	paths, err := ListReplaceCandidateFiles(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 2 || paths[0] != "chapters/ch01.md" || paths[1] != "notes.txt" {
		t.Fatalf("候选文件应只包含可见文本文件，实际: %#v", paths)
	}
}
