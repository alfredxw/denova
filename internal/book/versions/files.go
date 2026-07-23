package versions

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"denova/internal/workspacepath"
)

// WorkspaceFileSet defines which workspace files are visible to versioning.
type WorkspaceFileSet struct {
	root string
}

func (s *Service) collectVisibleFiles() ([]versionFileData, error) {
	return WorkspaceFileSet{root: s.workspace}.Collect()
}

func (w WorkspaceFileSet) Collect() ([]versionFileData, error) {
	return collectVersionFiles(w.root, w.root)
}

func collectVersionFiles(root, base string) ([]versionFileData, error) {
	files := []versionFileData{}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if path == root {
			return nil
		}
		rel, err := filepath.Rel(base, path)
		if err != nil {
			return nil
		}
		relSlash := filepath.ToSlash(rel)
		if isVersionExcludedRelPath(relSlash) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil || info.Mode()&os.ModeSymlink != 0 {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		state := versionFileStateFromBytes(data)
		files = append(files, versionFileData{
			Path:  filepath.ToSlash(rel),
			Abs:   path,
			Hash:  state.Hash,
			Size:  info.Size(),
			Chars: state.Chars,
			Text:  state.Text,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.SliceStable(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, nil
}

type versionFileState struct {
	Hash  string
	Size  int64
	Chars int
	Text  bool
}

func versionFileStateFromBytes(data []byte) versionFileState {
	hashBytes := sha256.Sum256(data)
	text := isTextBytes(data)
	chars := 0
	if text {
		chars = utf8.RuneCount(data)
	}
	return versionFileState{
		Hash:  hex.EncodeToString(hashBytes[:]),
		Size:  int64(len(data)),
		Chars: chars,
		Text:  text,
	}
}

func isTextBytes(data []byte) bool {
	if len(data) == 0 {
		return true
	}
	if !utf8.Valid(data) {
		return false
	}
	for _, b := range data {
		if b == 0 {
			return false
		}
	}
	return true
}

func isVersionExcludedRelPath(relPath string) bool {
	cleanRel := filepath.ToSlash(filepath.Clean(filepath.FromSlash(relPath)))
	return cleanRel == ".git" || strings.HasPrefix(cleanRel, ".git/") ||
		cleanRel == workspacepath.CurrentRel("runs") || strings.HasPrefix(cleanRel, workspacepath.CurrentRel("runs")+"/") ||
		cleanRel == workspacepath.LegacyRel("runs") || strings.HasPrefix(cleanRel, workspacepath.LegacyRel("runs")+"/") ||
		cleanRel == workspacepath.CurrentRel("changes") || strings.HasPrefix(cleanRel, workspacepath.CurrentRel("changes")+"/") ||
		cleanRel == workspacepath.LegacyRel("changes") || strings.HasPrefix(cleanRel, workspacepath.LegacyRel("changes")+"/") ||
		cleanRel == workspacepath.CurrentRel("reviews") || strings.HasPrefix(cleanRel, workspacepath.CurrentRel("reviews")+"/") ||
		cleanRel == workspacepath.LegacyRel("reviews") || strings.HasPrefix(cleanRel, workspacepath.LegacyRel("reviews")+"/") ||
		cleanRel == workspacepath.CurrentRel("interactive") || strings.HasPrefix(cleanRel, workspacepath.CurrentRel("interactive")+"/") ||
		cleanRel == workspacepath.LegacyRel("interactive") || strings.HasPrefix(cleanRel, workspacepath.LegacyRel("interactive")+"/")
}

func versionProtectedExcludedDirs() []string {
	return []string{
		workspacepath.CurrentRel("runs"),
		workspacepath.LegacyRel("runs"),
		workspacepath.CurrentRel("changes"),
		workspacepath.LegacyRel("changes"),
		workspacepath.CurrentRel("reviews"),
		workspacepath.LegacyRel("reviews"),
		workspacepath.CurrentRel("interactive"),
		workspacepath.LegacyRel("interactive"),
	}
}

func safeVisiblePath(workspace, relPath string) (string, error) {
	if strings.TrimSpace(relPath) == "" {
		return "", errors.New("路径不能为空")
	}
	if filepath.IsAbs(relPath) {
		return "", errors.New("不允许使用绝对路径")
	}

	cleanRel := filepath.Clean(filepath.FromSlash(relPath))
	if cleanRel == "." || strings.HasPrefix(cleanRel, ".."+string(filepath.Separator)) || cleanRel == ".." {
		return "", errors.New("路径不在 workspace 范围内")
	}
	if isVersionExcludedRelPath(filepath.ToSlash(cleanRel)) {
		return "", errors.New("不允许操作版本排除路径")
	}

	for _, part := range strings.Split(cleanRel, string(filepath.Separator)) {
		if part == "" {
			return "", errors.New("路径不能为空")
		}
	}

	cleanWorkspace := filepath.Clean(workspace)
	absPath := filepath.Clean(filepath.Join(cleanWorkspace, cleanRel))
	if absPath != cleanWorkspace && !strings.HasPrefix(absPath, cleanWorkspace+string(filepath.Separator)) {
		return "", errors.New("路径不在 workspace 范围内")
	}
	return absPath, nil
}
