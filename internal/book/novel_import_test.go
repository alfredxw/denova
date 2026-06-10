package book

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPreviewNovelImportSplitsMarkdownAndChineseChapters(t *testing.T) {
	data := []byte(`# 序章

开场。

第一章 风起

第一章正文。

## 第二章 雨落

第二章正文。`)

	preview, err := PreviewNovelImport("长夜.md", data)
	if err != nil {
		t.Fatalf("PreviewNovelImport failed: %v", err)
	}
	if preview.Title != "长夜" {
		t.Fatalf("title = %q", preview.Title)
	}
	if preview.ChapterCount != 3 {
		t.Fatalf("chapter count = %d, chapters=%#v", preview.ChapterCount, preview.Chapters)
	}
	if preview.Chapters[0].Title != "序章" || preview.Chapters[1].Title != "第一章 风起" || preview.Chapters[2].Title != "第二章 雨落" {
		t.Fatalf("unexpected chapter titles: %#v", preview.Chapters)
	}
}

func TestPreviewNovelImportDetectsVolumeBoundaries(t *testing.T) {
	data := []byte(`序章
开场。

卷一 蓝天之上
卷正文。

第一章 起飞
正文。

尾声
结束。`)

	preview, err := PreviewNovelImport("蓝天.txt", data)
	if err != nil {
		t.Fatalf("PreviewNovelImport failed: %v", err)
	}
	if preview.ChapterCount != 4 {
		t.Fatalf("chapter count = %d, chapters=%#v", preview.ChapterCount, preview.Chapters)
	}
	want := []string{"序章", "卷一 蓝天之上", "第一章 起飞", "尾声"}
	for i, title := range want {
		if preview.Chapters[i].Title != title {
			t.Fatalf("chapter %d title = %q, want %q; chapters=%#v", i, preview.Chapters[i].Title, title, preview.Chapters)
		}
	}
	if preview.Chapters[1].Volume != "卷一 蓝天之上" || preview.Chapters[2].Volume != "卷一 蓝天之上" {
		t.Fatalf("unexpected chapter volumes: %#v", preview.Chapters)
	}
}

func TestImportNovelToWorkspaceWritesChapters(t *testing.T) {
	dir := t.TempDir()
	if err := NewState(dir).InitWorkspace(); err != nil {
		t.Fatalf("InitWorkspace failed: %v", err)
	}
	data := []byte("第一章 开始\n\n内容一\n\n第二章 继续\n\n内容二")

	preview, paths, err := ImportNovelToWorkspace(dir, "测试.txt", data)
	if err != nil {
		t.Fatalf("ImportNovelToWorkspace failed: %v", err)
	}
	if preview.ChapterCount != 2 || len(paths) != 2 {
		t.Fatalf("unexpected import result preview=%#v paths=%#v", preview, paths)
	}
	for _, rel := range paths {
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(rel))); err != nil {
			t.Fatalf("missing imported chapter %s: %v", rel, err)
		}
	}
}

