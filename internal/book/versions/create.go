package versions

import (
	"sort"
	"strings"
	"time"
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
	index, err := s.loadIndex()
	if err != nil {
		return VersionCommandResult{}, err
	}
	var base *VersionEntry
	if current := currentVersion(index.Items, index.CurrentID); current != nil {
		base = current
		changes, err := s.diffChanges(*current)
		if err != nil {
			return VersionCommandResult{}, err
		}
		if len(changes) == 0 && source != VersionSourceRollbackBackup {
			return VersionCommandResult{}, ErrVersionClean
		}
	}
	version, err := s.createSnapshot(message, source, base)
	if err != nil {
		return VersionCommandResult{}, err
	}
	index.Items = append(index.Items, version)
	index.CurrentID = version.ID
	settings = normalizeVersionAutoSettings(settings)
	if err := s.saveIndex(index); err != nil {
		return VersionCommandResult{}, err
	}
	if err := s.pruneAutoVersions(settings.Retention); err != nil {
		return VersionCommandResult{}, err
	}
	status, statusErr := s.statusLocked(settings)
	result := VersionCommandResult{Message: "版本已保存", Version: &version}
	if statusErr == nil {
		result.Status = &status
	}
	return result, nil
}

func (s *Service) createSnapshot(message, source string, base *VersionEntry) (VersionEntry, error) {
	files, err := s.collectVisibleFiles()
	if err != nil {
		return VersionEntry{}, err
	}
	now := time.Now()
	var total int64
	for _, file := range files {
		total += file.Size
	}
	changed := make([]string, 0)
	if base != nil {
		changes, err := s.diffChanges(*base)
		if err != nil {
			return VersionEntry{}, err
		}
		for _, change := range changes {
			changed = append(changed, change.Path)
		}
	} else {
		for _, file := range files {
			changed = append(changed, file.Path)
		}
	}
	sort.Strings(changed)
	repo, err := s.openVersionRepo()
	if err != nil {
		return VersionEntry{}, err
	}
	hash, err := s.commitWorkspaceSnapshot(repo, files, message, now)
	if err != nil {
		return VersionEntry{}, err
	}
	version := VersionEntry{
		ID:           hash.String(),
		Message:      message,
		CreatedAt:    now.Format(time.RFC3339),
		Source:       source,
		FileCount:    len(files),
		TotalBytes:   total,
		ChangedPaths: changed,
	}
	return version, nil
}
