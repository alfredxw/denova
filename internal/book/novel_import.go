package book

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	NovelImportMaxPreviewChapters = 12
	NovelImportDefaultSampleChars = 20000
	NovelImportMinSampleChars     = 2000
	NovelImportMaxSampleChars     = 100000

	NovelImportSplitStrategyBuiltin = "builtin"
	NovelImportSplitStrategyLocal   = "local_regex"
	NovelImportSplitStrategyAgent   = "tool_agent_regex"
	NovelImportSplitStrategyCustom  = "custom_regex"

	NovelImportSingleChapterWarning       = "novel_import_single_chapter"
	NovelImportAgentFallbackWarning       = "novel_import_agent_fallback"
	NovelImportRegexFewChaptersWarning    = "novel_import_regex_few_chapters"
	NovelImportRegexFallbackWarningPrefix = "novel_import_regex_fallback:"
)

const (
	NovelImportLanguageChinese = "zh"
	NovelImportLanguageEnglish = "en"
	NovelImportLanguageUnknown = "unknown"
)

var (
	mdHeadingRe      = regexp.MustCompile(`^\s{0,3}#{1,6}\s+(.+?)\s*$`)
	cnChapterRe      = regexp.MustCompile(`^\s*第[0-9零〇一二三四五六七八九十百千万两]+[章节回集][^\n\r]{0,80}$`)
	cnVolumeRe       = regexp.MustCompile(`^\s*(?:第[0-9零〇一二三四五六七八九十百千万两]+[卷部]|卷[0-9零〇一二三四五六七八九十百千万两]+|[0-9零〇一二三四五六七八九十百千万两]+卷|[上下前后终][卷部])[^\n\r]{0,80}$`)
	prefaceTitleRe   = regexp.MustCompile(`^\s*(?:(?:序章|楔子|引子|前言|序幕|序言|尾声|后记|番外)(?:\s*[0-9零〇一二三四五六七八九十百千万两]*\s*(?:[:：、\-—.．]\s*)?[^\n\r]{0,80})?|序(?:\s*[:：、\-—.．]\s*[^\n\r]{0,80})?)\s*$`)
	enChapterRe      = regexp.MustCompile(`(?i)^\s*chapter\s+[0-9ivxlcdm]+[^\n\r]{0,80}$`)
	enVolumeRe       = regexp.MustCompile(`(?i)^\s*(?:part|volume)\s+[0-9ivxlcdm]+[^\n\r]{0,80}$`)
	enSpecialTitleRe = regexp.MustCompile(`(?i)^\s*(prologue|epilogue)\b`)
	numberedTitleRe  = regexp.MustCompile(`^\s*[0-9]{1,4}[\.、]\s*[^\n\r]{1,80}$`)
	blankLinePattern = regexp.MustCompile(`\n{3,}`)
)

type localChapterRegexCandidate struct {
	name    string
	pattern string
}

var localChapterRegexCandidates = []localChapterRegexCandidate{
	{name: "common_novel_title", pattern: `(?i)^\s*((?:第[0-9零〇一二三四五六七八九十百千万两]+[章节卷回部集]|卷[0-9零〇一二三四五六七八九十百千万两]+|[0-9零〇一二三四五六七八九十百千万两]+卷|[上下前后终][卷部]|序章|楔子|引子|前言|序幕|序言|尾声|后记|番外|chapter\s+[0-9ivxlcdm]+|part\s+[0-9ivxlcdm]+|volume\s+[0-9ivxlcdm]+)[^\n\r]{0,80})$`},
	{name: "markdown", pattern: `^\s{0,3}#{1,6}\s+(.+?)\s*$`},
	{name: "cn_chapter", pattern: `^\s*(第[0-9零〇一二三四五六七八九十百千万两]+[章节回集][^\n\r]{0,80})$`},
	{name: "cn_volume", pattern: `^\s*((?:第[0-9零〇一二三四五六七八九十百千万两]+[卷部]|卷[0-9零〇一二三四五六七八九十百千万两]+|[0-9零〇一二三四五六七八九十百千万两]+卷|[上下前后终][卷部])[^\n\r]{0,80})$`},
	{name: "preface_or_epilogue", pattern: `^\s*((?:(?:序章|楔子|引子|前言|序幕|序言|尾声|后记|番外)(?:\s*[0-9零〇一二三四五六七八九十百千万两]*\s*(?:[:：、\-—.．]\s*)?[^\n\r]{0,80})?|序(?:\s*[:：、\-—.．]\s*[^\n\r]{0,80})?))\s*$`},
	{name: "en_chapter", pattern: `(?i)^\s*(chapter\s+[0-9ivxlcdm]+[^\n\r]{0,80})$`},
	{name: "en_volume", pattern: `(?i)^\s*((?:part|volume)\s+[0-9ivxlcdm]+[^\n\r]{0,80})$`},
	{name: "numbered_title", pattern: `^\s*([0-9]{1,4}[\.、]\s*[^\n\r]{1,80})$`},
	{name: "cn_numbered_title", pattern: `^\s*([一二三四五六七八九十百千万两〇零]{1,8}[、\.．]\s*[^\n\r]{1,80})$`},
	{name: "bracket_numbered_title", pattern: `^\s*([（(]?[0-9一二三四五六七八九十百千万两〇零]{1,8}[）)]\s*[^\n\r]{1,80})$`},
}

