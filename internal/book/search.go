package book

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
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

// SearchResult 表示 workspace 全文搜索的一条结果。
type SearchResult struct {
	Path      string `json:"path"`
	Line      int    `json:"line"`
	Column    int    `json:"column"`
	Preview   string `json:"preview"`
	MatchText string `json:"match_text"`
}

// Search 在当前作品 workspace 内执行大小写不敏感的扫描式搜索。
func (s *Service) Search(query string, limit int) ([]SearchResult, error) {
	return SearchWorkspace(s.workspace, query, limit)
}

// SearchWorkspace 递归扫描 workspace 下的文本文件和文件路径。
func SearchWorkspace(workspace, query string, limit int) ([]SearchResult, error) {
	normalizedQuery := strings.TrimSpace(query)
	if normalizedQuery == "" {
		return []SearchResult{}, nil
	}
	if limit <= 0 {
		limit = DefaultSearchLimit
	}
	if limit > MaxSearchLimit {
		limit = MaxSearchLimit
	}

	var results []SearchResult
	err := filepath.WalkDir(workspace, func(path string, entry fs.DirEntry, err error) error {
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

		results = append(results, matchPath(rel, normalizedQuery)...)
		if len(results) >= limit {
			return errSearchLimitReached
		}

		fileResults, err := searchFile(path, rel, normalizedQuery, limit-len(results))
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

func matchPath(relPath, query string) []SearchResult {
	index := indexFoldRunes(relPath, query)
	if index < 0 {
		return nil
	}
	runes := []rune(relPath)
	queryRunes := []rune(query)
	end := index + len(queryRunes)
	if end > len(runes) {
		end = len(runes)
	}
	return []SearchResult{{
		Path:      relPath,
		Line:      0,
		Column:    index + 1,
		Preview:   relPath,
		MatchText: string(runes[index:end]),
	}}
}

func searchFile(path, relPath, query string, limit int) ([]SearchResult, error) {
	if limit <= 0 || !isSearchableTextFile(relPath) {
		return nil, nil
	}
	info, err := os.Stat(path)
	if err != nil || info.IsDir() || info.Size() > maxSearchFileSize {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if err != nil || !utf8.Valid(data) || looksBinary(data) {
		return nil, nil
	}

	lines := strings.Split(string(data), "\n")
	results := make([]SearchResult, 0)
	for lineIndex, line := range lines {
		searchFrom := 0
		for searchFrom <= len([]rune(line)) {
			matchIndex := indexFoldRunesFrom(line, query, searchFrom)
			if matchIndex < 0 {
				break
			}
			lineRunes := []rune(line)
			queryRunes := []rune(query)
			end := matchIndex + len(queryRunes)
			if end > len(lineRunes) {
				end = len(lineRunes)
			}
			results = append(results, SearchResult{
				Path:      relPath,
				Line:      lineIndex + 1,
				Column:    matchIndex + 1,
				Preview:   buildSearchPreview(lineRunes, matchIndex, end),
				MatchText: string(lineRunes[matchIndex:end]),
			})
			if len(results) >= limit {
				return results, nil
			}
			searchFrom = end
		}
	}
	return results, nil
}

func isSearchableTextFile(path string) bool {
	_, ok := searchableTextExtensions[strings.ToLower(filepath.Ext(path))]
	return ok
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

func indexFoldRunes(text, query string) int {
	return indexFoldRunesFrom(text, query, 0)
}

func indexFoldRunesFrom(text, query string, from int) int {
	textRunes := []rune(text)
	queryRunes := []rune(query)
	if len(queryRunes) == 0 || len(queryRunes) > len(textRunes) || from >= len(textRunes) {
		return -1
	}
	for i := from; i <= len(textRunes)-len(queryRunes); i++ {
		matched := true
		for j := range queryRunes {
			if unicode.ToLower(textRunes[i+j]) != unicode.ToLower(queryRunes[j]) {
				matched = false
				break
			}
		}
		if matched {
			return i
		}
	}
	return -1
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
