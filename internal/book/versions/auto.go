package versions

import (
	"fmt"
	"time"
)

func (s *Service) MaybeCreateTimed(settings VersionAutoSettings) (VersionAutoResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	settings = normalizeVersionAutoSettings(settings)
	if !settings.TimedEnabled {
		return VersionAutoResult{Skipped: true, Reason: "自动版本已关闭"}, nil
	}
	items, err := s.loadVersions()
	if err != nil {
		return VersionAutoResult{}, err
	}
	retryAfter := timedVersionRetryAfter(items, settings.TimedIntervalMinutes, time.Now())
	if retryAfter > 0 {
		return VersionAutoResult{Skipped: true, Reason: "未到自动版本最小间隔", RetryAfter: retryAfter}, nil
	}
	status, err := s.statusLocked(settings)
	if err != nil {
		return VersionAutoResult{}, err
	}
	if status.Clean {
		return VersionAutoResult{Skipped: true, Reason: "工作区无变更"}, nil
	}
	result, err := s.createLocked(fmt.Sprintf("自动版本：%s", time.Now().Format("2006-01-02 15:04")), VersionSourceTimer, settings)
	if err != nil {
		return VersionAutoResult{}, err
	}
	return VersionAutoResult{Version: result.Version}, nil
}

func normalizeVersionAutoSettings(settings VersionAutoSettings) VersionAutoSettings {
	defaults := DefaultAutoSettings()
	if settings.TimedIntervalMinutes <= 0 {
		settings.TimedIntervalMinutes = defaults.TimedIntervalMinutes
	}
	if settings.Retention <= 0 {
		settings.Retention = defaults.Retention
	}
	return settings
}

func lastAutoVersionAt(items []VersionEntry) string {
	autoItems := []VersionEntry{}
	for _, item := range items {
		if item.Source == VersionSourceTimer || item.Source == VersionSourceAgent {
			autoItems = append(autoItems, item)
		}
	}
	latest := latestVersion(autoItems)
	if latest == nil {
		return ""
	}
	return latest.CreatedAt
}

func timedVersionRetryAfter(items []VersionEntry, intervalMinutes int, now time.Time) time.Duration {
	if intervalMinutes <= 0 {
		intervalMinutes = DefaultTimedVersionIntervalMinutes
	}
	var latest *VersionEntry
	for _, item := range items {
		if item.Source != VersionSourceTimer {
			continue
		}
		itemCopy := item
		if latest == nil || itemCopy.CreatedAt > latest.CreatedAt {
			latest = &itemCopy
		}
	}
	if latest == nil {
		return 0
	}
	t, err := time.Parse(time.RFC3339, latest.CreatedAt)
	if err != nil {
		return 0
	}
	retryAfter := time.Duration(intervalMinutes)*time.Minute - now.Sub(t)
	if retryAfter <= 0 {
		return 0
	}
	return retryAfter
}
