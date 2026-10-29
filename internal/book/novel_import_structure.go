package book

import (
	"path/filepath"
	"sort"
	"strings"
)

// Explicit novel labels win over Markdown heading depth. Generic Markdown only
// implies three levels when three distinct heading levels are actually present.
func classifyMarkdownStructure(lines []string, markers []novelImportMarker) {
	levels := map[int]bool{}
	for _, marker := range markers {
		if cnChapterRe.MatchString(marker.info.title) || cnVolumeRe.MatchString(marker.info.title) || enChapterRe.MatchString(marker.info.title) || enVolumeRe.MatchString(marker.info.title) || enSectionRe.MatchString(marker.info.title) {
			return
		}
		line := strings.TrimSpace(lines[marker.line])
		level := len(line) - len(strings.TrimLeft(line, "#"))
		if level > 0 {
			levels[level] = true
		}
	}
	if len(levels) != 3 {
		return
	}
	ordered := []int{}
	for level := range levels {
		ordered = append(ordered, level)
	}
	sort.Ints(ordered)
	for i := range markers {
		line := strings.TrimSpace(lines[markers[i].line])
		level := len(line) - len(strings.TrimLeft(line, "#"))
		switch level {
		case ordered[0]:
			markers[i].info.kind = novelImportTitleVolume
		case ordered[1]:
			markers[i].info.kind = novelImportTitleChapter
		case ordered[2]:
			markers[i].info.kind = novelImportTitleSection
		}
	}
}

func threeLevelChaptersFromMarkers(lines []string, markers []novelImportMarker) []parsedNovelChapter {
	chapters := []parsedNovelChapter{}
	volume, parent := "", ""
	volumeOrder, parentOrder := 0, 0
	if preface := strings.TrimSpace(strings.Join(lines[:markers[0].line], "\n")); preface != "" {
		chapters = append(chapters, parsedNovelChapter{NovelImportChapter: NovelImportChapter{Title: "序章"}, Content: preface + "\n"})
	}
	for i, marker := range markers {
		end := len(lines)
		if i+1 < len(markers) {
			end = markers[i+1].line
		}
		body := strings.TrimSpace(strings.Join(lines[marker.line+1:end], "\n"))
		content := strings.TrimSpace(strings.Join(lines[marker.line:end], "\n")) + "\n"
		entry := parsedNovelChapter{NovelImportChapter: NovelImportChapter{Title: marker.info.title}, Content: content}
		switch marker.info.kind {
		case novelImportTitleVolume:
			volume, parent = marker.info.title, ""
			volumeOrder++
			parentOrder++
			entry.volumeOnly = body == ""
		case novelImportTitleChapter:
			parent = marker.info.title
			parentOrder++
			entry.emptyContainer = body == ""
		case novelImportTitleSection:
			if parent == "" {
				parent = marker.info.title
				parentOrder++
			}
		}
		entry.Volume, entry.volumeOrder = volume, volumeOrder
		entry.parent, entry.parentOrder = parent, parentOrder
		chapters = append(chapters, entry)
	}
	return chapters
}

func reshapeNovelChapters(chapters []parsedNovelChapter, target ChapterStructure, language string) ([]parsedNovelChapter, ChapterStructure, []string) {
	detected := StructureChapters
	for _, chapter := range chapters {
		if chapter.parent != "" {
			detected = StructureSections
			break
		}
		if chapter.Volume != "" {
			detected = StructureVolumes
		}
	}
	if target == "" {
		target = detected
	}
	if target == detected && detected != StructureSections {
		volumePaths := map[int]string{}
		for i := range chapters {
			parent := "chapters"
			if chapters[i].Volume != "" {
				order := chapters[i].volumeOrder
				if volumePaths[order] == "" {
					volumePaths[order] = prefixedVolumePath(len(volumePaths)+1, chapters[i].Volume)
				}
				parent = volumePaths[order]
				chapters[i].VolumePath = parent
			}
			chapters[i].Path = parent + "/" + chapterFilename(i+1, chapters[i].Title, language)
		}
		return chapters, target, nil
	}
	titles := StructureTitles{Volume: "Volume 1", Section: "Section 1"}
	if language == NovelImportLanguageChinese {
		titles = StructureTitles{Volume: "第一卷", Section: "第一节"}
	}
	volumes := []manuscriptVolume{}
	lastVolume, lastParent, lastOrder := "", "", -1
	lastVolumeOrder := -1
	for _, source := range chapters {
		if len(volumes) == 0 || source.Volume != lastVolume || source.volumeOrder != lastVolumeOrder {
			volumes = append(volumes, manuscriptVolume{title: source.Volume})
			lastVolume, lastVolumeOrder, lastParent, lastOrder = source.Volume, source.volumeOrder, "", -1
		}
		volume := &volumes[len(volumes)-1]
		if source.volumeOnly {
			continue
		}
		if source.parent == "" || source.parent != lastParent || source.parentOrder != lastOrder || len(volume.chapters) == 0 {
			title := source.parent
			if title == "" {
				title = source.Title
			}
			volume.chapters = append(volume.chapters, manuscriptChapter{title: title, container: source.parent != ""})
			lastParent, lastOrder = source.parent, source.parentOrder
		}
		chapter := &volume.chapters[len(volume.chapters)-1]
		if !source.emptyContainer {
			chapter.parts = append(chapter.parts, manuscriptPart{title: source.Title, content: source.Content})
		}
	}
	plan := buildStructure(volumes, target, titles)
	result, containers := []parsedNovelChapter{}, []string{}
	var visit func([]ManuscriptNode, string, string)
	visit = func(nodes []ManuscriptNode, volume, volumePath string) {
		for _, node := range nodes {
			if node.Directory {
				containers = append(containers, node.Path)
				if filepath.Dir(node.Path) == "chapters" {
					volume, volumePath = node.Title, node.Path
				}
				visit(node.Children, volume, volumePath)
			} else {
				result = append(result, parsedNovelChapter{NovelImportChapter: NovelImportChapter{Title: node.Title, Path: node.Path, Volume: volume, VolumePath: volumePath}, Content: string(plan.files[node.Path])})
			}
		}
	}
	visit(plan.After, "", "")
	return result, target, containers
}
