package book

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type chapterPathEntry struct {
	Path string
}

type chapterDirectoryEntry struct {
	Name      string
	Directory bool
}

// InvalidateChapterPaths discards the rebuildable path projection after an
// authoritative filesystem resync. Ordinary create/delete/rename operations
// are detected through exact directory-entry snapshots without reading chapter
// bodies or depending on filesystem timestamp resolution.
func (s *State) InvalidateChapterPaths(_ []string, resync bool) {
	if s == nil || !resync {
		return
	}
	s.chapterPathMu.Lock()
	s.chapterPathDirty = true
	s.chapterPathMu.Unlock()
}

func (s *State) chapterPaths() []chapterPathEntry {
	if s == nil {
		return nil
	}
	s.chapterPathMu.Lock()
	defer s.chapterPathMu.Unlock()
	if !s.chapterPathDirty && chapterDirectoriesUnchanged(s.chapterPathDirectories) {
		return append([]chapterPathEntry(nil), s.chapterPathEntries...)
	}
	entries, directories := scanChapterPaths(s.workspace)
	s.chapterPathEntries = entries
	s.chapterPathDirectories = directories
	s.chapterPathDirty = false
	return append([]chapterPathEntry(nil), entries...)
}

func chapterDirectoriesUnchanged(directories map[string][]chapterDirectoryEntry) bool {
	if len(directories) == 0 {
		return false
	}
	for path, expected := range directories {
		current, err := readChapterDirectory(path)
		if err != nil || !equalChapterDirectoryEntries(current, expected) {
			return false
		}
	}
	return true
}

func scanChapterPaths(workspace string) ([]chapterPathEntry, map[string][]chapterDirectoryEntry) {
	root := filepath.Join(workspace, "chapters")
	entries := make([]chapterPathEntry, 0)
	directories := make(map[string][]chapterDirectoryEntry)
	scanChapterDirectory(workspace, root, &entries, directories)
	sort.Slice(entries, func(i, j int) bool {
		left, right := entries[i], entries[j]
		if cmp := compareChapterLikeNames(filepath.Base(left.Path), filepath.Base(right.Path)); cmp != 0 {
			return cmp < 0
		}
		return left.Path < right.Path
	})
	return entries, directories
}

func scanChapterDirectory(workspace, path string, paths *[]chapterPathEntry, directories map[string][]chapterDirectoryEntry) {
	entries, err := readChapterDirectory(path)
	if err != nil {
		return
	}
	directories[path] = entries
	for _, entry := range entries {
		child := filepath.Join(path, entry.Name)
		if entry.Directory {
			scanChapterDirectory(workspace, child, paths, directories)
			continue
		}
		rel, err := filepath.Rel(workspace, child)
		if err == nil {
			*paths = append(*paths, chapterPathEntry{Path: filepath.ToSlash(rel)})
		}
	}
}

func readChapterDirectory(path string) ([]chapterDirectoryEntry, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	result := make([]chapterDirectoryEntry, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") || !entry.IsDir() && !isChapterTextFile(name) {
			continue
		}
		result = append(result, chapterDirectoryEntry{Name: name, Directory: entry.IsDir()})
	}
	return result, nil
}

func equalChapterDirectoryEntries(left, right []chapterDirectoryEntry) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
