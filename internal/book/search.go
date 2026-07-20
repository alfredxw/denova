package book

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	DefaultSearchLimit = 100
	MaxSearchLimit     = 500
	maxSearchFileSize  = 2 * 1024 * 1024
	searchPreviewRunes = 48
)

var searchableTextExtensions = map[string]struct{}{
	".csv":  {},
	".json": {},
	".md":   {},
	".toml": {},
	".txt":  {},
	".yaml": {},
	".yml":  {},
}

// ErrInvalidSearchRegex 表示查询字符串不是合法的正则表达式。
var ErrInvalidSearchRegex = errors.New("invalid search regex")

// SearchOptions 控制全局搜索的匹配方式。
type SearchOptions struct {
	// Regex 为 true 时按 Go(RE2) 正则表达式匹配且大小写敏感；
	// 为 false 时按大小写不敏感的字面量匹配。
	Regex bool
}

// SearchResult 表示 workspace 全文搜索的一条结果。
type SearchResult struct {
	Path      string `json:"path"`
	Line      int    `json:"line"`
	Column    int    `json:"column"`
	Preview   string `json:"preview"`
	MatchText string `json:"match_text"`
}

// Search 在当前作品 workspace 内执行扫描式搜索。
func (s *Service) Search(query string, limit int, opts SearchOptions) ([]SearchResult, error) {
	return SearchWorkspace(s.workspace, query, limit, opts)
}

// SearchWorkspace 递归扫描 workspace 下的文本文件和文件路径。
func SearchWorkspace(workspace, query string, limit int, opts SearchOptions) ([]SearchResult, error) {
	normalizedQuery := strings.TrimSpace(query)
	if normalizedQuery == "" {
		return []SearchResult{}, nil
	}
	matcher, err := newSearchMatcher(normalizedQuery, opts)
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = DefaultSearchLimit
	}
	if limit > MaxSearchLimit {
		limit = MaxSearchLimit
	}

	var results []SearchResult
	err = walkVisibleWorkspaceFiles(workspace, func(path, rel string) error {
		results = append(results, matchPath(rel, matcher)...)
		if len(results) >= limit {
			return errSearchLimitReached
		}
		fileResults, err := searchFile(path, rel, matcher, limit-len(results))
		if err != nil {
			return nil
		}
		results = append(results, fileResults...)
		if len(results) >= limit {
			return errSearchLimitReached
		}
		return nil
	})
	if errors.Is(err, errSearchLimitReached) {
		err = nil
	}
	if err != nil {
		return nil, err
	}
	sort.SliceStable(results, func(i, j int) bool {
		if results[i].Path != results[j].Path {
			return results[i].Path < results[j].Path
		}
		if results[i].Line != results[j].Line {
			return results[i].Line < results[j].Line
		}
		return results[i].Column < results[j].Column
	})
	if len(results) > limit {
		results = results[:limit]
	}
	return results, nil
}

var errSearchLimitReached = errors.New("search limit reached")

// walkVisibleWorkspaceFiles 按统一规则遍历 workspace 内的可见文件：
// 跳过隐藏文件/目录、符号链接以及未通过 SafePath 校验的路径。
func walkVisibleWorkspaceFiles(workspace string, visit func(absPath, rel string) error) error {
	return filepath.WalkDir(workspace, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		name := entry.Name()
		if name != "." && strings.HasPrefix(name, ".") {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		rel, err := filepath.Rel(workspace, path)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if _, err := SafePath(workspace, rel); err != nil {
			return nil
		}
		return visit(path, rel)
	})
}

// searchMatcher 统一字面量（大小写不敏感折叠）与正则两种匹配方式，
// 匹配结果以 rune 索引区间 [start, end) 表示。
type searchMatcher struct {
	literal string
	regex   *regexp.Regexp
}

func newSearchMatcher(query string, opts SearchOptions) (*searchMatcher, error) {
	if !opts.Regex {
		return &searchMatcher{literal: query}, nil
	}
	re, err := regexp.Compile(query)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidSearchRegex, err)
	}
	return &searchMatcher{regex: re}, nil
}

