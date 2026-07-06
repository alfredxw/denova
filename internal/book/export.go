package book

import (
	"errors"
	"path/filepath"
	"strings"
	"unicode"
)

// ErrNoExportableChapters indicates that a book has no non-empty chapters to export.
var ErrNoExportableChapters = errors.New("没有可导出的非空章节")

// TextExport is the assembled plain-text reading export for a book workspace.
type TextExport struct {
	Content      string
	ChapterCount int
}

// ExportText assembles all non-empty chapters into a single plain-text manuscript.
func (s *Service) ExportText(meta BookMeta) (TextExport, error) {
	summary, err := s.Summary()
	if err != nil {
		return TextExport{}, err
	}

	title := firstNonEmptyText(meta.Title, summary.Title, filepath.Base(s.workspace))
	author := firstNonEmptyText(meta.Author, summary.Author)
	blocks := make([]string, 0, len(summary.Chapters)*2+1)
	headerLines := []string{}
	if title != "" {
		headerLines = append(headerLines, title)
	}
	if author != "" {
		headerLines = append(headerLines, "作者: "+author)
	}
	if len(headerLines) > 0 {
		blocks = append(blocks, strings.Join(headerLines, "\n"))
	}

	lastVolumePath := ""
	chapterCount := 0
	for _, chapter := range summary.Chapters {
		if chapter.Words == 0 {
			continue
		}
		content, err := s.ReadFile(chapter.Path)
		if err != nil {
			return TextExport{}, err
		}
		body := exportChapterBody(content, chapter.DisplayTitle)
		if strings.TrimSpace(body) == "" {
			continue
		}
		if shouldWriteExportVolume(chapter, lastVolumePath) {
			blocks = append(blocks, chapter.Volume)
			lastVolumePath = chapter.VolumePath
		}
		if chapter.VolumePath != "" {
			lastVolumePath = chapter.VolumePath
		}
		blocks = append(blocks, chapter.DisplayTitle, body)
		chapterCount++
	}
	if chapterCount == 0 {
		return TextExport{}, ErrNoExportableChapters
	}
	return TextExport{
		Content:      strings.TrimSpace(strings.Join(blocks, "\n\n")) + "\n",
		ChapterCount: chapterCount,
	}, nil
}

func shouldWriteExportVolume(chapter ChapterSummary, lastVolumePath string) bool {
	return chapter.VolumePath != "" && chapter.VolumePath != "chapters" && chapter.VolumePath != lastVolumePath && strings.TrimSpace(chapter.Volume) != ""
}

func exportChapterBody(content, displayTitle string) string {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.ReplaceAll(content, "\r", "\n")
	content = strings.TrimPrefix(content, "\ufeff")
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		if sameExportTitle(line, displayTitle) {
			lines = append(lines[:i], lines[i+1:]...)
		}
		break
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func sameExportTitle(line, title string) bool {
	left := normalizeExportTitle(line)
	right := normalizeExportTitle(title)
	return left != "" && left == right
}

func normalizeExportTitle(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, "# \t")
	value = strings.TrimSpace(value)
	var b strings.Builder
	for _, r := range value {
		if unicode.IsSpace(r) || isExportTitlePunctuation(r) {
			continue
		}
		b.WriteRune(unicode.ToLower(r))
	}
	return b.String()
}

func isExportTitlePunctuation(r rune) bool {
	return strings.ContainsRune("#*_`~[]()（）【】《》<>:：-—_、，,.．。", r)
}

func firstNonEmptyText(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
