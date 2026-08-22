package versions

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

func (s *Service) Diff(id, path string, comparison VersionDiffComparison) (VersionDiff, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	version, err := s.findVersion(id)
	if err != nil {
		return VersionDiff{}, err
	}
	diff := VersionDiff{Version: version, Comparison: comparison}
	var originalReader func(string) ([]byte, error)
	var modifiedReader func(string) ([]byte, error)

	switch comparison {
	case VersionDiffComparisonWorkspace:
		snapshot, snapshotErr := s.collectWorkspaceSnapshot(nil)
		if snapshotErr != nil {
			return VersionDiff{}, snapshotErr
		}
		baseline, indexErr := s.commitFileIndex(version.ID)
		if indexErr != nil {
			return VersionDiff{}, indexErr
		}
		diff.Changes = diffFileStates(baseline, snapshot.byPath)
		originalReader = func(path string) ([]byte, error) { return s.readCommitFile(version.ID, path) }
		modifiedReader = func(path string) ([]byte, error) {
			return os.ReadFile(filepath.Join(s.workspace, filepath.FromSlash(path)))
		}
	case VersionDiffComparisonParent:
		selected, indexErr := s.commitFileIndex(version.ID)
		if indexErr != nil {
			return VersionDiff{}, indexErr
		}
		baseID, baseVersion, parentErr := s.parentVersion(version.ID)
		if parentErr != nil {
			return VersionDiff{}, parentErr
		}
		baseline := map[string]versionFileData{}
		if baseID != "" {
			baseline, indexErr = s.commitFileIndex(baseID)
			if indexErr != nil {
				return VersionDiff{}, indexErr
			}
		}
		diff.BaseVersion = baseVersion
		diff.Changes = diffFileStates(baseline, selected)
		originalReader = func(path string) ([]byte, error) {
			if baseID == "" {
				return nil, object.ErrFileNotFound
			}
			return s.readCommitFile(baseID, path)
		}
		modifiedReader = func(path string) ([]byte, error) { return s.readCommitFile(version.ID, path) }
	default:
		return VersionDiff{}, errors.New("unsupported version diff comparison")
	}

	path = strings.TrimSpace(path)
	if path == "" {
		return diff, nil
	}
	if _, err := safeVisiblePath(s.workspace, path); err != nil {
		return VersionDiff{}, err
	}
	diff.Path = filepath.ToSlash(filepath.Clean(filepath.FromSlash(path)))
	original, originalErr := originalReader(diff.Path)
	modified, modifiedErr := modifiedReader(diff.Path)
	if errors.Is(originalErr, object.ErrFileNotFound) {
		diff.MissingInOriginal = true
	} else if originalErr != nil {
		return VersionDiff{}, originalErr
	}
	if errors.Is(modifiedErr, os.ErrNotExist) || errors.Is(modifiedErr, object.ErrFileNotFound) {
		diff.MissingInModified = true
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

func (s *Service) parentVersion(id string) (string, *VersionEntry, error) {
	repo, err := s.openExistingVersionRepo()
	if err != nil {
		return "", nil, err
	}
	if repo == nil {
		return "", nil, ErrVersionNotFound
	}
	commit, err := repo.CommitObject(plumbing.NewHash(strings.TrimSpace(id)))
	if err != nil {
		if errors.Is(err, plumbing.ErrObjectNotFound) {
			return "", nil, ErrVersionNotFound
		}
		return "", nil, err
	}
	if len(commit.ParentHashes) == 0 {
		return "", nil, nil
	}
	parent, err := repo.CommitObject(commit.ParentHashes[0])
	if err != nil {
		return "", nil, err
	}
	entry, err := versionEntryFromCommit(parent)
	if err != nil {
		return "", nil, err
	}
	return parent.Hash.String(), &entry, nil
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
	return diffFileStates(committed, current.byPath), nil
}

func diffFileStates(baseline, target map[string]versionFileData) []VersionChange {
	changes := make([]VersionChange, 0)
	seen := map[string]bool{}
	for path, file := range target {
		seen[path] = true
		oldFile, ok := baseline[path]
		if !ok {
			changes = append(changes, VersionChange{Path: path, Status: "added"})
			continue
		}
		if oldFile.Hash != file.Hash || oldFile.Mode != file.Mode {
			changes = append(changes, VersionChange{Path: path, Status: "modified"})
		}
	}
	for path := range baseline {
		if !seen[path] {
			changes = append(changes, VersionChange{Path: path, Status: "deleted"})
		}
	}
	sort.SliceStable(changes, func(i, j int) bool { return changes[i].Path < changes[j].Path })
	return changes
}
