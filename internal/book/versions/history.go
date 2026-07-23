package versions

import (
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

const (
	versionSourceTrailer       = "Denova-Source:"
	legacyVersionSourceTrailer = "Nova-Source:"
)

func (s *Service) loadVersions() ([]VersionEntry, error) {
	repo, err := s.openExistingVersionRepo()
	if err != nil {
		return nil, err
	}
	if repo == nil {
		return []VersionEntry{}, nil
	}
	iter, err := repo.CommitObjects()
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	items := []VersionEntry{}
	err = iter.ForEach(func(commit *object.Commit) error {
		entry, err := versionEntryFromCommit(commit)
		if err != nil {
			return err
		}
		items = append(items, entry)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sortVersionsAsc(items)
	return items, nil
}

func (s *Service) headVersion() (*VersionEntry, error) {
	repo, err := s.openExistingVersionRepo()
	if err != nil {
		return nil, err
	}
	if repo == nil {
		return nil, nil
	}
	ref, err := repo.Head()
	if errors.Is(err, plumbing.ErrReferenceNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	commit, err := repo.CommitObject(ref.Hash())
	if err != nil {
		return nil, err
	}
	entry, err := versionEntryFromCommit(commit)
	if err != nil {
		return nil, err
	}
	return &entry, nil
}

func (s *Service) findVersion(id string) (VersionEntry, error) {
	id = strings.TrimSpace(id)
	repo, err := s.openExistingVersionRepo()
	if err != nil {
		return VersionEntry{}, err
	}
	if repo == nil {
		return VersionEntry{}, ErrVersionNotFound
	}
	commit, err := repo.CommitObject(plumbing.NewHash(id))
	if err != nil {
		if errors.Is(err, plumbing.ErrObjectNotFound) {
			return VersionEntry{}, ErrVersionNotFound
		}
		return VersionEntry{}, err
	}
	return versionEntryFromCommit(commit)
}

func versionEntryFromCommit(commit *object.Commit) (VersionEntry, error) {
	files, err := commitFilesSummary(commit)
	if err != nil {
		return VersionEntry{}, err
	}
	message, source := parseCommitMessage(commit.Message)
	return VersionEntry{
		ID:           commit.Hash.String(),
		Message:      message,
		CreatedAt:    commit.Author.When.Format(time.RFC3339),
		Source:       source,
		FileCount:    files.count,
		TotalBytes:   files.totalBytes,
		ChangedPaths: commitChangedPaths(commit),
	}, nil
}

type commitFileSummary struct {
	count      int
	totalBytes int64
}

func commitFilesSummary(commit *object.Commit) (commitFileSummary, error) {
	iter, err := commit.Files()
	if err != nil {
		return commitFileSummary{}, err
	}
	summary := commitFileSummary{}
	err = iter.ForEach(func(file *object.File) error {
		summary.count++
		summary.totalBytes += file.Size
		return nil
	})
	return summary, err
}

func commitChangedPaths(commit *object.Commit) []string {
	files := map[string]bool{}
	collectAllFiles := func() {
		iter, err := commit.Files()
		if err != nil {
			return
		}
		_ = iter.ForEach(func(file *object.File) error {
			files[file.Name] = true
			return nil
		})
	}

	if commit.NumParents() == 0 {
		collectAllFiles()
	} else if parent, err := commit.Parent(0); err == nil {
		patch, err := parent.Patch(commit)
		if err != nil {
			collectAllFiles()
		} else {
			for _, stat := range patch.Stats() {
				if stat.Name != "" {
					files[stat.Name] = true
				}
			}
		}
	} else {
		collectAllFiles()
	}

	paths := make([]string, 0, len(files))
	for path := range files {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths
}

func formatCommitMessage(message, source string) string {
	message = strings.TrimSpace(message)
	source = normalizeVersionSource(source)
	if message == "" {
		message = defaultVersionMessage(source)
	}
	return message + "\n\n" + versionSourceTrailer + " " + source + "\n"
}

func parseCommitMessage(raw string) (string, string) {
	lines := strings.Split(strings.TrimSpace(raw), "\n")
	source := VersionSourceManual
	for len(lines) > 0 {
		line := strings.TrimSpace(lines[len(lines)-1])
		if line == "" {
			lines = lines[:len(lines)-1]
			continue
		}
		if strings.HasPrefix(line, versionSourceTrailer) {
			source = normalizeVersionSource(strings.TrimSpace(strings.TrimPrefix(line, versionSourceTrailer)))
			lines = lines[:len(lines)-1]
		} else if strings.HasPrefix(line, legacyVersionSourceTrailer) {
			source = normalizeVersionSource(strings.TrimSpace(strings.TrimPrefix(line, legacyVersionSourceTrailer)))
			lines = lines[:len(lines)-1]
		}
		break
	}
	message := strings.TrimSpace(strings.Join(lines, "\n"))
	if message == "" {
		message = defaultVersionMessage(source)
	}
	return message, source
}

func normalizeVersionSource(source string) string {
	switch strings.TrimSpace(source) {
	case VersionSourceTimer, VersionSourceAgent, VersionSourceRollbackBackup:
		return strings.TrimSpace(source)
	default:
		return VersionSourceManual
	}
}

func defaultVersionMessage(source string) string {
	switch source {
	case VersionSourceTimer:
		return "自动版本"
	case VersionSourceAgent:
		return "Agent 自动保存"
	case VersionSourceRollbackBackup:
		return "回滚前自动备份"
	default:
		return "手动保存版本"
	}
}

func latestVersion(items []VersionEntry) *VersionEntry {
	if len(items) == 0 {
		return nil
	}
	items = append([]VersionEntry(nil), items...)
	sortVersionsDesc(items)
	latest := items[0]
	return &latest
}

func sortVersionsDesc(items []VersionEntry) {
	sort.SliceStable(items, func(i, j int) bool { return items[i].CreatedAt > items[j].CreatedAt })
}

func sortVersionsAsc(items []VersionEntry) {
	sort.SliceStable(items, func(i, j int) bool { return items[i].CreatedAt < items[j].CreatedAt })
}
