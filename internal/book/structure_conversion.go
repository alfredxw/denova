package book

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	workspacelayout "denova/internal/workspace"
)

// StructureTitles supplies localized names only for containers absent in the
// original manuscript. Existing author-owned titles are always retained.
type StructureTitles struct{ Volume, Section string }

type StructurePreview struct {
	From           ChapterStructure `json:"from"`
	To             ChapterStructure `json:"to"`
	Revision       string           `json:"revision"`
	Before         []ManuscriptNode `json:"before"`
	After          []ManuscriptNode `json:"after"`
	MergedChapters int              `json:"merged_chapters"`
	FilesBefore    int              `json:"files_before"`
	FilesAfter     int              `json:"files_after"`
	files          map[string][]byte
	confirmed      map[string]bool
}

type manuscriptVolume struct {
	title    string
	chapters []manuscriptChapter
}
type manuscriptChapter struct {
	title     string
	parts     []manuscriptPart
	container bool
}
type manuscriptPart struct {
	title, content string
	confirmed      bool
}

// PreviewStructure reads a complete source snapshot. Its revision covers prose,
// empty directories, the selected preset and confirmation state, so application
// cannot accidentally use a preview of different content.
func (s *Service) PreviewStructure(target ChapterStructure, titles StructureTitles) (StructurePreview, error) {
	if !target.Valid() {
		return StructurePreview{}, ErrInvalidStructure
	}
	from, err := ReadChapterStructure(s.workspace)
	if err != nil {
		return StructurePreview{}, err
	}
	nodes, err := s.ManuscriptTree()
	if err != nil {
		return StructurePreview{}, err
	}
	status, err := s.readChapterStatusStore()
	if err != nil {
		return StructurePreview{}, err
	}
	hash := sha256.New()
	statusData, _ := json.Marshal(status)
	hash.Write(statusData)
	// Hash every entry, including hidden files, and reject material that cannot
	// be represented by a preset rather than dropping author data.
	err = filepath.WalkDir(filepath.Join(s.workspace, "chapters"), func(path string, entry os.DirEntry, walkErr error) error {
		if errors.Is(walkErr, os.ErrNotExist) && path == filepath.Join(s.workspace, "chapters") {
			return nil
		}
		if walkErr != nil {
			return walkErr
		}
		rel, _ := filepath.Rel(s.workspace, path)
		rel = filepath.ToSlash(rel)
		fmt.Fprintf(hash, "%s\x00%t\x00", rel, entry.IsDir())
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("%w: %s", ErrUnsupportedStructure, rel)
		}
		if entry.IsDir() {
			if strings.Count(rel, "/") > 2 || (strings.HasPrefix(entry.Name(), ".") && rel != "chapters") {
				return fmt.Errorf("%w: %s", ErrUnsupportedStructure, rel)
			}
			return nil
		}
		if !entry.Type().IsRegular() || (rel != structureFile && entry.Name() != ".keep" && !isChapterTextFile(entry.Name())) || (strings.HasPrefix(entry.Name(), ".") && rel != structureFile && entry.Name() != ".keep") {
			return fmt.Errorf("%w: %s", ErrUnsupportedStructure, rel)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		fmt.Fprintf(hash, "%d\x00", len(data))
		hash.Write(data)
		return nil
	})
	if err != nil {
		return StructurePreview{}, err
	}
	volumes := []manuscriptVolume{}
	readChapter := func(node ManuscriptNode) (manuscriptChapter, error) {
		chapter := manuscriptChapter{title: node.Title, container: node.Directory}
		parts := []ManuscriptNode{node}
		if node.Directory {
			parts = node.Children
		}
		for _, part := range parts {
			if part.Directory {
				return chapter, fmt.Errorf("%w: %s", ErrUnsupportedStructure, part.Path)
			}
			body, err := s.ReadFile(part.Path)
			if err != nil {
				return chapter, err
			}
			chapter.parts = append(chapter.parts, manuscriptPart{title: part.Title, content: body, confirmed: status.Chapters[part.Path]})
		}
		return chapter, nil
	}
	for _, node := range nodes {
		if node.Directory {
			volume := manuscriptVolume{title: node.Title}
			for _, child := range node.Children {
				chapter, err := readChapter(child)
				if err != nil {
					return StructurePreview{}, err
				}
				volume.chapters = append(volume.chapters, chapter)
			}
			volumes = append(volumes, volume)
		} else {
			chapter, err := readChapter(node)
			if err != nil {
				return StructurePreview{}, err
			}
			if len(volumes) == 0 || volumes[len(volumes)-1].title != "" {
				volumes = append(volumes, manuscriptVolume{})
			}
			volumes[len(volumes)-1].chapters = append(volumes[len(volumes)-1].chapters, chapter)
		}
	}
	plan := buildStructure(volumes, target, titles)
	plan.From, plan.Before, plan.Revision = from, nodes, hex.EncodeToString(hash.Sum(nil))
	return plan, nil
}

