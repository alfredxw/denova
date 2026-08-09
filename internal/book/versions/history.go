package versions

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

const (
	versionSourceTrailer        = "Denova-Source:"
	legacyVersionSourceTrailer  = "Nova-Source:"
	versionMetadataTrailer      = "Denova-Metadata:"
	versionCommitMetadataSchema = 1
)

var (
	versionHistoryReference = plumbing.ReferenceName("refs/denova/history")
	versionCurrentReference = plumbing.ReferenceName("refs/denova/current")
)

// versionCommitMetadata keeps history list reads independent from tree size.
// It is stored inside the commit so Project relinks and repository copies keep
// the version projection self-contained.
type versionCommitMetadata struct {
	Schema       int      `json:"schema"`
	FileCount    int      `json:"file_count"`
	TotalBytes   int64    `json:"total_bytes"`
	ChangedPaths []string `json:"changed_paths"`
	LastAutoAt   string   `json:"last_auto_at,omitempty"`
	LastTimedAt  string   `json:"last_timed_at,omitempty"`
}

func newVersionCommitMetadata(snapshot workspaceSnapshot, changes []VersionChange, lastAutoAt, lastTimedAt string) versionCommitMetadata {
	paths := make([]string, 0, len(changes))
	for _, change := range changes {
		paths = append(paths, change.Path)
	}
	sort.Strings(paths)
	return versionCommitMetadata{
		Schema:       versionCommitMetadataSchema,
		FileCount:    len(snapshot.files),
		TotalBytes:   snapshot.totalBytes,
		ChangedPaths: paths,
		LastAutoAt:   strings.TrimSpace(lastAutoAt),
		LastTimedAt:  strings.TrimSpace(lastTimedAt),
	}
}

func (s *Service) loadVersionHistory(limit int) ([]VersionEntry, error) {
	repo, err := s.openExistingVersionRepo()
	if err != nil {
		return nil, err
	}
	if repo == nil {
		return []VersionEntry{}, nil
	}
	hash, err := versionHistoryHeadHash(repo)
	if errors.Is(err, plumbing.ErrReferenceNotFound) {
		return []VersionEntry{}, nil
	}
	if err != nil {
		return nil, err
	}
	commit, err := repo.CommitObject(hash)
	if err != nil {
		return nil, err
	}
	items := make([]VersionEntry, 0, limit)
	for commit != nil && (limit <= 0 || len(items) < limit) {
		entry, err := versionEntryFromCommit(commit)
		if err != nil {
			return nil, err
		}
		items = append(items, entry)
		if len(commit.ParentHashes) == 0 || (limit > 0 && len(items) >= limit) {
			break
		}
		commit, err = repo.CommitObject(commit.ParentHashes[0])
		if err != nil {
			return nil, err
		}
	}
	return items, nil
}

// versionHistoryHeadHash is separate from workspace HEAD: restoring an older
// snapshot moves HEAD to that baseline, while this ref keeps every created
// version in one chronological first-parent chain.
func versionHistoryHeadHash(repo *git.Repository) (plumbing.Hash, error) {
	ref, err := repo.Reference(versionHistoryReference, true)
	if err == nil {
		return ref.Hash(), nil
	}
	if !errors.Is(err, plumbing.ErrReferenceNotFound) {
		return plumbing.ZeroHash, err
	}
	head, err := repo.Head()
	if err != nil {
		return plumbing.ZeroHash, err
	}
	return head.Hash(), nil
}

