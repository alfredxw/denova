package book

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	NovelImportMaxPreviewChapters = 12

	NovelImportSingleChapterWarning = "novel_import_single_chapter"
)

var (
	mdHeadingRe      = regexp.MustCompile(`^\s{0,3}#{1,6}\s+(.+?)\s*$`)
	cnChapterRe      = regexp.MustCompile(`^\s*第[0-9零〇一二三四五六七八九十百千万两]+[章节卷回部集][^\n\r]{0,80}$`)
	enChapterRe      = regexp.MustCompile(`(?i)^\s*(chapter|part|volume)\s+[0-9ivxlcdm]+[^\n\r]{0,80}$`)
	numberedTitleRe  = regexp.MustCompile(`^\s*[0-9]{1,4}[\.、]\s*[^\n\r]{1,80}$`)
	blankLinePattern = regexp.MustCompile(`\n{3,}`)
)

// NovelImportPreview describes the chapters parsed from an uploaded novel file.
type NovelImportPreview struct {
	Title        string               `json:"title"`
	ChapterCount int                  `json:"chapter_count"`
	TotalChars   int                  `json:"total_chars"`
	Chapters     []NovelImportChapter `json:"chapters"`
	Warnings     []string             `json:"warnings,omitempty"`
}

// NovelImportChapter is a parsed source chapter.
type NovelImportChapter struct {
	Index int    `json:"index"`
	Title string `json:"title"`
	Chars int    `json:"chars"`
	Path  string `json:"path,omitempty"`
}

type parsedNovelChapter struct {
	NovelImportChapter
	Content string
}

type parsedNovel struct {
	Preview  NovelImportPreview
	Chapters []parsedNovelChapter
}

// NovelImportResult describes a completed file import.
type NovelImportResult struct {
	Workspace    string    `json:"workspace"`
	BookMeta     *BookMeta `json:"book_meta,omitempty"`
	Title        string    `json:"title"`
	ChapterCount int       `json:"chapter_count"`
	TotalChars   int       `json:"total_chars"`
	ChapterPaths []string  `json:"chapter_paths"`
	Message      string    `json:"message"`
}

// PreviewNovelImport parses a txt/md upload without writing workspace files.
func PreviewNovelImport(filename string, data []byte) (NovelImportPreview, error) {
	parsed, err := parseNovelImport(filename, data)
	if err != nil {
		return NovelImportPreview{}, err
	}
	return parsed.Preview, nil
}

// ImportNovelToWorkspace writes parsed txt/md chapters into an initialized workspace.
func ImportNovelToWorkspace(workspace, filename string, data []byte) (NovelImportPreview, []string, error) {
	parsed, err := parseNovelImport(filename, data)
	if err != nil {
		return NovelImportPreview{}, nil, err
	}
	chapterDir := filepath.Join(workspace, "chapters")
	if err := os.MkdirAll(chapterDir, 0o755); err != nil {
		return NovelImportPreview{}, nil, fmt.Errorf("创建章节目录失败: %w", err)
	}
	paths := make([]string, 0, len(parsed.Chapters))
	for _, chapter := range parsed.Chapters {
		rel := chapter.Path
		if rel == "" {
			rel = chapterPath(chapter.Index, chapter.Title)
		}
		dst := filepath.Join(workspace, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return NovelImportPreview{}, nil, fmt.Errorf("创建章节子目录失败: %w", err)
		}
		if err := os.WriteFile(dst, []byte(chapter.Content), 0o644); err != nil {
			return NovelImportPreview{}, nil, fmt.Errorf("写入章节失败 %s: %w", rel, err)
		}
		paths = append(paths, rel)
	}
	preview := parsed.Preview
	for i := range preview.Chapters {
		if i < len(paths) {
			preview.Chapters[i].Path = paths[i]
		}
	}
	return preview, paths, nil
}