// findAll 返回 text 内所有非空匹配的 rune 区间；零宽匹配会被跳过。
func (m *searchMatcher) findAll(text string) [][2]int {
	if m.regex != nil {
		matches := m.regex.FindAllStringIndex(text, -1)
		spans := make([][2]int, 0, len(matches))
		for _, match := range matches {
			if match[0] == match[1] {
				continue
			}
			start := utf8.RuneCountInString(text[:match[0]])
			end := start + utf8.RuneCountInString(text[match[0]:match[1]])
			spans = append(spans, [2]int{start, end})
		}
		return spans
	}
	return literalFoldSpans(text, m.literal)
}

func literalFoldSpans(text, query string) [][2]int {
	queryLen := len([]rune(query))
	if queryLen == 0 {
		return nil
	}
	spans := make([][2]int, 0)
	for from := 0; ; {
		index := indexFoldRunesFrom(text, query, from)
		if index < 0 {
			return spans
		}
		spans = append(spans, [2]int{index, index + queryLen})
		from = index + queryLen
	}
}

func matchPath(relPath string, matcher *searchMatcher) []SearchResult {
	spans := matcher.findAll(relPath)
	if len(spans) == 0 {
		return nil
	}
	runes := []rune(relPath)
	start, end := spans[0][0], spans[0][1]
	return []SearchResult{{
		Path:      relPath,
		Line:      0,
		Column:    start + 1,
		Preview:   relPath,
		MatchText: string(runes[start:end]),
	}}
}

func searchFile(path, relPath string, matcher *searchMatcher, limit int) ([]SearchResult, error) {
	if limit <= 0 || !isSearchableTextFile(relPath) {
		return nil, nil
	}
	info, err := os.Stat(path)
	if err != nil || info.IsDir() || info.Size() > maxSearchFileSize {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if err != nil || !IsSearchableContent(data) {
		return nil, nil
	}

	lines := strings.Split(string(data), "\n")
	results := make([]SearchResult, 0)
	for lineIndex, line := range lines {
		lineRunes := []rune(line)
		for _, span := range matcher.findAll(line) {
			start, end := span[0], span[1]
			results = append(results, SearchResult{
				Path:      relPath,
				Line:      lineIndex + 1,
				Column:    start + 1,
				Preview:   buildSearchPreview(lineRunes, start, end),
				MatchText: string(lineRunes[start:end]),
			})
			if len(results) >= limit {
				return results, nil
			}
		}
	}
	return results, nil
}

func isSearchableTextFile(path string) bool {
	_, ok := searchableTextExtensions[strings.ToLower(filepath.Ext(path))]
	return ok
}

// IsSearchableContent 报告文件内容是否为可参与搜索/替换的 UTF-8 文本。
func IsSearchableContent(data []byte) bool {
	return utf8.Valid(data) && !looksBinary(data)
}

func looksBinary(data []byte) bool {
	if len(data) == 0 {
		return false
	}
	sampleSize := len(data)
	if sampleSize > 4096 {
		sampleSize = 4096
	}
	for _, b := range data[:sampleSize] {
		if b == 0 {
			return true
		}
	}
	return false
}

func indexFoldRunesFrom(text, query string, from int) int {
	textRunes := []rune(text)
	queryRunes := []rune(query)
	if len(queryRunes) == 0 || len(queryRunes) > len(textRunes) || from >= len(textRunes) {
		return -1
	}
	for i := from; i <= len(textRunes)-len(queryRunes); i++ {
		if foldMatchAt(textRunes, queryRunes, i) {
			return i
		}
	}
	return -1
}

func foldMatchAt(textRunes, queryRunes []rune, i int) bool {
	for j := range queryRunes {
		if unicode.ToLower(textRunes[i+j]) != unicode.ToLower(queryRunes[j]) {
			return false
		}
	}
	return true
}

func buildSearchPreview(lineRunes []rune, start, end int) string {
	from := start - searchPreviewRunes
	if from < 0 {
		from = 0
	}
	to := end + searchPreviewRunes
	if to > len(lineRunes) {
		to = len(lineRunes)
	}
	preview := strings.TrimSpace(string(lineRunes[from:to]))
	if from > 0 {
		preview = "..." + preview
	}
	if to < len(lineRunes) {
		preview += "..."
	}
	return preview
}