func (s *Service) headVersion() (*VersionEntry, error) {
	repo, err := s.openExistingVersionRepo()
	if err != nil {
		return nil, err
	}
	if repo == nil {
		return nil, nil
	}
	hash, err := versionCurrentHeadHash(repo)
	if errors.Is(err, plumbing.ErrReferenceNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	commit, err := repo.CommitObject(hash)
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
	message, source, metadata, hasMetadata := parseVersionCommitMessage(commit.Message)
	if hasMetadata {
		return VersionEntry{
			ID:           commit.Hash.String(),
			Message:      message,
			CreatedAt:    commit.Author.When.Format(time.RFC3339),
			Source:       source,
			FileCount:    metadata.FileCount,
			TotalBytes:   metadata.TotalBytes,
			ChangedPaths: append([]string(nil), metadata.ChangedPaths...),
		}, nil
	}

	// Released commits without embedded metadata remain readable, but this
	// tree-level fallback runs only for entries inside the requested page.
	files, err := commitFilesSummary(commit)
	if err != nil {
		return VersionEntry{}, err
	}
	changedPaths, err := commitChangedPaths(commit)
	if err != nil {
		return VersionEntry{}, err
	}
	return VersionEntry{
		ID:           commit.Hash.String(),
		Message:      message,
		CreatedAt:    commit.Author.When.Format(time.RFC3339),
		Source:       source,
		FileCount:    files.count,
		TotalBytes:   files.totalBytes,
		ChangedPaths: changedPaths,
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

func commitChangedPaths(commit *object.Commit) ([]string, error) {
	files := map[string]bool{}
	collectAllFiles := func() error {
		iter, err := commit.Files()
		if err != nil {
			return err
		}
		return iter.ForEach(func(file *object.File) error {
			files[file.Name] = true
			return nil
		})
	}

	if commit.NumParents() == 0 {
		if err := collectAllFiles(); err != nil {
			return nil, err
		}
	} else {
		parent, err := commit.Parent(0)
		if err != nil {
			return nil, err
		}
		parentTree, err := parent.Tree()
		if err != nil {
			return nil, err
		}
		currentTree, err := commit.Tree()
		if err != nil {
			return nil, err
		}
		changes, err := object.DiffTree(parentTree, currentTree)
		if err != nil {
			return nil, err
		}
		for _, change := range changes {
			name := change.To.Name
			if name == "" {
				name = change.From.Name
			}
			if name != "" {
				files[name] = true
			}
		}
	}

	paths := make([]string, 0, len(files))
	for path := range files {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths, nil
}

func formatCommitMessage(message, source string, metadata versionCommitMetadata) (string, error) {
	message = strings.TrimSpace(message)
	source = normalizeVersionSource(source)
	if message == "" {
		message = defaultVersionMessage(source)
	}
	encodedMetadata, err := json.Marshal(metadata)
	if err != nil {
		return "", fmt.Errorf("encode version commit metadata: %w", err)
	}
	return message + "\n\n" +
		versionSourceTrailer + " " + source + "\n" +
		versionMetadataTrailer + " " + base64.RawURLEncoding.EncodeToString(encodedMetadata) + "\n", nil
}

func parseVersionCommitMessage(raw string) (string, string, versionCommitMetadata, bool) {
	lines := strings.Split(strings.TrimSpace(raw), "\n")
	source := VersionSourceManual
	metadata := versionCommitMetadata{}
	hasMetadata := false

trailers:
	for len(lines) > 0 {
		line := strings.TrimSpace(lines[len(lines)-1])
		if line == "" {
			lines = lines[:len(lines)-1]
			continue
		}
		switch {
		case strings.HasPrefix(line, versionMetadataTrailer):
			encoded := strings.TrimSpace(strings.TrimPrefix(line, versionMetadataTrailer))
			decoded, err := base64.RawURLEncoding.DecodeString(encoded)
			if err == nil && json.Unmarshal(decoded, &metadata) == nil && validVersionCommitMetadata(metadata) {
				hasMetadata = true
			}
			lines = lines[:len(lines)-1]
		case strings.HasPrefix(line, versionSourceTrailer):
			source = normalizeVersionSource(strings.TrimSpace(strings.TrimPrefix(line, versionSourceTrailer)))
			lines = lines[:len(lines)-1]
		case strings.HasPrefix(line, legacyVersionSourceTrailer):
			source = normalizeVersionSource(strings.TrimSpace(strings.TrimPrefix(line, legacyVersionSourceTrailer)))
			lines = lines[:len(lines)-1]
		default:
			break trailers
		}
	}
	message := strings.TrimSpace(strings.Join(lines, "\n"))
	if message == "" {
		message = defaultVersionMessage(source)
	}
	return message, source, metadata, hasMetadata
}

func validVersionCommitMetadata(metadata versionCommitMetadata) bool {
	if metadata.Schema != versionCommitMetadataSchema || metadata.FileCount < 0 || metadata.TotalBytes < 0 {
		return false
	}
	if metadata.LastAutoAt != "" {
		if _, err := time.Parse(time.RFC3339, metadata.LastAutoAt); err != nil {
			return false
		}
	}
	if metadata.LastTimedAt != "" {
		if _, err := time.Parse(time.RFC3339, metadata.LastTimedAt); err != nil {
			return false
		}
	}
	return true
}

func (s *Service) latestVersionTimes() (string, string, error) {
	repo, err := s.openExistingVersionRepo()
	if err != nil || repo == nil {
		return "", "", err
	}
	hash, err := versionHistoryHeadHash(repo)
	if errors.Is(err, plumbing.ErrReferenceNotFound) {
		return "", "", nil
	}
	if err != nil {
		return "", "", err
	}
	commit, err := repo.CommitObject(hash)
	if err != nil {
		return "", "", err
	}
	lastAutoAt := ""
	lastTimedAt := ""
	for commit != nil {
		_, source, metadata, hasMetadata := parseVersionCommitMessage(commit.Message)
		if hasMetadata {
			return metadata.LastAutoAt, metadata.LastTimedAt, nil
		}
		createdAt := commit.Author.When.Format(time.RFC3339)
		if source == VersionSourceTimer && lastTimedAt == "" {
			lastTimedAt = createdAt
		}
		if (source == VersionSourceTimer || source == VersionSourceAgent) && lastAutoAt == "" {
			lastAutoAt = createdAt
		}
		if lastAutoAt != "" && lastTimedAt != "" {
			return lastAutoAt, lastTimedAt, nil
		}
		if len(commit.ParentHashes) == 0 {
			return lastAutoAt, lastTimedAt, nil
		}
		commit, err = repo.CommitObject(commit.ParentHashes[0])
		if err != nil {
			return "", "", err
		}
	}
	return lastAutoAt, lastTimedAt, nil
}

// versionCurrentHeadHash identifies the snapshot the workspace is currently
// based on. It intentionally differs from chronological history after a user
// restores an older version.
func versionCurrentHeadHash(repo *git.Repository) (plumbing.Hash, error) {
	ref, err := repo.Reference(versionCurrentReference, true)
	if err == nil {
		return ref.Hash(), nil
	}
	if !errors.Is(err, plumbing.ErrReferenceNotFound) {
		return plumbing.ZeroHash, err
	}
	head, err := repo.Head()
	if err != nil {
		return plumbing.ZeroHash, err
	}
	return head.Hash(), nil
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