func parseNovelImport(filename string, data []byte) (parsedNovel, error) {
	name := strings.TrimSpace(filename)
	ext := strings.ToLower(filepath.Ext(name))
	if ext != ".txt" && ext != ".md" && ext != ".markdown" {
		return parsedNovel{}, fmt.Errorf("只支持 txt/md 文件")
	}
	if len(data) == 0 {
		return parsedNovel{}, fmt.Errorf("文件为空")
	}
	if !utf8.Valid(data) {
		return parsedNovel{}, fmt.Errorf("只支持 UTF-8 编码的 txt/md 文件")
	}
	text := normalizeNovelText(string(data))
	if strings.TrimSpace(text) == "" {
		return parsedNovel{}, fmt.Errorf("文件内容为空")
	}

	title := strings.TrimSuffix(filepath.Base(name), filepath.Ext(name))
	chapters := splitNovelChapters(text)
	totalChars := 0
	for i := range chapters {
		chapters[i].Index = i + 1
		chapters[i].Path = chapterPath(chapters[i].Index, chapters[i].Title)
		chapters[i].Chars = utf8.RuneCountInString(chapters[i].Content)
		totalChars += chapters[i].Chars
	}

	warnings := []string{}
	if len(chapters) == 1 {
		warnings = append(warnings, NovelImportSingleChapterWarning)
	}
	previewChapters := make([]NovelImportChapter, 0, minInt(len(chapters), NovelImportMaxPreviewChapters))
	for i := 0; i < len(chapters) && i < NovelImportMaxPreviewChapters; i++ {
		previewChapters = append(previewChapters, chapters[i].NovelImportChapter)
	}
	return parsedNovel{
		Preview: NovelImportPreview{
			Title:        title,
			ChapterCount: len(chapters),
			TotalChars:   totalChars,
			Chapters:     previewChapters,
			Warnings:     warnings,
		},
		Chapters: chapters,
	}, nil
}

func normalizeNovelText(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	text = strings.TrimPrefix(text, "\ufeff")
	text = blankLinePattern.ReplaceAllString(text, "\n\n")
	return strings.TrimSpace(text)
}

func splitNovelChapters(text string) []parsedNovelChapter {
	lines := strings.Split(text, "\n")
	type marker struct {
		line  int
		title string
	}
	markers := []marker{}
	for i, line := range lines {
		if title, ok := chapterTitle(line); ok {
			markers = append(markers, marker{line: i, title: title})
		}
	}
	if len(markers) == 0 {
		return []parsedNovelChapter{{
			NovelImportChapter: NovelImportChapter{Title: "正文"},
			Content:            text + "\n",
		}}
	}
	chapters := []parsedNovelChapter{}
	preface := strings.TrimSpace(strings.Join(lines[:markers[0].line], "\n"))
	if preface != "" {
		chapters = append(chapters, parsedNovelChapter{
			NovelImportChapter: NovelImportChapter{Title: "序章"},
			Content:            preface + "\n",
		})
	}
	for i, current := range markers {
		end := len(lines)
		if i+1 < len(markers) {
			end = markers[i+1].line
		}
		content := strings.TrimSpace(strings.Join(lines[current.line:end], "\n"))
		if content == "" {
			continue
		}
		chapters = append(chapters, parsedNovelChapter{
			NovelImportChapter: NovelImportChapter{Title: current.title},
			Content:            content + "\n",
		})
	}
	if len(chapters) == 0 {
		return []parsedNovelChapter{{
			NovelImportChapter: NovelImportChapter{Title: "正文"},
			Content:            text + "\n",
		}}
	}
	return chapters
}

func chapterTitle(line string) (string, bool) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return "", false
	}
	if matches := mdHeadingRe.FindStringSubmatch(trimmed); len(matches) == 2 {
		return strings.TrimSpace(matches[1]), true
	}
	if cnChapterRe.MatchString(trimmed) && !hasSentencePunctuation(trimmed) {
		return strings.Trim(trimmed, "# \t"), true
	}
	if enChapterRe.MatchString(trimmed) || numberedTitleRe.MatchString(trimmed) {
		return strings.Trim(trimmed, "# \t"), true
	}
	return "", false
}

func hasSentencePunctuation(s string) bool {
	return strings.ContainsAny(s, "。！？；，,.!?;")
}

func chapterPath(index int, title string) string {
	cleanTitle := safeFilenamePart(title)
	if cleanTitle == "" {
		cleanTitle = "chapter"
	}
	return fmt.Sprintf("chapters/ch%04d-%s.md", index, cleanTitle)
}

func safeFilenamePart(input string) string {
	input = strings.TrimSpace(input)
	var out []rune
	lastDash := false
	for _, r := range input {
		if r == '/' || r == '\\' || r == ':' || r == '*' || r == '?' || r == '"' || r == '<' || r == '>' || r == '|' {
			if !lastDash {
				out = append(out, '-')
				lastDash = true
			}
			continue
		}
		if unicode.IsSpace(r) {
			if !lastDash {
				out = append(out, '-')
				lastDash = true
			}
			continue
		}
		out = append(out, r)
		lastDash = false
		if len(out) >= 48 {
			break
		}
	}
	return strings.Trim(strings.TrimSpace(string(out)), "-.")
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