// NovelImportOptions controls the confirmed chapter splitting flow.
type NovelImportOptions struct {
	SampleChars     int
	SplitStrategy   string
	SplitRegex      string
	InferSplitRegex func(sample string) (string, error)
	sourceExt       string
}

// NovelImportPreview describes the chapters parsed from an uploaded novel file.
type NovelImportPreview struct {
	Title                 string               `json:"title"`
	Language              string               `json:"language,omitempty"`
	ChapterFilenameFormat string               `json:"chapter_filename_format,omitempty"`
	SplitStrategy         string               `json:"split_strategy"`
	SplitRegex            string               `json:"split_regex"`
	SampleChars           int                  `json:"sample_chars"`
	ChapterCount          int                  `json:"chapter_count"`
	TotalChars            int                  `json:"total_chars"`
	Chapters              []NovelImportChapter `json:"chapters"`
	Warnings              []string             `json:"warnings,omitempty"`
}

// NovelImportChapter is a parsed source chapter.
type NovelImportChapter struct {
	Index      int    `json:"index"`
	Title      string `json:"title"`
	Chars      int    `json:"chars"`
	Path       string `json:"path,omitempty"`
	Volume     string `json:"volume,omitempty"`
	VolumePath string `json:"volume_path,omitempty"`
}

type parsedNovelChapter struct {
	NovelImportChapter
	Content string
}

type parsedNovel struct {
	Preview  NovelImportPreview
	Chapters []parsedNovelChapter
}

type novelImportMarker struct {
	line int
	info novelImportTitle
}

type novelImportTitleKind string

const (
	novelImportTitleChapter novelImportTitleKind = "chapter"
	novelImportTitleVolume  novelImportTitleKind = "volume"
)