func TestImportNovelToWorkspaceFormatsTxtLineBreaksForMarkdown(t *testing.T) {
	dir := t.TempDir()
	if err := NewState(dir).InitWorkspace(); err != nil {
		t.Fatalf("InitWorkspace failed: %v", err)
	}
	data := []byte("第一章 起飞\n第一行没有空行\n第二行也没有空行\n第二章 巡航\n第三行")

	_, paths, err := ImportNovelToWorkspace(dir, "蓝天.txt", data)
	if err != nil {
		t.Fatalf("ImportNovelToWorkspace failed: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(paths[0])))
	if err != nil {
		t.Fatalf("read imported chapter failed: %v", err)
	}
	want := "第一章 起飞\n\n第一行没有空行\n\n第二行也没有空行\n"
	if string(got) != want {
		t.Fatalf("chapter content = %q, want %q", string(got), want)
	}
}

func TestImportNovelToWorkspaceAvoidsIndentedCodeBlocksForTxt(t *testing.T) {
	dir := t.TempDir()
	if err := NewState(dir).InitWorkspace(); err != nil {
		t.Fatalf("InitWorkspace failed: %v", err)
	}
	data := []byte("第一章 起飞\n    四空格缩进不会变代码块\n\tTab 缩进也不会变代码块\n第二章 巡航\n正文")

	_, paths, err := ImportNovelToWorkspace(dir, "蓝天.txt", data)
	if err != nil {
		t.Fatalf("ImportNovelToWorkspace failed: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(paths[0])))
	if err != nil {
		t.Fatalf("read imported chapter failed: %v", err)
	}
	want := "第一章 起飞\n\n四空格缩进不会变代码块\n\nTab 缩进也不会变代码块\n"
	if string(got) != want {
		t.Fatalf("chapter content = %q, want %q", string(got), want)
	}
}

func TestNormalizePlainTextLineForMarkdownEscapesBlockSyntax(t *testing.T) {
	tests := map[string]string{
		"> 不会变引用":    "\\> 不会变引用",
		"# 不会变标题":    "\\# 不会变标题",
		"- 不会变列表":    "\\- 不会变列表",
		"1. 不会变列表":   "1\\. 不会变列表",
		"```不会变围栏":   "\\```不会变围栏",
		"    不会变代码块": "不会变代码块",
		"\t也不会变代码块":  "也不会变代码块",
		"　全角缩进保留为正文": "　全角缩进保留为正文",
	}
	for in, want := range tests {
		if got := normalizePlainTextLineForMarkdown(in); got != want {
			t.Fatalf("normalizePlainTextLineForMarkdown(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestImportNovelToWorkspaceWritesChaptersIntoVolumes(t *testing.T) {
	dir := t.TempDir()
	if err := NewState(dir).InitWorkspace(); err != nil {
		t.Fatalf("InitWorkspace failed: %v", err)
	}
	data := []byte(`第一卷 蓝天之上

第一章 起飞
内容一

第二章 巡航
内容二

第二卷 风暴航线

第三章 穿云
内容三`)

	preview, paths, err := ImportNovelToWorkspace(dir, "蓝天.txt", data)
	if err != nil {
		t.Fatalf("ImportNovelToWorkspace failed: %v", err)
	}
	if preview.ChapterCount != 3 || len(paths) != 3 {
		t.Fatalf("unexpected import result preview=%#v paths=%#v", preview, paths)
	}
	wantPaths := []string{
		"chapters/第一卷-蓝天之上/第一章-起飞.md",
		"chapters/第一卷-蓝天之上/第二章-巡航.md",
		"chapters/第二卷-风暴航线/第三章-穿云.md",
	}
	for i, want := range wantPaths {
		if paths[i] != want {
			t.Fatalf("path %d = %q, want %q; paths=%#v", i, paths[i], want, paths)
		}
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(want))); err != nil {
			t.Fatalf("missing imported chapter %s: %v", want, err)
		}
	}
	if preview.Chapters[0].Volume != "第一卷 蓝天之上" || preview.Chapters[0].VolumePath != "chapters/第一卷-蓝天之上" {
		t.Fatalf("unexpected preview volume metadata: %#v", preview.Chapters[0])
	}
}

func TestImportNovelToWorkspaceUsesChineseChapterFilenameStyle(t *testing.T) {
	dir := t.TempDir()
	if err := NewState(dir).InitWorkspace(); err != nil {
		t.Fatalf("InitWorkspace failed: %v", err)
	}
	data := []byte("第一章 缘起\n内容一\n\n第二章 风起\n内容二")

	preview, paths, err := ImportNovelToWorkspace(dir, "中文.txt", data)
	if err != nil {
		t.Fatalf("ImportNovelToWorkspace failed: %v", err)
	}
	if preview.Language != NovelImportLanguageChinese {
		t.Fatalf("language = %q", preview.Language)
	}
	if preview.ChapterFilenameFormat != "第{N}章-{title}.md" {
		t.Fatalf("chapter filename format = %q", preview.ChapterFilenameFormat)
	}
	want := []string{"chapters/第一章-缘起.md", "chapters/第二章-风起.md"}
	for i := range want {
		if paths[i] != want[i] {
			t.Fatalf("path %d = %q, want %q; paths=%#v", i, paths[i], want[i], paths)
		}
	}
}

func TestImportNovelToWorkspaceUsesEnglishChapterFilenameStyle(t *testing.T) {
	dir := t.TempDir()
	if err := NewState(dir).InitWorkspace(); err != nil {
		t.Fatalf("InitWorkspace failed: %v", err)
	}
	data := []byte("Chapter 1 Origin\nThe first line.\n\nChapter 2 Flight\nThe second line.")

	preview, paths, err := ImportNovelToWorkspace(dir, "english.txt", data)
	if err != nil {
		t.Fatalf("ImportNovelToWorkspace failed: %v", err)
	}
	if preview.Language != NovelImportLanguageEnglish {
		t.Fatalf("language = %q", preview.Language)
	}
	if preview.ChapterFilenameFormat != "Chapter {N} - {title}.md" {
		t.Fatalf("chapter filename format = %q", preview.ChapterFilenameFormat)
	}
	want := []string{"chapters/Chapter-1-Origin.md", "chapters/Chapter-2-Flight.md"}
	for i := range want {
		if paths[i] != want[i] {
			t.Fatalf("path %d = %q, want %q; paths=%#v", i, paths[i], want[i], paths)
		}
	}
}

func TestPreviewNovelImportUsesLocalRegexBeforeAgentForCommonTitles(t *testing.T) {
	data := []byte(`序章
开场。

卷一 蓝天之上
卷正文。

第一章 起飞
正文。

尾声
结束。`)

	preview, err := PreviewNovelImport("蓝天.txt", data, NovelImportOptions{
		InferSplitRegex: func(string) (string, error) {
			t.Fatalf("common title formats should be handled locally before calling tool agent")
			return "", nil
		},
	})
	if err != nil {
		t.Fatalf("PreviewNovelImport failed: %v", err)
	}
	if preview.SplitStrategy != NovelImportSplitStrategyLocal || preview.SplitRegex == "" {
		t.Fatalf("unexpected split metadata: %#v", preview)
	}
	if preview.ChapterCount != 4 || preview.Chapters[0].Title != "序章" || preview.Chapters[1].Title != "卷一 蓝天之上" || preview.Chapters[1].Volume != "卷一 蓝天之上" || preview.Chapters[3].Title != "尾声" {
		t.Fatalf("unexpected chapters: %#v", preview.Chapters)
	}
}

func TestPreviewNovelImportCanForceAgentRegexForCommonTitles(t *testing.T) {
	data := []byte(`序章
开场。

卷一 蓝天之上
卷正文。

第一章 起飞
正文。`)
	called := false

	preview, err := PreviewNovelImport("蓝天.txt", data, NovelImportOptions{
		SplitStrategy: NovelImportSplitStrategyAgent,
		InferSplitRegex: func(sample string) (string, error) {
			called = true
			if sample == "" {
				t.Fatalf("sample should not be empty")
			}
			return `^\s*((?:序章|卷一|第一章)[^\n\r]{0,80})$`, nil
		},
	})
	if err != nil {
		t.Fatalf("PreviewNovelImport failed: %v", err)
	}
	if !called {
		t.Fatalf("expected forced agent preview to call infer function")
	}
	if preview.SplitStrategy != NovelImportSplitStrategyAgent {
		t.Fatalf("strategy = %s", preview.SplitStrategy)
	}
	if preview.ChapterCount != 3 {
		t.Fatalf("chapter count = %d, chapters=%#v", preview.ChapterCount, preview.Chapters)
	}
	if preview.Chapters[1].Volume != "卷一 蓝天之上" {
		t.Fatalf("expected forced agent result to preserve volume metadata: %#v", preview.Chapters)
	}
}

func TestPreviewNovelImportUsesInferredRegex(t *testing.T) {
	data := []byte("== 开端 ==\n内容一\n\n== 转折 ==\n内容二")
	called := false
	preview, err := PreviewNovelImport("异形标题.txt", data, NovelImportOptions{
		SampleChars: 3000,
		InferSplitRegex: func(sample string) (string, error) {
			called = true
			if sample == "" {
				t.Fatalf("sample should not be empty")
			}
			return `^==\s*(.+?)\s*==$`, nil
		},
	})
	if err != nil {
		t.Fatalf("PreviewNovelImport failed: %v", err)
	}
	if !called {
		t.Fatalf("expected infer function to be called")
	}
	if preview.SplitStrategy != NovelImportSplitStrategyAgent || preview.SplitRegex == "" {
		t.Fatalf("unexpected split metadata: %#v", preview)
	}
	if preview.ChapterCount != 2 || preview.Chapters[0].Title != "开端" || preview.Chapters[1].Title != "转折" {
		t.Fatalf("unexpected chapters: %#v", preview.Chapters)
	}
	if preview.SampleChars != 3000 {
		t.Fatalf("sample chars = %d", preview.SampleChars)
	}
}

func TestPreviewNovelImportCustomRegexSkipsInfer(t *testing.T) {
	data := []byte("@@ A\n内容一\n\n@@ B\n内容二")
	preview, err := PreviewNovelImport("自定义.txt", data, NovelImportOptions{
		SplitRegex: `^@@\s*(.+)$`,
		InferSplitRegex: func(string) (string, error) {
			t.Fatalf("custom regex should skip infer")
			return "", nil
		},
	})
	if err != nil {
		t.Fatalf("PreviewNovelImport failed: %v", err)
	}
	if preview.SplitStrategy != NovelImportSplitStrategyCustom {
		t.Fatalf("strategy = %s", preview.SplitStrategy)
	}
	if preview.ChapterCount != 2 {
		t.Fatalf("chapter count = %d", preview.ChapterCount)
	}
}

func TestPreviewNovelImportRejectsInvalidCustomRegex(t *testing.T) {
	if _, err := PreviewNovelImport("bad.txt", []byte("正文"), NovelImportOptions{SplitRegex: `(`}); err == nil {
		t.Fatalf("expected invalid custom regex error")
	}
	if _, err := PreviewNovelImport("few.txt", []byte("A\n正文"), NovelImportOptions{SplitRegex: `^A$`}); err == nil {
		t.Fatalf("expected custom regex with fewer than 2 chapters to fail")
	}
}

func TestPreviewNovelImportFallsBackWhenInferFails(t *testing.T) {
	data := []byte("== 开始 ==\n内容一\n\n== 继续 ==\n内容二")
	preview, err := PreviewNovelImport("回退.txt", data, NovelImportOptions{
		InferSplitRegex: func(string) (string, error) {
			return "", os.ErrNotExist
		},
	})
	if err != nil {
		t.Fatalf("PreviewNovelImport failed: %v", err)
	}
	if preview.SplitStrategy != NovelImportSplitStrategyBuiltin {
		t.Fatalf("strategy = %s", preview.SplitStrategy)
	}
	if preview.ChapterCount != 1 {
		t.Fatalf("chapter count = %d", preview.ChapterCount)
	}
	if len(preview.Warnings) == 0 || preview.Warnings[0] != NovelImportAgentFallbackWarning {
		t.Fatalf("warnings = %#v", preview.Warnings)
	}
}

func TestImportNovelToWorkspaceUsesConfirmedRegex(t *testing.T) {
	dir := t.TempDir()
	if err := NewState(dir).InitWorkspace(); err != nil {
		t.Fatalf("InitWorkspace failed: %v", err)
	}
	data := []byte(":: 上\n内容一\n\n:: 下\n内容二")
	preview, paths, err := ImportNovelToWorkspace(dir, "确认.txt", data, NovelImportOptions{
		SplitRegex:  `^::\s*(.+)$`,
		SampleChars: 1000000,
	})
	if err != nil {
		t.Fatalf("ImportNovelToWorkspace failed: %v", err)
	}
	if preview.ChapterCount != 2 || len(paths) != preview.ChapterCount {
		t.Fatalf("unexpected import result preview=%#v paths=%#v", preview, paths)
	}
	if preview.SampleChars != NovelImportMaxSampleChars {
		t.Fatalf("sample chars should clamp to max: %d", preview.SampleChars)
	}
}

func TestPreviewNovelImportSampleCharsDefaultAndClamp(t *testing.T) {
	data := []byte("第一章 开始\n内容一\n\n第二章 继续\n内容二")
	tests := []struct {
		name string
		in   int
		want int
	}{
		{name: "default", in: 0, want: NovelImportDefaultSampleChars},
		{name: "min", in: 1, want: NovelImportMinSampleChars},
		{name: "max", in: NovelImportMaxSampleChars + 1, want: NovelImportMaxSampleChars},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			preview, err := PreviewNovelImport("样本.txt", data, NovelImportOptions{SampleChars: tt.in})
			if err != nil {
				t.Fatalf("PreviewNovelImport failed: %v", err)
			}
			if preview.SampleChars != tt.want {
				t.Fatalf("sample chars = %d, want %d", preview.SampleChars, tt.want)
			}
		})
	}
}

func TestPreviewNovelImportRejectsUnsupportedFiles(t *testing.T) {
	if _, err := PreviewNovelImport("novel.pdf", []byte("正文")); err == nil {
		t.Fatalf("expected unsupported file error")
	}
	if _, err := PreviewNovelImport("novel.txt", []byte{0xff, 0xfe}); err == nil {
		t.Fatalf("expected utf-8 error")
	}
}
