package book

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode"
)

// WorkspaceSummary 汇总当前作品的写作进度。
type WorkspaceSummary struct {
	Title        string            `json:"title"`
	Author       string            `json:"author"`
	ChapterCount int               `json:"chapter_count"`
	TotalWords   int               `json:"total_words"`
	Chapters     []ChapterSummary  `json:"chapters"`
	Outline      *DocumentPreview  `json:"outline,omitempty"`
	ChapterPlans []DocumentPreview `json:"chapter_plans"`
}

// ChapterSummary 描述单个章节文件的轻量统计信息。
type ChapterSummary struct {
	Path         string `json:"path"`
	FileName     string `json:"file_name"`
	DisplayTitle string `json:"display_title"`
	Index        int    `json:"index"`
	Words        int    `json:"words"`
	Status       string `json:"status"`
	UpdatedAt    string `json:"updated_at"`
	Volume       string `json:"volume"`
	VolumePath   string `json:"volume_path"`
}

// DocumentPreview 描述大纲、细纲等规划文档的轻量预览。
type DocumentPreview struct {
	Path      string `json:"path"`
	Title     string `json:"title"`
	Excerpt   string `json:"excerpt"`
	Words     int    `json:"words"`
	UpdatedAt string `json:"updated_at"`
}

var chapterNamePattern = regexp.MustCompile(`(?i)^ch(\d+)[-_ ]*(.*)$`)
var numericChapterNamePattern = regexp.MustCompile(`^(\d{1,6})[-_ 、.．]+(.*)$`)
var chineseChapterNamePattern = regexp.MustCompile(`^第([0-9零〇一二三四五六七八九十百千万两]+)[章节回集][-_ 、.．]*(.*)$`)
var englishChapterNamePattern = regexp.MustCompile(`(?i)^(?:chapter|ch)[-_ ]*([0-9ivxlcdm]+)[-_ .:：]*(.*)$`)
var groupNamePattern = regexp.MustCompile(`(?i)^group(\d+)[-_ ]*(.*)$`)

type chapterNameMeta struct {
	index   int
	display string
}

// Summary 统计 workspace 的章节进度和书籍元信息。
func (s *Service) Summary() (WorkspaceSummary, error) {
	meta := ReadBookMetaFromDir(s.workspace)
	summary := WorkspaceSummary{
		Title:        meta.Title,
		Author:       meta.Author,
		ChapterPlans: []DocumentPreview{},
	}

	summary.Outline = s.documentPreview("setting/outline.md", "长期大纲")
	summary.ChapterPlans = s.chapterPlanPreviews()

	chapterRoot := filepath.Join(s.workspace, "chapters")
	if _, err := os.Stat(chapterRoot); os.IsNotExist(err) {
		return summary, nil
	} else if err != nil {
		return summary, err
	}

	err := filepath.WalkDir(chapterRoot, func(path string, entry os.DirEntry, err error) error {
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
		if entry.IsDir() || !isChapterTextFile(name) {
			return nil
		}

		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		info, statErr := entry.Info()
		if statErr != nil {
			return nil
		}
		rel, relErr := filepath.Rel(s.workspace, path)
		if relErr != nil {
			return nil
		}
		words := countWritingWords(string(data))
		chapter := ChapterSummary{
			Path:         filepath.ToSlash(rel),
			FileName:     name,
			DisplayTitle: chapterDisplayTitle(name),
			Index:        chapterIndex(name),
			Words:        words,
			Status:       chapterStatus(words),
			UpdatedAt:    info.ModTime().Format("2006-01-02 15:04"),
		}
		chapter.Volume, chapter.VolumePath = chapterVolume(chapter.Path)
		summary.Chapters = append(summary.Chapters, chapter)
		summary.TotalWords += words
		return nil
	})
	if err != nil {
		return summary, err
	}

	sort.Slice(summary.Chapters, func(i, j int) bool {
		left, right := summary.Chapters[i], summary.Chapters[j]
		if left.Index > 0 && right.Index > 0 && left.Index != right.Index {
			return left.Index < right.Index
		}
		return left.Path < right.Path
	})
	summary.ChapterCount = len(summary.Chapters)
	return summary, nil
}