type novelImportTitle struct {
	title string
	kind  novelImportTitleKind
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
func PreviewNovelImport(filename string, data []byte, opts ...NovelImportOptions) (NovelImportPreview, error) {
	parsed, err := parseNovelImport(filename, data, mergeNovelImportOptions(opts...))
	if err != nil {
		return NovelImportPreview{}, err
	}
	return parsed.Preview, nil
}

// ImportNovelToWorkspace writes parsed txt/md chapters into an initialized workspace.
func ImportNovelToWorkspace(workspace, filename string, data []byte, opts ...NovelImportOptions) (NovelImportPreview, []string, error) {
	parsed, err := parseNovelImport(filename, data, mergeNovelImportOptions(opts...))
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
			rel = chapterPath(chapter.Index, chapter.Title, chapter.Volume, parsed.Preview.Language)
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

func parseNovelImport(filename string, data []byte, opts NovelImportOptions) (parsedNovel, error) {
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
	opts.sourceExt = ext
	language := detectNovelImportLanguage(text)
	chapterFilenameFormat := chapterFilenameFormatForLanguage(language)

	title := strings.TrimSuffix(filepath.Base(name), filepath.Ext(name))
	log.Printf("[novel-import] parse begin filename=%q bytes=%d text_chars=%d sample_chars=%d requested_strategy=%q has_split_regex=%t", name, len(data), utf8.RuneCountInString(text), opts.SampleChars, opts.SplitStrategy, opts.SplitRegex != "")
	chapters, splitStrategy, splitRegex, warnings, err := splitNovelChaptersWithOptions(text, opts)
	if err != nil {
		log.Printf("[novel-import] parse failed filename=%q err=%v", name, err)
		return parsedNovel{}, err
	}
	totalChars := 0
	for i := range chapters {
		if ext == ".txt" {
			chapters[i].Content = formatPlainTextChapterForMarkdown(chapters[i].Content)
		}
		chapters[i].Index = i + 1
		chapters[i].Path = chapterPath(chapters[i].Index, chapters[i].Title, chapters[i].Volume, language)
		if chapters[i].Volume != "" {
			chapters[i].VolumePath = volumePath(chapters[i].Volume)
		}
		chapters[i].Chars = utf8.RuneCountInString(chapters[i].Content)
		totalChars += chapters[i].Chars
	}

	if len(chapters) == 1 {
		warnings = append(warnings, NovelImportSingleChapterWarning)
	}
	log.Printf("[novel-import] parse done filename=%q strategy=%s regex=%q chapters=%d total_chars=%d warnings=%v", name, splitStrategy, splitRegex, len(chapters), totalChars, warnings)
	previewChapters := make([]NovelImportChapter, 0, minInt(len(chapters), NovelImportMaxPreviewChapters))
	for i := 0; i < len(chapters) && i < NovelImportMaxPreviewChapters; i++ {
		previewChapters = append(previewChapters, chapters[i].NovelImportChapter)
	}
	return parsedNovel{
		Preview: NovelImportPreview{
			Title:                 title,
			Language:              language,
			ChapterFilenameFormat: chapterFilenameFormat,
			SplitStrategy:         splitStrategy,
			SplitRegex:            splitRegex,
			SampleChars:           opts.SampleChars,
			ChapterCount:          len(chapters),
			TotalChars:            totalChars,
			Chapters:              previewChapters,
			Warnings:              warnings,
		},
		Chapters: chapters,
	}, nil
}

func mergeNovelImportOptions(opts ...NovelImportOptions) NovelImportOptions {
	out := NovelImportOptions{SampleChars: NovelImportDefaultSampleChars}
	if len(opts) > 0 {
		out = opts[0]
	}
	out.SampleChars = normalizeNovelImportSampleChars(out.SampleChars)
	out.SplitStrategy = strings.TrimSpace(out.SplitStrategy)
	out.SplitRegex = strings.TrimSpace(out.SplitRegex)
	return out
}

func normalizeNovelImportSampleChars(value int) int {
	if value <= 0 {
		return NovelImportDefaultSampleChars
	}
	if value < NovelImportMinSampleChars {
		return NovelImportMinSampleChars
	}
	if value > NovelImportMaxSampleChars {
		return NovelImportMaxSampleChars
	}
	return value
}

func normalizeNovelText(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	text = strings.TrimPrefix(text, "\ufeff")
	text = blankLinePattern.ReplaceAllString(text, "\n\n")
	return strings.TrimSpace(text)
}

func formatPlainTextChapterForMarkdown(content string) string {
	lines := strings.Split(strings.TrimSpace(content), "\n")
	paragraphs := make([]string, 0, len(lines))
	for _, line := range lines {
		line = normalizePlainTextLineForMarkdown(line)
		if strings.TrimSpace(line) == "" {
			continue
		}
		paragraphs = append(paragraphs, line)
	}
	if len(paragraphs) == 0 {
		return "\n"
	}
	return strings.Join(paragraphs, "\n\n") + "\n"
}

func normalizePlainTextLineForMarkdown(line string) string {
	line = strings.TrimRight(line, " \t")
	line = strings.TrimLeft(line, " \t")
	if line == "" {
		return ""
	}
	if strings.HasPrefix(line, "```") || strings.HasPrefix(line, "~~~") {
		return "\\" + line
	}
	switch line[0] {
	case '#', '>':
		return "\\" + line
	case '-', '+', '*':
		if len(line) == 1 || line[1] == ' ' || line[1] == '\t' || line[1] == line[0] {
			return "\\" + line
		}
	}
	for i := 0; i < len(line); i++ {
		ch := line[i]
		if ch >= '0' && ch <= '9' {
			continue
		}
		if i > 0 && (ch == '.' || ch == ')') && i+1 < len(line) && (line[i+1] == ' ' || line[i+1] == '\t') {
			return line[:i] + "\\" + line[i:]
		}
		break
	}
	return line
}

func splitNovelChaptersWithOptions(text string, opts NovelImportOptions) ([]parsedNovelChapter, string, string, []string, error) {
	warnings := []string{}
	if opts.SplitRegex != "" {
		log.Printf("[novel-import] split using provided regex strategy=%q regex=%q", opts.SplitStrategy, opts.SplitRegex)
		chapters, err := splitNovelChaptersByRegex(text, opts.SplitRegex)
		if err != nil {
			log.Printf("[novel-import] provided regex invalid regex=%q err=%v", opts.SplitRegex, err)
			return nil, "", "", nil, err
		}
		if len(chapters) < 2 {
			log.Printf("[novel-import] provided regex matched fewer than 2 chapters regex=%q chapters=%d", opts.SplitRegex, len(chapters))
			return nil, "", "", nil, fmt.Errorf("自定义章节正则至少需要识别 2 个章节标题")
		}
		strategy := NovelImportSplitStrategyCustom
		if opts.SplitStrategy == NovelImportSplitStrategyAgent {
			strategy = NovelImportSplitStrategyAgent
		}
		return chapters, strategy, opts.SplitRegex, warnings, nil
	}
	if opts.SplitStrategy == NovelImportSplitStrategyBuiltin {
		chapters := splitNovelChapters(text, novelImportAllowMarkdownHeadings(opts.sourceExt))
		log.Printf("[novel-import] split using builtin by request chapters=%d", len(chapters))
		return chapters, NovelImportSplitStrategyBuiltin, "", warnings, nil
	}
	if opts.InferSplitRegex != nil {
		sample := firstRunes(text, opts.SampleChars)
		if opts.SplitStrategy != NovelImportSplitStrategyAgent {
			if regex, ok := inferLocalChapterSplitRegex(sample, novelImportAllowMarkdownHeadings(opts.sourceExt)); ok {
				log.Printf("[novel-import] local regex inferred regex=%q", regex)
				chapters, splitErr := splitNovelChaptersByRegex(text, regex)
				if splitErr == nil && len(chapters) >= 2 {
					return chapters, NovelImportSplitStrategyLocal, regex, warnings, nil
				}
				log.Printf("[novel-import] local regex rejected regex=%q chapters=%d err=%v; continue=tool_agent", regex, len(chapters), splitErr)
			}
		} else {
			log.Printf("[novel-import] skip local regex inference by requested strategy=%q", opts.SplitStrategy)
		}
		agentContext := buildChapterSplitInferenceContext(sample)
		log.Printf("[novel-import] split requesting tool agent regex sample_chars=%d sample_lines=%d context_chars=%d context_lines=%d", utf8.RuneCountInString(sample), len(strings.Split(sample, "\n")), utf8.RuneCountInString(agentContext), len(strings.Split(agentContext, "\n")))
		regex, err := opts.InferSplitRegex(agentContext)
		if err != nil {
			log.Printf("[novel-import] tool agent regex inference failed err=%v; fallback=builtin", err)
			warnings = append(warnings, NovelImportAgentFallbackWarning)
			return splitNovelChapters(text, novelImportAllowMarkdownHeadings(opts.sourceExt)), NovelImportSplitStrategyBuiltin, "", warnings, nil
		}
		regex = strings.TrimSpace(regex)
		log.Printf("[novel-import] tool agent regex inferred regex=%q", regex)
		chapters, splitErr := splitNovelChaptersByRegex(text, regex)
		if splitErr != nil {
			log.Printf("[novel-import] tool agent regex split failed regex=%q err=%v; fallback=builtin", regex, splitErr)
			warnings = append(warnings, NovelImportRegexFallbackWarningPrefix+splitErr.Error())
			return splitNovelChapters(text, novelImportAllowMarkdownHeadings(opts.sourceExt)), NovelImportSplitStrategyBuiltin, "", warnings, nil
		}
		if len(chapters) < 2 {
			log.Printf("[novel-import] tool agent regex matched fewer than 2 chapters regex=%q chapters=%d; fallback=builtin", regex, len(chapters))
			warnings = append(warnings, NovelImportRegexFewChaptersWarning)
			return splitNovelChapters(text, novelImportAllowMarkdownHeadings(opts.sourceExt)), NovelImportSplitStrategyBuiltin, "", warnings, nil
		}
		return chapters, NovelImportSplitStrategyAgent, regex, warnings, nil
	}
	chapters := splitNovelChapters(text, novelImportAllowMarkdownHeadings(opts.sourceExt))
	log.Printf("[novel-import] split using builtin no_tool_agent chapters=%d", len(chapters))
	return chapters, NovelImportSplitStrategyBuiltin, "", warnings, nil
}

func splitNovelChapters(text string, allowMarkdownHeadings bool) []parsedNovelChapter {
	lines := strings.Split(text, "\n")
	markers := []novelImportMarker{}
	for i, line := range lines {
		if info, ok := novelImportLineTitle(line, allowMarkdownHeadings); ok {
			markers = append(markers, novelImportMarker{line: i, info: info})
		}
	}
	if len(markers) == 0 {
		return []parsedNovelChapter{{
			NovelImportChapter: NovelImportChapter{Title: "正文"},
			Content:            text + "\n",
		}}
	}
	chapters, err := chaptersFromMarkers(lines, markers)
	if err != nil {
		return []parsedNovelChapter{{
			NovelImportChapter: NovelImportChapter{Title: "正文"},
			Content:            text + "\n",
		}}
	}
	return chapters
}

func splitNovelChaptersByRegex(text, pattern string) ([]parsedNovelChapter, error) {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return nil, fmt.Errorf("章节正则不能为空")
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("章节正则无效: %w", err)
	}
	lines := strings.Split(text, "\n")
	markers := []novelImportMarker{}
	for i, line := range lines {
		matches := re.FindStringSubmatch(line)
		if len(matches) == 0 {
			continue
		}
		title := strings.TrimSpace(matches[0])
		if len(matches) > 1 && strings.TrimSpace(matches[1]) != "" {
			title = strings.TrimSpace(matches[1])
		}
		if title == "" {
			continue
		}
		title = strings.Trim(title, "# \t")
		markers = append(markers, novelImportMarker{line: i, info: classifyNovelImportTitle(title)})
	}
	log.Printf("[novel-import] regex marker scan regex=%q markers=%d first_titles=%q", pattern, len(markers), markerTitlePreview(markers, 5))
	if len(markers) == 0 {
		return []parsedNovelChapter{{
			NovelImportChapter: NovelImportChapter{Title: "正文"},
			Content:            text + "\n",
		}}, nil
	}
	return chaptersFromMarkers(lines, markers)
}

func inferLocalChapterSplitRegex(sample string, allowMarkdownHeadings bool) (string, bool) {
	type result struct {
		candidate localChapterRegexCandidate
		markers   []novelImportMarker
	}
	results := []result{}
	for _, candidate := range localChapterRegexCandidates {
		if candidate.name == "markdown" && !allowMarkdownHeadings {
			continue
		}
		markers, err := scanRegexMarkers(sample, candidate.pattern)
		if err != nil {
			log.Printf("[novel-import] local regex candidate invalid name=%s regex=%q err=%v", candidate.name, candidate.pattern, err)
			continue
		}
		if len(markers) < 2 {
			continue
		}
		results = append(results, result{candidate: candidate, markers: markers})
	}
	if len(results) == 0 {
		log.Printf("[novel-import] local regex inference found no stable candidate")
		return "", false
	}
	best := results[0]
	for _, current := range results[1:] {
		if len(current.markers) > len(best.markers) {
			best = current
		}
	}
	log.Printf("[novel-import] local regex candidate selected name=%s regex=%q markers=%d first_titles=%q", best.candidate.name, best.candidate.pattern, len(best.markers), markerTitlePreview(best.markers, 5))
	return best.candidate.pattern, true
}

func scanRegexMarkers(text, pattern string) ([]novelImportMarker, error) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(text, "\n")
	markers := []novelImportMarker{}
	for i, line := range lines {
		matches := re.FindStringSubmatch(line)
		if len(matches) == 0 {
			continue
		}
		title := strings.TrimSpace(matches[0])
		if len(matches) > 1 && strings.TrimSpace(matches[1]) != "" {
			title = strings.TrimSpace(matches[1])
		}
		title = strings.Trim(title, "# \t")
		if !isLikelyStandaloneChapterTitle(title) {
			continue
		}
		markers = append(markers, novelImportMarker{line: i, info: classifyNovelImportTitle(title)})
	}
	return markers, nil
}

