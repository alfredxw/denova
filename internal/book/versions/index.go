package versions

import (
	"encoding/json"
	"errors"
	"os"
	"sort"
	"strings"

	"github.com/go-git/go-git/v5/plumbing"
)

func (s *Service) loadIndex() (VersionIndex, error) {
	path := s.indexPath()
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return VersionIndex{Version: versionIndexVersion, Items: []VersionEntry{}}, nil
	}
	if err != nil {
		return VersionIndex{}, err
	}
	var index VersionIndex
	if err := json.Unmarshal(data, &index); err != nil {
		return VersionIndex{}, err
	}
	if index.Version == 0 {
		index.Version = versionIndexVersion
	}
	if index.Items == nil {
		index.Items = []VersionEntry{}
	}
	return s.filterIndexWithGitCommits(index)
}

func (s *Service) saveIndex(index VersionIndex) error {
	index.Version = versionIndexVersion
	if index.CurrentID == "" {
		if latest := latestVersion(index.Items); latest != nil {
			index.CurrentID = latest.ID
		}
	}
	sortVersionsAsc(index.Items)
	if err := os.MkdirAll(s.versionsDir(), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.indexPath(), append(data, '\n'), 0o644)
}

func (s *Service) findVersion(id string) (VersionEntry, error) {
	id = strings.TrimSpace(id)
	index, err := s.loadIndex()
	if err != nil {
		return VersionEntry{}, err
	}
	for _, item := range index.Items {
		if item.ID == id {
			return item, nil
		}
	}
	return VersionEntry{}, ErrVersionNotFound
}

func (s *Service) pruneAutoVersions(retention int) error {
	if retention <= 0 {
		retention = DefaultAutoVersionRetention
	}
	index, err := s.loadIndex()
	if err != nil {
		return err
	}
	autoItems := []VersionEntry{}
	for _, item := range index.Items {
		if isPrunableAutoVersion(item.Source) {
			autoItems = append(autoItems, item)
		}
	}
	sortVersionsDesc(autoItems)
	if len(autoItems) <= retention {
		return nil
	}
	removeIDs := map[string]bool{}
	for _, item := range autoItems[retention:] {
		removeIDs[item.ID] = true
	}
	next := index.Items[:0]
	for _, item := range index.Items {
		if item.ID != index.CurrentID && removeIDs[item.ID] {
			continue
		}
		next = append(next, item)
	}
	index.Items = next
	return s.saveIndex(index)
}

func (s *Service) filterIndexWithGitCommits(index VersionIndex) (VersionIndex, error) {
	if len(index.Items) == 0 {
		return index, nil
	}
	repo, err := s.openVersionRepo()
	if err != nil {
		return VersionIndex{}, err
	}
	filtered := index.Items[:0]
	for _, item := range index.Items {
		if item.ID == "" {
			continue
		}
		if _, err := repo.CommitObject(plumbing.NewHash(item.ID)); err != nil {
			if errors.Is(err, plumbing.ErrObjectNotFound) {
				continue
			}
			return VersionIndex{}, err
		}
		filtered = append(filtered, item)
	}
	index.Items = filtered
	if index.CurrentID != "" && !versionEntriesContain(index.Items, index.CurrentID) {
		index.CurrentID = ""
	}
	if index.CurrentID == "" {
		if latest := latestVersion(index.Items); latest != nil {
			index.CurrentID = latest.ID
		}
	}
	return index, nil
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
		return "定时自动保存"
	case VersionSourceAgent:
		return "Agent 自动保存"
	case VersionSourceRollbackBackup:
		return "回滚前自动备份"
	default:
		return "手动保存版本"
	}
}

func currentVersion(items []VersionEntry, currentID string) *VersionEntry {
	currentID = strings.TrimSpace(currentID)
	if currentID != "" {
		for _, item := range items {
			if item.ID == currentID {
				current := item
				return &current
			}
		}
	}
	return latestVersion(items)
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