func buildStructure(volumes []manuscriptVolume, target ChapterStructure, titles StructureTitles) StructurePreview {
	plan := StructurePreview{To: target, After: []ManuscriptNode{}, files: map[string][]byte{}, confirmed: map[string]bool{}}
	chapterOrder := 0
	for vi, volume := range volumes {
		parent := "chapters"
		volumeTitle := volume.title
		if volumeTitle == "" {
			volumeTitle = titles.Volume
		}
		volumeNode := ManuscriptNode{}
		if target != StructureChapters {
			parent = prefixedVolumePath(vi+1, volumeTitle)
			volumeNode = ManuscriptNode{Path: parent, Title: volumeTitle, Directory: true}
			plan.files[parent+"/.keep"] = []byte{}
		} else if len(volume.chapters) == 0 {
			// Preserve the title of an empty volume as an empty chapter.
			volume.chapters = []manuscriptChapter{{title: volumeTitle}}
		}
		for _, chapter := range volume.chapters {
			chapterOrder++
			chapterPath := parent + "/" + chapterFilename(chapterOrder, chapter.title, "")
			plan.FilesBefore += len(chapter.parts)
			node := ManuscriptNode{Path: chapterPath, Title: chapter.title}
			if target == StructureSections {
				node.Path = strings.TrimSuffix(chapterPath, ".md")
				node.Directory = true
				plan.files[node.Path+"/.keep"] = []byte{}
				for pi, part := range chapter.parts {
					title := part.title
					if !chapter.container {
						title = titles.Section
					}
					path := node.Path + "/" + sectionFilename(pi+1, title)
					node.Children = append(node.Children, ManuscriptNode{Path: path, Title: title})
					plan.files[path] = []byte(part.content)
					if part.confirmed {
						plan.confirmed[path] = true
					}
					plan.FilesAfter++
				}
			} else {
				content := ""
				confirmed := len(chapter.parts) > 0
				if chapter.container {
					plan.MergedChapters++
					if len(chapter.parts) > 0 {
						content = "# " + chapter.title + "\n\n"
					}
					for _, part := range chapter.parts {
						content += withManuscriptHeading(part.title, part.content) + "\n\n"
					}
				} else if len(chapter.parts) > 0 {
					content = chapter.parts[0].content
				}
				for _, part := range chapter.parts {
					confirmed = confirmed && part.confirmed
				}
				plan.files[node.Path] = []byte(content)
				if confirmed {
					plan.confirmed[node.Path] = true
				}
				plan.FilesAfter++
			}
			if target == StructureChapters {
				plan.After = append(plan.After, node)
			} else {
				volumeNode.Children = append(volumeNode.Children, node)
			}
		}
		if target != StructureChapters {
			plan.After = append(plan.After, volumeNode)
		}
	}
	return plan
}

func sectionFilename(order int, title string) string {
	name := safeFilenamePart(title)
	if name == "" {
		name = "section"
	}
	return fmt.Sprintf("s%05d-%s.md", order, name)
}

func withManuscriptHeading(title, content string) string {
	trimmed := strings.TrimSpace(content)
	line, _, _ := strings.Cut(trimmed, "\n")
	if sameExportTitle(line, title) {
		return content
	}
	return "## " + title + "\n\n" + content
}

// PreserveEmptyManuscriptDirectories makes released, filesystem-created empty
// containers visible to file-based version snapshots before a conversion.
// The caller must hold the workspace mutation lease and validate the preview.
func (s *Service) PreserveEmptyManuscriptDirectories(nodes []ManuscriptNode) error {
	for _, node := range nodes {
		if !node.Directory {
			continue
		}
		if len(node.Children) > 0 {
			if err := s.PreserveEmptyManuscriptDirectories(node.Children); err != nil {
				return err
			}
			continue
		}
		marker := filepath.Join(s.workspace, filepath.FromSlash(node.Path), ".keep")
		file, err := os.OpenFile(marker, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if errors.Is(err, os.ErrExist) {
			continue
		}
		if err != nil {
			return err
		}
		if err := file.Close(); err != nil {
			return err
		}
	}
	return nil
}

// ApplyStructure requires the caller's exclusive workspace lease and a durable
// version backup. Stage on the same filesystem and roll back both directory and
// confirmation state if any step fails. Version history covers crash recovery.
func (s *Service) ApplyStructure(plan StructurePreview) (err error) {
	if !plan.To.Valid() || plan.files == nil {
		return ErrInvalidStructure
	}
	stageRoot := workspacelayout.Path(s.workspace, "changes")
	if err = os.MkdirAll(stageRoot, 0o755); err != nil {
		return err
	}
	stage, err := os.MkdirTemp(stageRoot, "structure-")
	if err != nil {
		return err
	}
	cleanupStage := true
	defer func() {
		if cleanupStage {
			_ = os.RemoveAll(stage)
		}
	}()
	if err = InitializeChapterStructure(stage, plan.To); err != nil {
		return err
	}
	for path, data := range plan.files {
		absolute := filepath.Join(stage, filepath.FromSlash(path))
		if err = os.MkdirAll(filepath.Dir(absolute), 0o755); err != nil {
			return err
		}
		if err = os.WriteFile(absolute, data, 0o644); err != nil {
			return err
		}
	}
	oldStatus, err := s.readChapterStatusStore()
	if err != nil {
		return err
	}
	root := filepath.Join(s.workspace, "chapters")
	if err = os.MkdirAll(root, 0o755); err != nil {
		return err
	}
	backup := filepath.Join(stage, "original")
	if err = os.Rename(root, backup); err != nil {
		return err
	}
	defer func() {
		if err != nil {
			removeErr := os.RemoveAll(root)
			restoreErr := os.Rename(backup, root)
			if restoreErr != nil {
				cleanupStage = false
			}
			statusErr := s.writeChapterStatusStore(oldStatus)
			err = errors.Join(err, removeErr, restoreErr, statusErr)
		}
		s.InvalidateSummary(nil, true)
	}()
	if err = os.Rename(filepath.Join(stage, "chapters"), root); err != nil {
		return err
	}
	return s.writeChapterStatusStore(chapterStatusStore{Chapters: plan.confirmed})
}
