package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
)

const countWordsToolName = "count_words"

var countWordsToolDescription = strings.TrimSpace(`Count characters and words in a workspace file or inline text. Returns multiple metrics so the caller can pick the one that matches the writing plan.

Primary metric for Chinese fiction: chinese_chars (CJK unified ideographs, one per character).

- Provide file_path to count a file inside the workspace, or text to count inline content. Provide exactly one.
- chinese_chars: number of CJK unified ideographs (U+4E00–U+9FFF, U+3400–U+4DBF, U+F900–U+FAFF, and CJK extension ranges). This is the default "中文字数" metric.
- chars_no_space: total characters excluding all whitespace (spaces, tabs, newlines). Includes CJK, punctuation, and Latin letters.
- chars_with_space: total characters including whitespace.
- lines: number of lines (newline-separated).
- non_cjk_words: space-separated word count of non-CJK text segments (useful for mixed-language content).

统计 workspace 文件或内联文本的字符数和字数，返回多维度计数供调用方选择。

中文小说的主要指标：chinese_chars（CJK 统一表意文字，每字计 1）。

- 提供 file_path 统计工作区内文件，或提供 text 统计内联内容。两者只能提供一个。
- chinese_chars：CJK 统一表意文字数量（即"中文字数"）。
- chars_no_space：不计空白的总字符数（含中文、标点、英文字母）。
- chars_with_space：计空白的总字符数。
- lines：行数。
- non_cjk_words：非中文部分按空格分词的单词数（用于中英混排内容）。`)

type countWordsInput struct {
	FilePath string `json:"file_path,omitempty" jsonschema:"description=Absolute path of a workspace file to count. Mutually exclusive with text."`
	Text     string `json:"text,omitempty" jsonschema:"description=Inline text to count. Mutually exclusive with file_path."`
}

type countWordsResult struct {
	Schema         string `json:"schema"`
	Source         string `json:"source"`
	ChineseChars   int    `json:"chinese_chars"`
	CharsNoSpace   int    `json:"chars_no_space"`
	CharsWithSpace int    `json:"chars_with_space"`
	Lines          int    `json:"lines"`
	NonCJKWords    int    `json:"non_cjk_words"`
}

func newCountWordsTool(workspaces ...string) (tool.BaseTool, error) {
	workspace := ""
	if len(workspaces) > 0 {
		workspace = strings.TrimSpace(workspaces[0])
	}
	return utils.InferTool(countWordsToolName, countWordsToolDescription, func(ctx context.Context, input countWordsInput) (string, error) {
		hasFile := strings.TrimSpace(input.FilePath) != ""
		hasText := len(input.Text) > 0
		if hasFile == hasText {
			return "", fmt.Errorf("provide exactly one of file_path or text")
		}

		var content string
		var source string
		if hasFile {
			absolute, _, err := resolveWorkspaceReadPath(workspace, input.FilePath)
			if err != nil {
				return "", err
			}
			data, readErr := readFileContentForCount(absolute)
			if readErr != nil {
				return "", fmt.Errorf("read file for count_words: %w", readErr)
			}
			content = data
			source = absolute
		} else {
			content = input.Text
			source = "inline_text"
		}

		result := countTextMetrics(content)
		result.Schema = "count_words.v1"
		result.Source = source

		out, err := json.Marshal(result)
		if err != nil {
			return "", fmt.Errorf("serialize count_words result: %w", err)
		}
		return string(out), nil
	})
}

// countTextMetrics computes multi-dimensional character and word counts.
func countTextMetrics(text string) countWordsResult {
	var chineseChars, charsNoSpace, charsWithSpace, nonCJKWords int
	lines := 1
	inNonCJKWord := false

	for i := 0; i < len(text); {
		r, size := utf8.DecodeRuneInString(text[i:])
		i += size

		charsWithSpace++

		if r == '\n' {
			lines++
			if inNonCJKWord {
				nonCJKWords++
				inNonCJKWord = false
			}
			continue
		}

		if unicode.IsSpace(r) {
			if inNonCJKWord {
				nonCJKWords++
				inNonCJKWord = false
			}
			continue
		}

		charsNoSpace++

		if isCJKRune(r) {
			chineseChars++
			if inNonCJKWord {
				nonCJKWords++
				inNonCJKWord = false
			}
		} else {
			inNonCJKWord = true
		}
	}
	if inNonCJKWord {
		nonCJKWords++
	}
	if len(text) == 0 {
		lines = 0
	}

	return countWordsResult{
		ChineseChars:   chineseChars,
		CharsNoSpace:   charsNoSpace,
		CharsWithSpace: charsWithSpace,
		Lines:          lines,
		NonCJKWords:    nonCJKWords,
	}
}

// isCJKRune reports whether r is a CJK unified ideograph.
func isCJKRune(r rune) bool {
	return (r >= 0x4E00 && r <= 0x9FFF) || // CJK Unified Ideographs
		(r >= 0x3400 && r <= 0x4DBF) || // CJK Extension A
		(r >= 0xF900 && r <= 0xFAFF) || // CJK Compatibility Ideographs
		(r >= 0x20000 && r <= 0x2A6DF) || // CJK Extension B
		(r >= 0x2A700 && r <= 0x2B73F) || // CJK Extension C
		(r >= 0x2B740 && r <= 0x2B81F) || // CJK Extension D
		(r >= 0x2B820 && r <= 0x2CEAF) || // CJK Extension E
		(r >= 0x2CEB0 && r <= 0x2EBEF) || // CJK Extension F
		(r >= 0x30000 && r <= 0x3134F) // CJK Extension G
}

// readFileContentForCount reads a file's full content for counting.
func readFileContentForCount(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
