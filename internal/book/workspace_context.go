package book

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"denova/internal/book/lore"
)

const (
	workspaceContextChapterGroupLimit = 2
	workspaceContextChapterLimit      = 12
)

// WorkspaceContext is the complete writing-workspace projection consumed by
// Agent context assembly. File-backed writing state appears only as
// workspace-relative locators. Stable may additionally contain enabled
// resident Lore bodies, while all other Lore bodies stay behind Lore tools.
type WorkspaceContext struct {
	Stable  string
	Dynamic string
}

// Markdown renders the projection for status and diagnostics without changing
// the stable/dynamic placement used by Agent context assembly.
func (c WorkspaceContext) Markdown() string {
	return strings.TrimSpace(strings.Join(nonEmptyContextValues(c.Stable, c.Dynamic), "\n\n"))
}

// WorkspaceContext returns a compact source index. Content changes do not
// rotate the index, which keeps the model prefix cacheable while every file
// read observes the authoritative workspace state at tool-execution time.
func (s *State) WorkspaceContext() WorkspaceContext {
	if s == nil {
		return WorkspaceContext{}
	}

	stableFiles := make([]workspaceContextPath, 0, 2)
	if s.hasMeaningfulIdeas() {
		stableFiles = append(stableFiles, workspaceContextPath{Purpose: "Creative ideas and current direction", Path: IdeasFileName})
	}
	if s.hasNonEmptyFile(filepath.Join("setting", "outline.md")) {
		stableFiles = append(stableFiles, workspaceContextPath{Purpose: "Long-term outline", Path: "setting/outline.md"})
	}

	dynamicFiles := make([]workspaceContextPath, 0, 2)
	if s.hasNonEmptyFile(filepath.Join("setting", "progress.md")) {
		dynamicFiles = append(dynamicFiles, workspaceContextPath{Purpose: "Established writing progress", Path: "setting/progress.md"})
	}
	if s.hasNonEmptyFile(filepath.Join("setting", CharacterStatesFileName)) {
		dynamicFiles = append(dynamicFiles, workspaceContextPath{Purpose: "Current character continuity state", Path: "setting/" + CharacterStatesFileName})
	}

	stable := renderWorkspacePathIndex("Workspace Source Index", []workspaceContextPathSection{
		{Title: "Canonical writing files", Paths: stableFiles},
	})
	if loreContext := s.progressiveLoreContext(); loreContext != "" {
		stable = strings.TrimSpace(strings.Join(nonEmptyContextValues(stable, loreContext), "\n\n"))
	}

	dynamic := renderWorkspacePathIndex("Current Writing Source Index", []workspaceContextPathSection{
		{Title: "Current writing state", Paths: dynamicFiles},
		{Title: "Recent chapter-group plans", Paths: contextPaths("Chapter-group plan", s.recentChapterGroupPaths(workspaceContextChapterGroupLimit))},
		{Title: "Recent chapter paths", Paths: contextPaths("Chapter path", s.recentChapterPaths(workspaceContextChapterLimit))},
	})

	return WorkspaceContext{Stable: stable, Dynamic: dynamic}
}

type workspaceContextPath struct {
	Purpose string
	Path    string
}

type workspaceContextPathSection struct {
	Title string
	Paths []workspaceContextPath
}

func renderWorkspacePathIndex(title string, sections []workspaceContextPathSection) string {
	entryCount := 0
	for _, section := range sections {
		entryCount += len(section.Paths)
	}
	if entryCount == 0 {
		return ""
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "## %s\n\n", title)
	sb.WriteString("Source: current book workspace snapshot. Entries are workspace-relative discovery paths only; file contents are intentionally omitted. Use `read` for known files, `glob` to discover additional paths, and `grep` for targeted searches. Read only the sources required by the current request and writing workflow. A path's presence does not prove that its file is non-empty or finalized.\n")
	for _, section := range sections {
		if len(section.Paths) == 0 {
			continue
		}
		fmt.Fprintf(&sb, "\n### %s\n\n", section.Title)
		for _, source := range section.Paths {
			fmt.Fprintf(&sb, "- %s: %s\n", source.Purpose, strconv.Quote(filepath.ToSlash(source.Path)))
		}
	}
	return strings.TrimSpace(sb.String())
}

func contextPaths(purpose string, paths []string) []workspaceContextPath {
	result := make([]workspaceContextPath, 0, len(paths))
	for _, path := range paths {
		result = append(result, workspaceContextPath{Purpose: purpose, Path: path})
	}
	return result
}

func (s *State) hasMeaningfulIdeas() bool {
	info, err := os.Stat(s.IdeasPath())
	if err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
		return false
	}
	template := []byte(IdeasTemplate)
	if info.Size() != int64(len(template)) {
		return true
	}
	data, err := os.ReadFile(s.IdeasPath())
	if err != nil {
		return false
	}
	content := bytes.TrimSpace(data)
	return len(content) > 0 && !bytes.Equal(content, bytes.TrimSpace(template))
}

func (s *State) hasNonEmptyFile(relativePath string) bool {
	info, err := os.Stat(filepath.Join(s.workspace, filepath.FromSlash(relativePath)))
	return err == nil && info.Mode().IsRegular() && info.Size() > 0
}

func (s *State) recentChapterGroupPaths(limit int) []string {
	entries, err := os.ReadDir(s.ChapterGroupDir())
	if err != nil {
		return nil
	}
	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || strings.HasPrefix(name, ".") {
			continue
		}
		ext := strings.ToLower(filepath.Ext(name))
		if ext != ".md" && ext != ".txt" {
			continue
		}
		paths = append(paths, filepath.ToSlash(filepath.Join("setting", "chapter-groups", name)))
	}
	sort.Strings(paths)
	return latestContextPaths(paths, limit)
}

func (s *State) recentChapterPaths(limit int) []string {
	entries := s.chapterPaths()
	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		paths = append(paths, entry.Path)
	}
	return latestContextPaths(paths, limit)
}

func latestContextPaths(paths []string, limit int) []string {
	if limit <= 0 || len(paths) <= limit {
		return paths
	}
	return paths[len(paths)-limit:]
}

func (s *State) progressiveLoreContext() string {
	context, err := lore.NewStore(s.workspace).ProgressiveContextMarkdown()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(context)
}

func nonEmptyContextValues(values ...string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			result = append(result, value)
		}
	}
	return result
}
