package book

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type chapterPathEntry struct {
	Path   string
	Index  int
	Volume string
}

// InvalidateChapterPaths discards the rebuildable path projection after an
// authoritative filesystem resync. Ordinary create/delete/rename operations
// are detected through directory mtimes without rescanning chapter files.
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

func chapterDirectoriesUnchanged(directories map[string]int64) bool {
	if len(directories) == 0 {
		return false
	}
	for path, modifiedNS := range directories {
		info, err := os.Stat(path)
		if err != nil || !info.IsDir() || info.ModTime().UnixNano() != modifiedNS {
			return false
		}
	}
	return true
}

func scanChapterPaths(workspace string) ([]chapterPathEntry, map[string]int64) {
	root := filepath.Join(workspace, "chapters")
	entries := make([]chapterPathEntry, 0)
	directories := make(map[string]int64)
	_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		name := entry.Name()
		if path != root && strings.HasPrefix(name, ".") {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			if info, err := entry.Info(); err == nil {
				directories[path] = info.ModTime().UnixNano()
			}
			return nil
		}
		if !isChapterTextFile(name) {
			return nil
		}
		rel, err := filepath.Rel(workspace, path)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		volume, _ := chapterVolume(rel)
		entries = append(entries, chapterPathEntry{Path: rel, Index: chapterIndex(name), Volume: volume})
		return nil
	})
	sort.Slice(entries, func(i, j int) bool {
		left, right := entries[i], entries[j]
		if cmp := compareChapterLikeNames(filepath.Base(left.Path), filepath.Base(right.Path)); cmp != 0 {
			return cmp < 0
		}
		return left.Path < right.Path
	})
	return entries, directories
}