func buildChapterSplitInferenceContext(sample string) string {
	lines := strings.Split(sample, "\n")
	var sb strings.Builder
	sb.WriteString("以下是从小说样本中抽取的短行候选，不是完整正文。请只基于这些候选判断章节/分卷标题行正则。\n")
	sb.WriteString("候选行：\n")
	count := 0
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !isPotentialTitleContextLine(trimmed) {
			continue
		}
		sb.WriteString(fmt.Sprintf("%d: %s\n", i+1, trimmed))
		count++
		if count >= 160 {
			break
		}
	}
	if count == 0 {
		sb.WriteString("（未抽取到短行候选，下面提供样本开头片段）\n")
		sb.WriteString(firstRunes(sample, 3000))
	}
	return sb.String()
}

func isPotentialTitleContextLine(line string) bool {
	if line == "" {
		return false
	}
	runes := utf8.RuneCountInString(line)
	if runes > 100 {
		return false
	}
	if isLikelyStandaloneChapterTitle(line) {
		return true
	}
	if strings.ContainsAny(line, "。！？；!?;") {
		return false
	}
	return strings.ContainsAny(line, "#第章节卷回部集序楔子引前言幕尾声后记番外上下前后终ChapterchapterPARTPartVolumevolume0123456789一二三四五六七八九十百千万〇零、．.")
}

