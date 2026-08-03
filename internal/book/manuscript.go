package book

import (
	"errors"
	"path/filepath"
	"strings"
	"unicode"
)

// ErrNoExportableChapters indicates that a book has no non-empty chapters to export.
var ErrNoExportableChapters = errors.New("没有可导出的非空章节")

// Manuscript is the structured, format-independent reading export of a book:
// title/author metadata plus ordered volumes, each holding ordered chapters.
// Renderers (plain text, EPUB, …) consume this single model so chapter grouping
// and heading de-duplication live in one place instead of per format.
type Manuscript struct {
	Title   string
	Author  string
	Volumes []ManuscriptVolume
}

// ManuscriptVolume groups chapters under a volume heading. An empty Title marks
// the implicit "未分卷" volume (chapters directly under chapters/) that must not
// render a volume heading.
type ManuscriptVolume struct {
	Title    string
	Chapters []ManuscriptChapter
}

// ManuscriptChapter is one exportable chapter with its display title and body,
// where the body already has any duplicate leading heading removed.
type ManuscriptChapter struct {
	Title string
	Body  string
}

// ChapterCount returns the total number of chapters across all volumes.
func (m Manuscript) ChapterCount() int {
	count := 0
	for _, vol := range m.Volumes {
		count += len(vol.Chapters)
	}
	return count
}

// Manuscript assembles all non-empty chapters into the structured export model,
// grouping chapters by their source volume in first-encounter order. It returns
// ErrNoExportableChapters when no chapter has renderable body content.
func (s *Service) Manuscript(meta BookMeta) (Manuscript, error) {
	summary, err := s.Summary()
	if err != nil {
		return Manuscript{}, err
	}

	manuscript := Manuscript{
		Title:  firstNonEmptyText(meta.Title, summary.Title, filepath.Base(s.workspace)),
		Author: firstNonEmptyText(meta.Author, summary.Author),
	}

	// Map a volume path to its slot in Volumes so chapters that share a volume
	// collapse under a single heading regardless of global sort interleaving.
	volumeSlot := map[string]int{}
	for _, chapter := range summary.Chapters {
		if chapter.Words == 0 {
			continue
		}
		content, err := s.ReadFile(chapter.Path)
		if err != nil {
			return Manuscript{}, err
		}
		body := exportChapterBody(content, chapter.DisplayTitle)
		if strings.TrimSpace(body) == "" {
			continue
		}

		slot, ok := volumeSlot[chapter.VolumePath]
		if !ok {
			manuscript.Volumes = append(manuscript.Volumes, ManuscriptVolume{Title: exportVolumeTitle(chapter)})
			slot = len(manuscript.Volumes) - 1
			volumeSlot[chapter.VolumePath] = slot
		}
		manuscript.Volumes[slot].Chapters = append(manuscript.Volumes[slot].Chapters, ManuscriptChapter{
			Title: chapter.DisplayTitle,
			Body:  body,
		})
	}

	if manuscript.ChapterCount() == 0 {
		return Manuscript{}, ErrNoExportableChapters
	}
	return manuscript, nil
}

// exportVolumeTitle returns the heading for a chapter's volume, or "" for the
// implicit unnamed volume so no volume heading is rendered.
func exportVolumeTitle(chapter ChapterSummary) string {
	if chapter.VolumePath == "" || chapter.VolumePath == "chapters" {
		return ""
	}
	return strings.TrimSpace(chapter.Volume)
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
