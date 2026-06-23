package book

import (
	"os"
	"path/filepath"
	"testing"
)

func TestServiceSummaryCountsChapters(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "chapters"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "chapters", "ch02-第二章.md"), []byte("第二章\n\n三个人出发。"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "chapters", "ch01-开局.md"), []byte("第一章\n\n天亮了。"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "book.json"), []byte(`{"title":"无限狩猎","author":"Nova"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	summary, err := NewService(root).Summary()
	if err != nil {
		t.Fatal(err)
	}

	if summary.Title != "无限狩猎" {
		t.Fatalf("title = %q", summary.Title)
	}
	if summary.ChapterCount != 2 {
		t.Fatalf("chapter count = %d", summary.ChapterCount)
	}
	if summary.Chapters[0].Path != "chapters/ch01-开局.md" {
		t.Fatalf("first chapter = %q", summary.Chapters[0].Path)
	}
	if summary.Chapters[0].DisplayTitle != "01 开局" {
		t.Fatalf("display title = %q", summary.Chapters[0].DisplayTitle)
	}
	if summary.TotalWords == 0 {
		t.Fatal("expected non-zero total words")
	}
}

func TestServiceSummaryUsesManualChapterConfirmation(t *testing.T) {
	root := t.TempDir()
	chapterDir := filepath.Join(root, "chapters")
	if err := os.MkdirAll(chapterDir, 0o755); err != nil {
		t.Fatal(err)
	}
	longContent := make([]byte, 6000)
	for i := range longContent {
		longContent[i] = 'a'
	}
	if err := os.WriteFile(filepath.Join(chapterDir, "ch01-long.md"), longContent, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(chapterDir, "ch02-empty.md"), nil, 0o644); err != nil {
		t.Fatal(err)
	}

	service := NewService(root)
	summary, err := service.Summary()
	if err != nil {
		t.Fatal(err)
	}
	if summary.Chapters[0].Status != "初稿" || summary.Chapters[0].Confirmed {
		t.Fatalf("long chapter should stay unconfirmed draft before manual confirmation: %#v", summary.Chapters[0])
	}
	if summary.Chapters[1].Status != "空章" {
		t.Fatalf("empty chapter status = %q", summary.Chapters[1].Status)
	}

	if err := service.SetChapterConfirmed("chapters/ch01-long.md", true); err != nil {
		t.Fatal(err)
	}
	summary, err = service.Summary()
	if err != nil {
		t.Fatal(err)
	}
	if summary.Chapters[0].Status != "成章" || !summary.Chapters[0].Confirmed {
		t.Fatalf("confirmed chapter status mismatch: %#v", summary.Chapters[0])
	}

	if err := service.SetChapterConfirmed("chapters/ch01-long.md", false); err != nil {
		t.Fatal(err)
	}
	summary, err = service.Summary()
	if err != nil {
		t.Fatal(err)
	}
	if summary.Chapters[0].Status != "初稿" || summary.Chapters[0].Confirmed {
		t.Fatalf("revoked chapter should return to draft: %#v", summary.Chapters[0])
	}
}

func TestSetChapterConfirmedRejectsNonChapterPath(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "ideas.md"), []byte("idea"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := NewService(root).SetChapterConfirmed("ideas.md", true); err == nil {
		t.Fatal("expected non-chapter path to be rejected")
	}
}

func TestChapterDisplayTitleAndIndexSupportMultipleFilenameStyles(t *testing.T) {
	tests := []struct {
		name        string
		wantIndex   int
		wantDisplay string
	}{
		{name: "序章.md", wantIndex: 0, wantDisplay: "序章"},
		{name: "ch0001-开局.md", wantIndex: 1, wantDisplay: "0001 开局"},
		{name: "ch00001-序章.md", wantIndex: 1, wantDisplay: "序章"},
		{name: "ch00002-第一章-缘起.md", wantIndex: 2, wantDisplay: "第一章 缘起"},
		{name: "001-开局.md", wantIndex: 1, wantDisplay: "001 开局"},
		{name: "第一章-缘起.md", wantIndex: 1, wantDisplay: "第一章 缘起"},
		{name: "第12章-归来.md", wantIndex: 12, wantDisplay: "第12章 归来"},
		{name: "Chapter-2-Flight.md", wantIndex: 2, wantDisplay: "Chapter 2 Flight"},
		{name: "Chapter XII Return.md", wantIndex: 12, wantDisplay: "Chapter XII Return"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := chapterIndex(tt.name); got != tt.wantIndex {
				t.Fatalf("chapterIndex(%q) = %d, want %d", tt.name, got, tt.wantIndex)
			}
			if got := chapterDisplayTitle(tt.name); got != tt.wantDisplay {
				t.Fatalf("chapterDisplayTitle(%q) = %q, want %q", tt.name, got, tt.wantDisplay)
			}
		})
	}
}

func TestServiceSummarySortsHiddenChapterPrefixes(t *testing.T) {
	root := t.TempDir()
	chapterDir := filepath.Join(root, "chapters", "v00001-第一卷-风起")
	if err := os.MkdirAll(chapterDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		"ch00011-第十章-交锋.md",
		"ch00002-第一章-开局.md",
		"ch00111-第一百一十一章-归途.md",
		"ch00001-序章.md",
	} {
		if err := os.WriteFile(filepath.Join(chapterDir, name), []byte("正文"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	summary, err := NewService(root).Summary()
	if err != nil {
		t.Fatal(err)
	}

	got := make([]string, 0, len(summary.Chapters))
	displays := make([]string, 0, len(summary.Chapters))
	for _, chapter := range summary.Chapters {
		got = append(got, chapter.FileName)
		displays = append(displays, chapter.DisplayTitle)
		if chapter.Volume != "第一卷 风起" {
			t.Fatalf("volume display should hide prefix, got %q", chapter.Volume)
		}
	}
	want := []string{
		"ch00001-序章.md",
		"ch00002-第一章-开局.md",
		"ch00011-第十章-交锋.md",
		"ch00111-第一百一十一章-归途.md",
	}
	wantDisplays := []string{"序章", "第一章 开局", "第十章 交锋", "第一百一十一章 归途"}
	for i := range want {
		if got[i] != want[i] || displays[i] != wantDisplays[i] {
			t.Fatalf("hidden prefix order/display mismatch: paths=%v displays=%v", got, displays)
		}
	}
}

func TestServiceSummarySortsChineseChapterOrdinals(t *testing.T) {
	root := t.TempDir()
	chapterDir := filepath.Join(root, "chapters")
	if err := os.MkdirAll(chapterDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		"第十一章-潮声.md",
		"第一百一十一章-归途.md",
		"第一章-开局.md",
		"第一千一百一十一章-终局.md",
		"序章.md",
		"第十章-交锋.md",
	} {
		if err := os.WriteFile(filepath.Join(chapterDir, name), []byte("正文"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	summary, err := NewService(root).Summary()
	if err != nil {
		t.Fatal(err)
	}

	got := make([]string, 0, len(summary.Chapters))
	for _, chapter := range summary.Chapters {
		got = append(got, chapter.FileName)
	}
	want := []string{
		"序章.md",
		"第一章-开局.md",
		"第十章-交锋.md",
		"第十一章-潮声.md",
		"第一百一十一章-归途.md",
		"第一千一百一十一章-终局.md",
	}
	if len(got) != len(want) {
		t.Fatalf("章节数量不符合预期: want=%v got=%v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("章节排序不符合预期: want=%v got=%v", want, got)
		}
	}
}