func (s *Service) documentPreview(relPath, fallbackTitle string) *DocumentPreview {
	absPath := filepath.Join(s.workspace, filepath.FromSlash(relPath))
	info, err := os.Stat(absPath)
	if err != nil || info.IsDir() {
		return nil
	}
	data, err := os.ReadFile(absPath)
	if err != nil {
		return nil
	}
	content := string(data)
	return &DocumentPreview{
		Path:      relPath,
		Title:     documentTitle(content, fallbackTitle, relPath),
		Excerpt:   documentExcerpt(content),
		Words:     countWritingWords(content),
		UpdatedAt: info.ModTime().Format("2006-01-02 15:04"),
	}
}

func (s *Service) chapterPlanPreviews() []DocumentPreview {
	root := filepath.Join(s.workspace, "setting", "chapter-groups")
	if _, err := os.Stat(root); err != nil {
		return []DocumentPreview{}
	}

	previews := []DocumentPreview{}
	_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
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
		if entry.IsDir() || !isChapterTextFile(name) {
			return nil
		}
		rel, relErr := filepath.Rel(s.workspace, path)
		if relErr != nil {
			return nil
		}
		relPath := filepath.ToSlash(rel)
		if preview := s.documentPreview(relPath, groupDisplayTitle(name)); preview != nil {
			previews = append(previews, *preview)
		}
		return nil
	})

	sort.Slice(previews, func(i, j int) bool {
		left, right := chapterPlanIndex(previews[i].Path), chapterPlanIndex(previews[j].Path)
		if left > 0 && right > 0 && left != right {
			return left < right
		}
		return previews[i].Path < previews[j].Path
	})
	return previews
}

func isChapterTextFile(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	return ext == ".md" || ext == ".txt"
}

func chapterDisplayTitle(name string) string {
	base := strings.TrimSuffix(name, filepath.Ext(name))
	if meta, ok := parseChapterNameMeta(base); ok {
		return meta.display
	}
	return displayNameFromFilenameBase(base)
}

func parseChapterNameMeta(base string) (chapterNameMeta, bool) {
	if matches := chapterNamePattern.FindStringSubmatch(base); len(matches) > 0 {
		title := strings.Trim(matches[2], "-_ ")
		if title == "" {
			return chapterNameMeta{index: parsePositiveInt(matches[1]), display: "第" + matches[1] + "章"}, true
		}
		return chapterNameMeta{index: parsePositiveInt(matches[1]), display: matches[1] + " " + displayNameFromFilenameBase(title)}, true
	}
	if matches := chineseChapterNamePattern.FindStringSubmatch(base); len(matches) > 0 {
		index := parseChapterOrdinal(matches[1])
		title := strings.Trim(matches[2], "-_ 、.． ")
		prefix := "第" + matches[1] + "章"
		if title == "" {
			return chapterNameMeta{index: index, display: prefix}, true
		}
		return chapterNameMeta{index: index, display: prefix + " " + displayNameFromFilenameBase(title)}, true
	}
	if matches := englishChapterNamePattern.FindStringSubmatch(base); len(matches) > 0 {
		index := parseChapterOrdinal(matches[1])
		title := strings.Trim(matches[2], "-_ .:： ")
		prefix := "Chapter " + matches[1]
		if title == "" {
			return chapterNameMeta{index: index, display: prefix}, true
		}
		return chapterNameMeta{index: index, display: prefix + " " + displayNameFromFilenameBase(title)}, true
	}
	if matches := numericChapterNamePattern.FindStringSubmatch(base); len(matches) > 0 {
		title := strings.Trim(matches[2], "-_ 、.． ")
		if title == "" {
			return chapterNameMeta{index: parsePositiveInt(matches[1]), display: matches[1]}, true
		}
		return chapterNameMeta{index: parsePositiveInt(matches[1]), display: matches[1] + " " + displayNameFromFilenameBase(title)}, true
	}
	return chapterNameMeta{}, false
}

func displayNameFromFilenameBase(base string) string {
	return strings.TrimSpace(strings.NewReplacer("-", " ", "_", " ").Replace(base))
}

func parsePositiveInt(value string) int {
	n := 0
	for _, ch := range value {
		if ch < '0' || ch > '9' {
			return 0
		}
		n = n*10 + int(ch-'0')
	}
	return n
}

func parseChapterOrdinal(value string) int {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	if allASCIIDigits(value) {
		return parsePositiveInt(value)
	}
	if n := parseRomanNumeral(value); n > 0 {
		return n
	}
	return parseChineseNumber(value)
}

