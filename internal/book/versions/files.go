package versions

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	workspacelayout "denova/internal/workspace"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	gitstorer "github.com/go-git/go-git/v5/plumbing/storer"
)

// WorkspaceFileSet defines which workspace files are visible to versioning.
type WorkspaceFileSet struct {
	root string
}

func (s *Service) collectVisibleFiles() ([]versionFileData, error) {
	snapshot, err := s.collectWorkspaceSnapshot(nil)
	return snapshot.files, err
}

func (w WorkspaceFileSet) Collect() ([]versionFileData, error) {
	snapshot, err := collectVersionFiles(w.root, w.root, nil)
	return snapshot.files, err
}

// collectWorkspaceSnapshot optionally persists each observed file as a Git
// blob while its bytes are already in memory. Create uses this path so the
// committed tree is exactly the collected snapshot without a second read.
func (s *Service) collectWorkspaceSnapshot(store gitstorer.EncodedObjectStorer) (workspaceSnapshot, error) {
	return collectVersionFiles(s.workspace, s.workspace, store)
}

func collectVersionFiles(root, base string, store gitstorer.EncodedObjectStorer) (workspaceSnapshot, error) {
	files := []versionFileData{}
	byPath := map[string]versionFileData{}
	var totalBytes int64
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return fmt.Errorf("walk version path %q: %w", path, walkErr)
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
		if err != nil {
			return fmt.Errorf("inspect version file %q: %w", path, err)
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read version file %q: %w", path, err)
		}
		var hash plumbing.Hash
		if store != nil {
			hash, err = storeGitBlob(store, data)
			if err != nil {
				return err
			}
		} else {
			hash = plumbing.ComputeHash(plumbing.BlobObject, data)
		}
		state := versionFileStateFromBytes(data, hash)
		mode, err := filemode.NewFromOSFileMode(info.Mode())
		if err != nil {
			return fmt.Errorf("map version file mode %q: %w", path, err)
		}
		file := versionFileData{
			Path:       filepath.ToSlash(rel),
			Abs:        path,
			Hash:       state.Hash,
			Size:       info.Size(),
			Chars:      state.Chars,
			Text:       state.Text,
			Mode:       mode,
			ModifiedAt: info.ModTime(),
		}
		files = append(files, file)
		byPath[file.Path] = file
		totalBytes += file.Size
		return nil
	})
	if err != nil {
		return workspaceSnapshot{}, err
	}
	sort.SliceStable(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return workspaceSnapshot{files: files, byPath: byPath, totalBytes: totalBytes}, nil
}

func storeGitBlob(store gitstorer.EncodedObjectStorer, data []byte) (plumbing.Hash, error) {
	object := store.NewEncodedObject()
	object.SetType(plumbing.BlobObject)
	object.SetSize(int64(len(data)))
	writer, err := object.Writer()
	if err != nil {
		return plumbing.ZeroHash, err
	}
	if _, err := writer.Write(data); err != nil {
		_ = writer.Close()
		return plumbing.ZeroHash, err
	}
	if err := writer.Close(); err != nil {
		return plumbing.ZeroHash, err
	}
	return store.SetEncodedObject(object)
}

type versionFileState struct {
	Hash  string
	Size  int64
	Chars int
	Text  bool
}

func versionFileStateFromBytes(data []byte, hash plumbing.Hash) versionFileState {
	text := isTextBytes(data)
	chars := 0
	if text {
		chars = utf8.RuneCount(data)
	}
	return versionFileState{
		Hash:  hash.String(),
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
		cleanRel == workspacelayout.CurrentRel("runs") || strings.HasPrefix(cleanRel, workspacelayout.CurrentRel("runs")+"/") ||
		cleanRel == workspacelayout.LegacyRel("runs") || strings.HasPrefix(cleanRel, workspacelayout.LegacyRel("runs")+"/") ||
		cleanRel == workspacelayout.CurrentRel("changes") || strings.HasPrefix(cleanRel, workspacelayout.CurrentRel("changes")+"/") ||
		cleanRel == workspacelayout.LegacyRel("changes") || strings.HasPrefix(cleanRel, workspacelayout.LegacyRel("changes")+"/") ||
		cleanRel == workspacelayout.CurrentRel("reviews") || strings.HasPrefix(cleanRel, workspacelayout.CurrentRel("reviews")+"/") ||
		cleanRel == workspacelayout.LegacyRel("reviews") || strings.HasPrefix(cleanRel, workspacelayout.LegacyRel("reviews")+"/") ||
		cleanRel == workspacelayout.CurrentRel("interactive") || strings.HasPrefix(cleanRel, workspacelayout.CurrentRel("interactive")+"/") ||
		cleanRel == workspacelayout.LegacyRel("interactive") || strings.HasPrefix(cleanRel, workspacelayout.LegacyRel("interactive")+"/")
}

func versionProtectedExcludedDirs() []string {
	return []string{
		workspacelayout.CurrentRel("runs"),
		workspacelayout.LegacyRel("runs"),
		workspacelayout.CurrentRel("changes"),
		workspacelayout.LegacyRel("changes"),
		workspacelayout.CurrentRel("reviews"),
		workspacelayout.LegacyRel("reviews"),
		workspacelayout.CurrentRel("interactive"),
		workspacelayout.LegacyRel("interactive"),
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
