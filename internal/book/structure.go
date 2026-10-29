package book

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ChapterStructure is a book-wide manuscript preset. Only the deepest level
// contains prose; ordering remains encoded in portable directory/file names.
type ChapterStructure string

const (
	StructureChapters ChapterStructure = "chapters"
	StructureVolumes  ChapterStructure = "volumes"
	StructureSections ChapterStructure = "sections"
	structureFile                      = "chapters/.structure.json"
)

var (
	ErrInvalidStructure     = errors.New("invalid chapter structure")
	ErrStructureConflict    = errors.New("manuscript changed since preview")
	ErrUnsupportedStructure = errors.New("manuscript contains unsupported files or nesting")
)

func (structure ChapterStructure) Valid() bool {
	return structure == StructureChapters || structure == StructureVolumes || structure == StructureSections
}

// ReadChapterStructure leaves released books untouched and infers their layout
// until the author explicitly chooses a preset.
func ReadChapterStructure(workspace string) (ChapterStructure, error) {
	data, err := os.ReadFile(filepath.Join(workspace, structureFile))
	if err == nil {
		var config struct {
			Structure ChapterStructure `json:"structure"`
		}
		if err := json.Unmarshal(data, &config); err != nil {
			return "", err
		}
		if !config.Structure.Valid() {
			return "", ErrInvalidStructure
		}
		return config.Structure, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	structure := StructureChapters
	root := filepath.Join(workspace, "chapters")
	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if errors.Is(walkErr, os.ErrNotExist) && path == root {
			return nil
		}
		if walkErr != nil {
			return walkErr
		}
		if path == root {
			return nil
		}
		if strings.HasPrefix(entry.Name(), ".") {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			rel, _ := filepath.Rel(root, path)
			if strings.Contains(filepath.ToSlash(rel), "/") {
				structure = StructureSections
			} else if structure == StructureChapters {
				structure = StructureVolumes
			}
		}
		return nil
	})
	return structure, err
}

// InitializeChapterStructure is only for empty/new workspaces. Existing books
// must use a previewed conversion with a version backup.
func InitializeChapterStructure(workspace string, structure ChapterStructure) error {
	if !structure.Valid() {
		return ErrInvalidStructure
	}
	if err := os.MkdirAll(filepath.Join(workspace, "chapters"), 0o755); err != nil {
		return err
	}
	data, _ := json.Marshal(struct {
		Structure ChapterStructure `json:"structure"`
	}{structure})
	return os.WriteFile(filepath.Join(workspace, structureFile), append(data, '\n'), 0o644)
}

// CompareManuscriptPaths compares ancestors before their children, preserving
// reading order when each volume or chapter restarts its local numbering.
func CompareManuscriptPaths(left, right string) int {
	l, r := strings.Split(filepath.ToSlash(left), "/"), strings.Split(filepath.ToSlash(right), "/")
	for i := 0; i < len(l) && i < len(r); i++ {
		if l[i] == r[i] {
			continue
		}
		if cmp := compareChapterLikeNames(l[i], r[i]); cmp != 0 {
			return cmp
		}
		return strings.Compare(l[i], r[i])
	}
	return len(l) - len(r)
}

// ManuscriptNode is a derived tree, including empty containers. It is never a
// second persisted ordering source.
type ManuscriptNode struct {
	Index     int              `json:"index,omitempty"`
	Path      string           `json:"path"`
	Title     string           `json:"title"`
	Children  []ManuscriptNode `json:"children,omitempty"`
	Directory bool             `json:"directory"`
}

func (s *Service) ManuscriptTree() ([]ManuscriptNode, error) {
	return readManuscriptTree(s.workspace, "chapters")
}

func readManuscriptTree(workspace, parent string) ([]ManuscriptNode, error) {
	entries, err := os.ReadDir(filepath.Join(workspace, filepath.FromSlash(parent)))
	if errors.Is(err, os.ErrNotExist) {
		return []ManuscriptNode{}, nil
	}
	if err != nil {
		return nil, err
	}
	nodes := make([]ManuscriptNode, 0, len(entries))
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("%w: %s", ErrUnsupportedStructure, entry.Name())
		}
		if !entry.IsDir() && !isChapterTextFile(entry.Name()) {
			continue
		}
		node := ManuscriptNode{Index: chapterIndex(entry.Name()), Path: parent + "/" + entry.Name(), Title: chapterDisplayTitle(entry.Name()), Directory: entry.IsDir()}
		if node.Directory {
			node.Title = manuscriptDisplayTitle(entry.Name())
			node.Children, err = readManuscriptTree(workspace, node.Path)
			if err != nil {
				return nil, err
			}
		}
		nodes = append(nodes, node)
	}
	sort.Slice(nodes, func(i, j int) bool { return CompareManuscriptPaths(nodes[i].Path, nodes[j].Path) < 0 })
	return nodes, nil
}