func allASCIIDigits(value string) bool {
	for _, ch := range value {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	return value != ""
}

func parseRomanNumeral(value string) int {
	values := map[rune]int{'I': 1, 'V': 5, 'X': 10, 'L': 50, 'C': 100, 'D': 500, 'M': 1000}
	value = strings.ToUpper(value)
	total := 0
	prev := 0
	for i := len(value) - 1; i >= 0; i-- {
		current := values[rune(value[i])]
		if current == 0 {
			return 0
		}
		if current < prev {
			total -= current
		} else {
			total += current
			prev = current
		}
	}
	return total
}

func parseChineseNumber(value string) int {
	digits := map[rune]int{'零': 0, '〇': 0, '一': 1, '二': 2, '两': 2, '三': 3, '四': 4, '五': 5, '六': 6, '七': 7, '八': 8, '九': 9}
	units := map[rune]int{'十': 10, '百': 100, '千': 1000, '万': 10000}
	total := 0
	section := 0
	number := 0
	seen := false
	for _, ch := range value {
		if digit, ok := digits[ch]; ok {
			number = digit
			seen = true
			continue
		}
		unit, ok := units[ch]
		if !ok {
			return 0
		}
		seen = true
		if unit == 10000 {
			if number != 0 {
				section += number
			}
			if section == 0 {
				section = 1
			}
			total += section * unit
			section = 0
			number = 0
			continue
		}
		if number == 0 {
			number = 1
		}
		section += number * unit
		number = 0
	}
	if !seen {
		return 0
	}
	return total + section + number
}

func groupDisplayTitle(name string) string {
	base := strings.TrimSuffix(name, filepath.Ext(name))
	matches := groupNamePattern.FindStringSubmatch(base)
	if len(matches) == 0 {
		return displayNameFromFilenameBase(base)
	}
	title := strings.Trim(matches[2], "-_ ")
	if title == "" {
		return "第" + matches[1] + "组细纲"
	}
	return matches[1] + " " + title
}

func chapterIndex(name string) int {
	meta, _ := parseChapterNameMeta(strings.TrimSuffix(name, filepath.Ext(name)))
	return meta.index
}

func chapterPlanIndex(path string) int {
	name := filepath.Base(path)
	matches := groupNamePattern.FindStringSubmatch(strings.TrimSuffix(name, filepath.Ext(name)))
	if len(matches) == 0 {
		return 0
	}
	n := 0
	for _, ch := range matches[1] {
		n = n*10 + int(ch-'0')
	}
	return n
}

func chapterVolume(relPath string) (string, string) {
	insideChapters := strings.TrimPrefix(relPath, "chapters/")
	parts := strings.Split(insideChapters, "/")
	if len(parts) <= 1 {
		return "未分卷", "chapters"
	}
	volume := strings.TrimSpace(parts[0])
	if volume == "" {
		return "未分卷", "chapters"
	}
	return volume, filepath.ToSlash(filepath.Join("chapters", filepath.FromSlash(volume)))
}

func documentTitle(content, fallbackTitle, relPath string) string {
	for _, line := range strings.Split(content, "\n") {
		text := strings.TrimSpace(line)
		if text == "" || !strings.HasPrefix(text, "#") {
			continue
		}
		title := strings.TrimSpace(strings.TrimLeft(text, "#"))
		if title != "" {
			return title
		}
	}
	if fallbackTitle != "" {
		return fallbackTitle
	}
	return strings.TrimSuffix(filepath.Base(relPath), filepath.Ext(relPath))
}

func documentExcerpt(content string) string {
	lines := []string{}
	for _, line := range strings.Split(content, "\n") {
		text := normalizePreviewLine(line)
		if text == "" {
			continue
		}
		lines = append(lines, text)
		if len(lines) >= 2 {
			break
		}
	}
	return truncateRunes(strings.Join(lines, " / "), 120)
}

func normalizePreviewLine(line string) string {
	text := strings.TrimSpace(line)
	if strings.HasPrefix(text, "#") {
		return ""
	}
	text = strings.TrimLeft(text, "-*+> 	")
	return strings.TrimSpace(text)
}

func truncateRunes(text string, limit int) string {
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return string(runes[:limit]) + "…"
}

func chapterStatus(words int) string {
	switch {
	case words == 0:
		return "空章"
	case words < 1500:
		return "草稿"
	case words < 5000:
		return "初稿"
	default:
		return "成章"
	}
}

func countWritingWords(content string) int {
	count := 0
	for _, ch := range content {
		if unicode.IsSpace(ch) {
			continue
		}
		count++
	}
	return count
}