func isLikelyStandaloneChapterTitle(title string) bool {
	title = strings.TrimSpace(title)
	if title == "" {
		return false
	}
	if utf8.RuneCountInString(title) > 100 {
		return false
	}
	return !strings.ContainsAny(title, "。！？；!?;")
}

func markerTitlePreview(markers []novelImportMarker, limit int) string {
	if limit <= 0 || len(markers) == 0 {
		return ""
	}
	titles := make([]string, 0, minInt(len(markers), limit))
	for i := 0; i < len(markers) && i < limit; i++ {
		titles = append(titles, markers[i].info.title)
	}
	return strings.Join(titles, " | ")
}

func chaptersFromMarkers(lines []string, markers []novelImportMarker) ([]parsedNovelChapter, error) {
	chapters := []parsedNovelChapter{}
	currentVolume := ""
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
		if current.info.kind == novelImportTitleVolume {
			currentVolume = current.info.title
			intro := strings.TrimSpace(strings.Join(lines[current.line+1:end], "\n"))
			if intro == "" {
				continue
			}
			chapters = append(chapters, parsedNovelChapter{
				NovelImportChapter: NovelImportChapter{Title: current.info.title, Volume: currentVolume},
				Content:            strings.TrimSpace(strings.Join(lines[current.line:end], "\n")) + "\n",
			})
			continue
		}
		content := strings.TrimSpace(strings.Join(lines[current.line:end], "\n"))
		if content == "" {
			continue
		}
		chapters = append(chapters, parsedNovelChapter{
			NovelImportChapter: NovelImportChapter{Title: current.info.title, Volume: currentVolume},
			Content:            content + "\n",
		})
	}
	if len(chapters) == 0 {
		return []parsedNovelChapter{{
			NovelImportChapter: NovelImportChapter{Title: "正文"},
			Content:            strings.TrimSpace(strings.Join(lines, "\n")) + "\n",
		}}, nil
	}
	return chapters, nil
}

