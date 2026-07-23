package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/cloudwego/eino/components/tool"
)

func TestCountTextMetricsChinese(t *testing.T) {
	text := "林川走进了余烬城。\nHe walked into the city."
	result := countTextMetrics(text)

	// 林川走进了余烬城 = 8 CJK chars
	if result.ChineseChars != 8 {
		t.Errorf("ChineseChars = %d, want 8", result.ChineseChars)
	}
	// Non-CJK words: "。", "He", "walked", "into", "the", "city." = 6
	// (fullwidth period 。is non-CJK, non-space, so it forms its own word)
	if result.NonCJKWords != 6 {
		t.Errorf("NonCJKWords = %d, want 6", result.NonCJKWords)
	}
	if result.Lines != 2 {
		t.Errorf("Lines = %d, want 2", result.Lines)
	}
	// chars_no_space should be > chinese_chars (includes punctuation and latin)
	if result.CharsNoSpace <= result.ChineseChars {
		t.Errorf("CharsNoSpace = %d, should be > ChineseChars = %d", result.CharsNoSpace, result.ChineseChars)
	}
	if result.CharsWithSpace < result.CharsNoSpace {
		t.Errorf("CharsWithSpace = %d, should be >= CharsNoSpace = %d", result.CharsWithSpace, result.CharsNoSpace)
	}
}

func TestCountTextMetricsEmpty(t *testing.T) {
	result := countTextMetrics("")
	if result.ChineseChars != 0 || result.CharsNoSpace != 0 || result.Lines != 0 {
		t.Errorf("empty text should return all zeros, got %+v", result)
	}
}

func TestCountTextMetricsPureEnglish(t *testing.T) {
	result := countTextMetrics("Hello world foo bar")
	if result.ChineseChars != 0 {
		t.Errorf("ChineseChars = %d, want 0", result.ChineseChars)
	}
	if result.NonCJKWords != 4 {
		t.Errorf("NonCJKWords = %d, want 4", result.NonCJKWords)
	}
	if result.Lines != 1 {
		t.Errorf("Lines = %d, want 1", result.Lines)
	}
}

func TestCountTextMetricsCJKPunctuation(t *testing.T) {
	// Chinese punctuation should NOT count as CJK ideographs
	text := "你好，世界！"
	result := countTextMetrics(text)
	// 你好世界 = 4 CJK chars; ，and ！are punctuation, not CJK ideographs
	if result.ChineseChars != 4 {
		t.Errorf("ChineseChars = %d, want 4 (punctuation excluded)", result.ChineseChars)
	}
}

func TestIsCJKRune(t *testing.T) {
	tests := []struct {
		r    rune
		want bool
	}{
		{'中', true},
		{'a', false},
		{'，', false},         // fullwidth comma is punctuation
		{'。', false},         // fullwidth period
		{'\u4E00', true},     // CJK Unified start
		{'\u9FFF', true},     // CJK Unified end
		{'\u3400', true},     // Extension A start
		{'\uF900', true},     // Compatibility start
		{'\U00020000', true}, // Extension B start
	}
	for _, tt := range tests {
		if got := isCJKRune(tt.r); got != tt.want {
			t.Errorf("isCJKRune(%q) = %v, want %v", tt.r, got, tt.want)
		}
	}
}

func TestCountWordsToolInlineText(t *testing.T) {
	base, err := newCountWordsTool()
	if err != nil {
		t.Fatalf("newCountWordsTool: %v", err)
	}
	invokable, ok := base.(tool.InvokableTool)
	if !ok {
		t.Fatal("count_words tool is not InvokableTool")
	}
	result, err := invokable.InvokableRun(context.Background(), `{"text":"林川走进了余烬城"}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	var parsed countWordsResult
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if parsed.ChineseChars != 8 {
		t.Errorf("ChineseChars = %d, want 8", parsed.ChineseChars)
	}
	if parsed.Source != "inline_text" {
		t.Errorf("Source = %q, want inline_text", parsed.Source)
	}
}

func TestCountWordsToolFile(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "chapter.md")
	content := "第一章\n\n林川走进了余烬城，心中满是忐忑。"
	if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	base, err := newCountWordsTool(dir)
	if err != nil {
		t.Fatalf("newCountWordsTool: %v", err)
	}
	invokable, ok := base.(tool.InvokableTool)
	if !ok {
		t.Fatal("count_words tool is not InvokableTool")
	}
	input, _ := json.Marshal(countWordsInput{FilePath: filePath})
	result, err := invokable.InvokableRun(context.Background(), string(input))
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	var parsed countWordsResult
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	// 第一章林川走进了余烬城心中满是忐忑 = 17 CJK chars
	if parsed.ChineseChars != 17 {
		t.Errorf("ChineseChars = %d, want 17", parsed.ChineseChars)
	}
	if parsed.Lines != 3 {
		t.Errorf("Lines = %d, want 3", parsed.Lines)
	}
}

func TestCountWordsToolRejectsBothInputs(t *testing.T) {
	base, err := newCountWordsTool()
	if err != nil {
		t.Fatalf("newCountWordsTool: %v", err)
	}
	invokable := base.(tool.InvokableTool)
	_, err = invokable.InvokableRun(context.Background(), `{"text":"hello","file_path":"/tmp/x"}`)
	if err == nil {
		t.Fatal("expected error when both text and file_path provided")
	}
}

func TestCountWordsToolRejectsNeitherInput(t *testing.T) {
	base, err := newCountWordsTool()
	if err != nil {
		t.Fatalf("newCountWordsTool: %v", err)
	}
	invokable := base.(tool.InvokableTool)
	_, err = invokable.InvokableRun(context.Background(), `{}`)
	if err == nil {
		t.Fatal("expected error when neither text nor file_path provided")
	}
}
