package versions

import (
	"sort"
	"strings"
	"time"

	git "github.com/go-git/go-git/v5"
)

func (s *Service) Create(message, source string, settings VersionAutoSettings) (VersionCommandResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.createLocked(message, source, settings)
}

func (s *Service) createLocked(message, source string, settings VersionAutoSettings) (VersionCommandResult, error) {
	source = normalizeVersionSource(source)
	message = strings.TrimSpace(message)
	if message == "" {
		message = defaultVersionMessage(source)
	}
	repo, err := s.openVersionRepo()
	if err != nil {
		return VersionCommandResult{}, err
	}
	snapshot, err := s.collectWorkspaceSnapshot(repo.Storer)
	if err != nil {
		return VersionCommandResult{}, err
	}
	current, err := s.headVersion()
	if err != nil {
		return VersionCommandResult{}, err
	}
	changes := make([]VersionChange, 0, len(snapshot.files))
	if current != nil {
		changes, err = s.diffChangesFromSnapshot(snapshot, current.ID)
		if err != nil {
			return VersionCommandResult{}, err
		}
		if len(changes) == 0 && source != VersionSourceRollbackBackup {
			return VersionCommandResult{}, ErrVersionClean
		}
	} else {
		for _, file := range snapshot.files {
			changes = append(changes, VersionChange{Path: file.Path, Status: "added"})
		}
	}
	now := time.Now()
	lastAutoAt, lastTimedAt, err := s.versionTimesForCommit(source, now)
	if err != nil {
		return VersionCommandResult{}, err
	}
	version, err := s.createSnapshot(repo, snapshot, changes, message, source, lastAutoAt, lastTimedAt, now)
	if err != nil {
		return VersionCommandResult{}, err
	}
	settings = normalizeVersionAutoSettings(settings)
	status := cleanStatusAfterCreate(version, settings, lastAutoAt)
	return VersionCommandResult{Message: "版本已保存", Version: &version, Status: &status}, nil
}

func (s *Service) versionTimesForCommit(source string, now time.Time) (string, string, error) {
	lastAutoAt, lastTimedAt, err := s.latestVersionTimes()
	if err != nil {
		return "", "", err
	}
	nowText := now.Format(time.RFC3339)
	if source == VersionSourceTimer {
		return nowText, nowText, nil
	}
	if source == VersionSourceAgent {
		lastAutoAt = nowText
	}
	return lastAutoAt, lastTimedAt, nil
}

func (s *Service) createSnapshot(repo *git.Repository, snapshot workspaceSnapshot, changes []VersionChange, message, source, lastAutoAt, lastTimedAt string, now time.Time) (VersionEntry, error) {
	metadata := newVersionCommitMetadata(snapshot, changes, lastAutoAt, lastTimedAt)
	hash, err := s.commitWorkspaceSnapshot(repo, snapshot, message, source, metadata, now)
	if err != nil {
		return VersionEntry{}, err
	}
	changed := append([]string(nil), metadata.ChangedPaths...)
	sort.Strings(changed)
	version := VersionEntry{
		ID:           hash.String(),
		Message:      message,
		CreatedAt:    now.Format(time.RFC3339),
		Source:       source,
		FileCount:    len(snapshot.files),
		TotalBytes:   snapshot.totalBytes,
		ChangedPaths: changed,
	}
	return version, nil
}

func cleanStatusAfterCreate(version VersionEntry, settings VersionAutoSettings, lastAutoAt string) VersionStatus {
	return VersionStatus{
		HasVersions: true,
		Clean:       true,
		Changes:     []VersionChange{},
		Latest:      &version,
		Auto: VersionAutoInfo{
			TimedEnabled:         settings.TimedEnabled,
			TimedIntervalMinutes: settings.TimedIntervalMinutes,
			Retention:            settings.Retention,
			LastAutoAt:           lastAutoAt,
		},
	}
}