func novelImportLineTitle(line string, allowMarkdownHeading bool) (novelImportTitle, bool) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return novelImportTitle{}, false
	}
	if allowMarkdownHeading {
		if matches := mdHeadingRe.FindStringSubmatch(trimmed); len(matches) == 2 {
			title := strings.TrimSpace(matches[1])
			return classifyNovelImportTitle(title), true
		}
	}
	if matches := mdHeadingRe.FindStringSubmatch(trimmed); len(matches) == 2 {
		title := strings.TrimSpace(matches[1])
		if cnChapterRe.MatchString(title) || cnVolumeRe.MatchString(title) || prefaceTitleRe.MatchString(title) || enChapterRe.MatchString(title) || enVolumeRe.MatchString(title) {
			return classifyNovelImportTitle(title), true
		}
	}
	if (cnVolumeRe.MatchString(trimmed) || enVolumeRe.MatchString(trimmed)) && !hasSentencePunctuation(trimmed) {
		return novelImportTitle{title: strings.Trim(trimmed, "# \t"), kind: novelImportTitleVolume}, true
	}
	if (cnChapterRe.MatchString(trimmed) || prefaceTitleRe.MatchString(trimmed)) && !hasSentencePunctuation(trimmed) {
		return novelImportTitle{title: strings.Trim(trimmed, "# \t"), kind: novelImportTitleChapter}, true
	}
	if enChapterRe.MatchString(trimmed) || numberedTitleRe.MatchString(trimmed) {
		return novelImportTitle{title: strings.Trim(trimmed, "# \t"), kind: novelImportTitleChapter}, true
	}
	return novelImportTitle{}, false
}

