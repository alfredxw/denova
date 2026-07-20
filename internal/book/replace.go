package book

import (
	"errors"
	"os"
	"sort"
	"strings"
)

// ErrRegexMatchesEmpty 表示正则表达式可以匹配空字符串，
// 全局替换会产生不可预期结果，因此拒绝执行。
var ErrRegexMatchesEmpty = errors.New("regex matches empty string")

// ReplaceFileResult 记录单个文件中被替换的次数。
type ReplaceFileResult struct {
	Path         string `json:"path"`
	Replacements int    `json:"replacements"`
}

// Replacer 是预编译的全局替换器。字面量模式按大小写不敏感匹配；
// 正则模式使用 Go(RE2) 语法、大小写敏感，替换文本支持 $1、${name} 捕获组引用。
type Replacer struct {
	matcher     *searchMatcher
	replacement string
}

// NewReplacer 校验并构建替换器；query 需与搜索侧保持同样的归一化（先 TrimSpace）。
func NewReplacer(query, replacement string, opts SearchOptions) (*Replacer, error) {
	matcher, err := newSearchMatcher(query, opts)
	if err != nil {
		return nil, err
	}
	if matcher.regex != nil && matcher.regex.MatchString("") {
		return nil, ErrRegexMatchesEmpty
	}
	template := replacement
	if matcher.regex != nil {
		template = convertReplacementTemplate(replacement, matcher.regex.NumSubexp())
	}
	return &Replacer{matcher: matcher, replacement: template}, nil
}

// convertReplacementTemplate 把 JS 风格的替换模板（$1、$&、$<name>、$$）转成
// Go regexp 模板（${1}、${0}、${name}、$$），与编辑器内替换的行为保持一致：
// Go 原生模板会把 $2与 解析为名为 "2与" 的变量，中文紧贴捕获组引用时会得到错误结果。
// 引用不存在的捕获组（序号大于分组数）时按字面文本保留，与 JS 行为一致。
func convertReplacementTemplate(replacement string, numSubexp int) string {
	if !strings.Contains(replacement, "$") {
		return replacement
	}
	var b strings.Builder
	b.Grow(len(replacement))
	for i := 0; i < len(replacement); i++ {
		if replacement[i] != '$' {
			b.WriteByte(replacement[i])
			continue
		}
		if i+1 >= len(replacement) {
			b.WriteString("$$")
			continue
		}
		next := replacement[i+1]
		switch {
		case next == '$':
			b.WriteString("$$")
			i++
		case next == '&':
			b.WriteString("${0}")
			i++
		case next >= '0' && next <= '9':
			j := i + 1
			for j < len(replacement) && replacement[j] >= '0' && replacement[j] <= '9' {
				j++
			}
			digits := replacement[i+1 : j]
			n := 0
			for _, d := range digits {
				n = n*10 + int(d-'0')
			}
			if n >= 1 && n <= numSubexp {
				b.WriteString("${")
				b.WriteString(digits)
				b.WriteString("}")
			} else {
				// 捕获组不存在：保留字面文本（JS 语义）。
				b.WriteString("$$")
				b.WriteString(digits)
			}
			i = j - 1
		case next == '<':
			end := strings.IndexByte(replacement[i+2:], '>')
			if end < 0 {
				b.WriteString("$$")
				continue
			}
			b.WriteString("${")
			b.WriteString(replacement[i+2 : i+2+end])
			b.WriteString("}")
			i += end + 2
		default:
			b.WriteString("$$")
		}
	}
	return b.String()
}

// ReplaceAll 返回替换后的内容与替换次数；没有匹配时原样返回内容且次数为 0。
func (r *Replacer) ReplaceAll(content string) (string, int) {
	if r.matcher.regex != nil {
		matches := r.matcher.regex.FindAllStringIndex(content, -1)
		if len(matches) == 0 {
			return content, 0
		}
		return r.matcher.regex.ReplaceAllString(content, r.replacement), len(matches)
	}
	return replaceLiteralFold(content, r.matcher.literal, r.replacement)
}

func replaceLiteralFold(content, query, replacement string) (string, int) {
	runes := []rune(content)
	queryRunes := []rune(query)
	if len(queryRunes) == 0 || len(queryRunes) > len(runes) {
		return content, 0
	}
	var b strings.Builder
	b.Grow(len(content))
	count := 0
	i := 0
	for i <= len(runes)-len(queryRunes) {
		if foldMatchAt(runes, queryRunes, i) {
			b.WriteString(replacement)
			i += len(queryRunes)
			count++
			continue
		}
		b.WriteRune(runes[i])
		i++
	}
	if count == 0 {
		return content, 0
	}
	b.WriteString(string(runes[i:]))
	return b.String(), count
}

// ListReplaceCandidateFiles 列出 workspace 内可参与全局替换的文本文件相对路径，
// 过滤规则与搜索一致：跳过隐藏项、符号链接、超出大小上限或非文本扩展名的文件。
func ListReplaceCandidateFiles(workspace string) ([]string, error) {
	paths := make([]string, 0)
	err := walkVisibleWorkspaceFiles(workspace, func(absPath, rel string) error {
		if !isSearchableTextFile(rel) {
			return nil
		}
		info, err := os.Stat(absPath)
		if err != nil || info.IsDir() || info.Size() > maxSearchFileSize {
			return nil
		}
		paths = append(paths, rel)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	return paths, nil
}
