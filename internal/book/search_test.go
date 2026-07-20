package book

import (
	"errors"
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

	results, err := service.Search("林川", 100, SearchOptions{})
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

	results, err := service.Search("characters", 100, SearchOptions{})
	if err != nil {
		t.Fatalf("搜索失败: %v", err)
	}
	if len(results) == 0 || results[0].Path != "setting/characters.md" || results[0].Line != 0 {
		t.Fatalf("应返回路径匹配结果: %#v", results)
	}
}

func TestServiceSearchRegexContentAndPath(t *testing.T) {
	service := NewService(t.TempDir())
	if err := service.Create("chapters/ch01.md", "file", "林川点燃火把\n韩月吹灭火把\n"); err != nil {
		t.Fatal(err)
	}

	results, err := service.Search(`林[川山].{2}`, 100, SearchOptions{Regex: true})
	if err != nil {
		t.Fatalf("正则搜索失败: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("正则应只命中一行，实际: %#v", results)
	}
	got := results[0]
	if got.Line != 1 || got.Column != 1 || got.MatchText != "林川点燃" {
		t.Fatalf("正则匹配结果不符合预期: %#v", got)
	}

	pathResults, err := service.Search(`ch\d+`, 100, SearchOptions{Regex: true})
	if err != nil {
		t.Fatalf("路径正则搜索失败: %v", err)
	}
	if len(pathResults) == 0 || pathResults[0].Line != 0 || pathResults[0].MatchText != "ch01" {
		t.Fatalf("路径正则匹配结果不符合预期: %#v", pathResults)
	}
}

func TestServiceSearchRegexIsCaseSensitive(t *testing.T) {
	service := NewService(t.TempDir())
	if err := service.Create("chapters/ch01.md", "file", "ABC abc\n"); err != nil {
		t.Fatal(err)
	}

	literalResults, err := service.Search("abc", 100, SearchOptions{})
	if err != nil {
		t.Fatalf("字面量搜索失败: %v", err)
	}
	if len(literalResults) != 2 {
		t.Fatalf("字面量搜索应大小写不敏感命中两处，实际: %#v", literalResults)
	}

	regexResults, err := service.Search("abc", 100, SearchOptions{Regex: true})
	if err != nil {
		t.Fatalf("正则搜索失败: %v", err)
	}
	if len(regexResults) != 1 || regexResults[0].Column != 5 {
		t.Fatalf("正则搜索应大小写敏感只命中一处，实际: %#v", regexResults)
	}
}

func TestServiceSearchRegexSkipsZeroWidthMatches(t *testing.T) {
	service := NewService(t.TempDir())
	if err := service.Create("chapters/ch01.md", "file", "bbb\naba\n"); err != nil {
		t.Fatal(err)
	}

	results, err := service.Search("a*", 100, SearchOptions{Regex: true})
	if err != nil {
		t.Fatalf("正则搜索失败: %v", err)
	}
	// 路径 "chapters/ch01.md" 也包含字母 a，会命中一条路径匹配，这里只校验正文结果。
	contentResults := make([]SearchResult, 0)
	for _, result := range results {
		if result.Line > 0 {
			contentResults = append(contentResults, result)
		}
	}
	if len(contentResults) != 2 {
		t.Fatalf("零宽匹配应被跳过，只保留两处 a，实际: %#v", results)
	}
	for _, result := range contentResults {
		if result.MatchText != "a" {
			t.Fatalf("零宽匹配不应出现在结果中: %#v", result)
		}
	}
}

func TestServiceSearchInvalidRegex(t *testing.T) {
	service := NewService(t.TempDir())
	if err := service.Create("chapters/ch01.md", "file", "正文\n"); err != nil {
		t.Fatal(err)
	}

	_, err := service.Search("(未闭合", 100, SearchOptions{Regex: true})
	if !errors.Is(err, ErrInvalidSearchRegex) {
		t.Fatalf("非法正则应返回 ErrInvalidSearchRegex，实际: %v", err)
	}
}