func novelImportAllowMarkdownHeadings(sourceExt string) bool {
	return sourceExt == ".md" || sourceExt == ".markdown"
}

func classifyNovelImportTitle(title string) novelImportTitle {
	title = strings.Trim(title, "# \t")
	if (cnVolumeRe.MatchString(title) || enVolumeRe.MatchString(title)) && !hasSentencePunctuation(title) {
		return novelImportTitle{title: title, kind: novelImportTitleVolume}
	}
	return novelImportTitle{title: title, kind: novelImportTitleChapter}
}

func hasSentencePunctuation(s string) bool {
	return strings.ContainsAny(s, "。！？；，,.!?;")
}

func firstRunes(value string, max int) string {
	if max <= 0 || utf8.RuneCountInString(value) <= max {
		return value
	}
	runes := []rune(value)
	return string(runes[:max])
}

func chapterPath(index int, title, volume, language string) string {
	filename := chapterFilename(index, title, language)
	if strings.TrimSpace(volume) == "" {
		return filepath.ToSlash(filepath.Join("chapters", filename))
	}
	return filepath.ToSlash(filepath.Join(volumePath(volume), filename))
}

func chapterFilename(index int, title, language string) string {
	cleanTitle := safeFilenamePart(title)
	if cleanTitle == "" {
		cleanTitle = "chapter"
	}
	switch language {
	case NovelImportLanguageChinese:
		if isChineseChapterLikeTitle(title) {
			return cleanTitle + ".md"
		}
		return fmt.Sprintf("第%d章-%s.md", index, cleanTitle)
	case NovelImportLanguageEnglish:
		if isEnglishChapterLikeTitle(title) {
			return cleanTitle + ".md"
		}
		return fmt.Sprintf("Chapter-%d-%s.md", index, cleanTitle)
	default:
		if isChineseChapterLikeTitle(title) {
			return cleanTitle + ".md"
		}
		return fmt.Sprintf("第%d章-%s.md", index, cleanTitle)
	}
}

func volumePath(volume string) string {
	cleanVolume := safeFilenamePart(volume)
	if cleanVolume == "" {
		return "chapters"
	}
	return filepath.ToSlash(filepath.Join("chapters", cleanVolume))
}

func detectNovelImportLanguage(text string) string {
	han := 0
	latin := 0
	for _, r := range text {
		switch {
		case unicode.Is(unicode.Han, r):
			han++
		case (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z'):
			latin++
		}
		if han+latin >= 4000 {
			break
		}
	}
	if han > 0 && han*4 >= latin {
		return NovelImportLanguageChinese
	}
	if latin > 0 {
		return NovelImportLanguageEnglish
	}
	return NovelImportLanguageUnknown
}

func chapterFilenameFormatForLanguage(language string) string {
	switch language {
	case NovelImportLanguageChinese:
		return "第{N}章-{title}.md"
	case NovelImportLanguageEnglish:
		return "Chapter {N} - {title}.md"
	default:
		return "第{N}章-{title}.md"
	}
}

func isChineseChapterLikeTitle(title string) bool {
	title = strings.TrimSpace(title)
	return cnChapterRe.MatchString(title) || cnVolumeRe.MatchString(title) || prefaceTitleRe.MatchString(title)
}

func isEnglishChapterLikeTitle(title string) bool {
	title = strings.TrimSpace(title)
	return enChapterRe.MatchString(title) || enVolumeRe.MatchString(title) || enSpecialTitleRe.MatchString(title)
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
