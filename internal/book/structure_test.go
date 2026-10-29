package book

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestThreeLevelImportPreservesReadingOrderAndChapterIntroductions(t *testing.T) {
	for _, strategy := range []string{NovelImportSplitStrategyBuiltin, NovelImportSplitStrategyCustom} {
		t.Run(strategy, func(t *testing.T) {
			workspace := t.TempDir()
			source := "第一卷 启程\n第一章 清晨\n章引言。\n第一节 出发\n甲正文。\n第二节 夜色\n乙正文。\n第二章 相遇\n第一节 出发\n丙正文。\n第二卷 启程\n第一章 清晨\n第一节 出发\n丁正文。"
			opts := NovelImportOptions{SplitStrategy: strategy}
			if strategy == NovelImportSplitStrategyCustom {
				opts.SplitRegex = `^(第[一二][卷章节].*)$`
			}
			preview, paths, err := ImportNovelToWorkspace(workspace, "旅程.txt", []byte(source), opts)
			if err != nil {
				t.Fatal(err)
			}
			if preview.Structure != StructureSections || len(paths) != 5 {
				t.Fatalf("preview=%+v paths=%v", preview, paths)
			}
			for _, path := range paths {
				if strings.Count(path, "/") != 3 {
					t.Fatalf("non-three-level path: %s", path)
				}
			}
			service := NewService(workspace)
			summary, err := service.Summary()
			if err != nil {
				t.Fatal(err)
			}
			got := []string{}
			for _, chapter := range summary.Chapters {
				got = append(got, chapter.Path)
			}
			if !reflect.DeepEqual(got, paths) {
				t.Fatalf("summary order=%v import=%v", got, paths)
			}
			contextPaths, _ := scanChapterPaths(workspace)
			for i, path := range contextPaths {
				if path.Path != paths[i] {
					t.Fatalf("Agent order differs at %d", i)
				}
			}
			exported, err := service.ExportText(BookMeta{Title: "旅程"})
			if err != nil {
				t.Fatal(err)
			}
			previous := -1
			for _, prose := range []string{"章引言。", "甲正文。", "乙正文。", "丙正文。", "丁正文。"} {
				position := strings.Index(exported.Content, prose)
				if position <= previous || strings.Count(exported.Content, prose) != 1 {
					t.Fatalf("lost or reordered %q in %s", prose, exported.Content)
				}
				previous = position
			}
			if !strings.Contains(exported.Content, "第二章 相遇") {
				t.Fatal("export lost chapter heading")
			}
		})
	}
}

func TestStructureConversionPreservesTextConfirmationAndEmptyContainers(t *testing.T) {
	workspace := t.TempDir()
	service := NewService(workspace)
	if err := InitializeChapterStructure(workspace, StructureChapters); err != nil {
		t.Fatal(err)
	}
	path, original := "chapters/ch00001-第一章-启程.md", "# 第一章 启程\n\n完整正文。\n"
	if err := service.Create(path, "file", original); err != nil {
		t.Fatal(err)
	}
	if err := service.SetChapterConfirmed(path, true); err != nil {
		t.Fatal(err)
	}
	titles := StructureTitles{Volume: "第一卷", Section: "第一节"}
	plan, err := service.PreviewStructure(StructureSections, titles)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.ApplyStructure(plan); err != nil {
		t.Fatal(err)
	}
	summary, err := service.Summary()
	if err != nil {
		t.Fatal(err)
	}
	if len(summary.Chapters) != 1 || !summary.Chapters[0].Confirmed {
		t.Fatalf("summary=%+v", summary)
	}
	content, err := service.ReadFile(summary.Chapters[0].Path)
	if err != nil || content != original {
		t.Fatalf("prose changed: %q %v", content, err)
	}
	if err := service.Create("chapters/v00002-空卷", "dir", ""); err != nil {
		t.Fatal(err)
	}
	if err := service.Create("chapters/v00001-第一卷/ch00002-空章", "dir", ""); err != nil {
		t.Fatal(err)
	}
	for _, target := range []ChapterStructure{StructureVolumes, StructureChapters} {
		plan, err = service.PreviewStructure(target, titles)
		if err != nil {
			t.Fatal(err)
		}
		if err = service.ApplyStructure(plan); err != nil {
			t.Fatal(err)
		}
		exported, err := service.ExportText(BookMeta{})
		if err != nil {
			t.Fatal(err)
		}
		if strings.Count(exported.Content, "完整正文。") != 1 {
			t.Fatalf("prose lost or duplicated: %s", exported.Content)
		}
		summary, err := service.Summary()
		if err != nil {
			t.Fatal(err)
		}
		emptyChapterFound := false
		for _, chapter := range summary.Chapters {
			if chapter.DisplayTitle == "空章" {
				emptyChapterFound = true
				content, err := service.ReadFile(chapter.Path)
				if err != nil || strings.TrimSpace(content) != "" {
					t.Fatalf("empty chapter gained prose: %q %v", content, err)
				}
			}
		}
		if !emptyChapterFound {
			t.Fatal("empty chapter was lost")
		}
	}
	structure, err := ReadChapterStructure(workspace)
	if err != nil || structure != StructureChapters {
		t.Fatalf("structure=%s err=%v", structure, err)
	}
	plan, err = service.PreviewStructure(StructureSections, titles)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "chapters", "notes.pdf"), []byte("owned data"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := service.PreviewStructure(StructureVolumes, titles); err == nil {
		t.Fatal("conversion must not silently drop extra files")
	}
	if _, err := os.Stat(filepath.Join(workspace, "chapters", "notes.pdf")); err != nil {
		t.Fatal(err)
	}
}
