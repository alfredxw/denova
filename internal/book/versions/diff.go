package versions

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/go-git/go-git/v5/plumbing/object"
)

func (s *Service) Diff(id, path string) (VersionDiff, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	version, err := s.findVersion(id)
	if err != nil {
		return VersionDiff{}, err
	}
	snapshot, err := s.collectWorkspaceSnapshot(nil)
	if err != nil {
		return VersionDiff{}, err
	}
	changes, err := s.diffChangesFromSnapshot(snapshot, version.ID)
	if err != nil {
		return VersionDiff{}, err
	}
	diff := VersionDiff{Version: version, Changes: changes}
	path = strings.TrimSpace(path)
	if path == "" {
		return diff, nil
	}
	if _, err := safeVisiblePath(s.workspace, path); err != nil {
		return VersionDiff{}, err
	}
	diff.Path = filepath.ToSlash(filepath.Clean(filepath.FromSlash(path)))
	workspacePath := filepath.Join(s.workspace, filepath.FromSlash(diff.Path))
	original, originalErr := s.readCommitFile(version.ID, diff.Path)
	modified, modifiedErr := os.ReadFile(workspacePath)
	if errors.Is(originalErr, object.ErrFileNotFound) {
		diff.MissingInVersion = true
	} else if originalErr != nil {
		return VersionDiff{}, originalErr
	}
	if errors.Is(modifiedErr, os.ErrNotExist) {
		diff.MissingInWorkspace = true
	} else if modifiedErr != nil {
		return VersionDiff{}, modifiedErr
	}
	if isTextBytes(original) && isTextBytes(modified) {
		diff.Text = true
		diff.Original = string(original)
		diff.Modified = string(modified)
	} else {
		diff.Binary = true
	}
	return diff, nil
}

// ReadFileAtVersion reads one file directly from a commit without computing a
// workspace-wide diff. Callers that already have Status changes can use this
// path for previews without repeating a full workspace scan per file.
func (s *Service) ReadFileAtVersion(id, path string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := safeVisiblePath(s.workspace, path); err != nil {
		return nil, err
	}
	return s.readCommitFile(id, filepath.ToSlash(filepath.Clean(filepath.FromSlash(path))))
}

func (s *Service) diffChangesFromSnapshot(current workspaceSnapshot, versionID string) ([]VersionChange, error) {
	committed, err := s.commitFileIndex(versionID)
	if err != nil {
		return nil, err
	}
	changes := make([]VersionChange, 0)
	seen := map[string]bool{}
	for path, file := range current.byPath {
		seen[path] = true
		oldFile, ok := committed[path]
		if !ok {
			changes = append(changes, VersionChange{Path: path, Status: "added"})
			continue
		}
		if oldFile.Hash != file.Hash || oldFile.Mode != file.Mode {
			changes = append(changes, VersionChange{Path: path, Status: "modified"})
		}
	}
	for path := range committed {
		if !seen[path] {
			changes = append(changes, VersionChange{Path: path, Status: "deleted"})
		}
	}
	sort.SliceStable(changes, func(i, j int) bool { return changes[i].Path < changes[j].Path })
	return changes, nil
}
