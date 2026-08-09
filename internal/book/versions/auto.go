package versions

import (
	"errors"
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
	_, lastTimedAt, err := s.latestVersionTimes()
	if err != nil {
		return VersionAutoResult{}, err
	}
	retryAfter := timedVersionRetryAfterTimestamp(lastTimedAt, settings.TimedIntervalMinutes, time.Now())
	if retryAfter > 0 {
		return VersionAutoResult{Skipped: true, Reason: "未到自动版本最小间隔", RetryAfter: retryAfter}, nil
	}
	result, err := s.createLocked(fmt.Sprintf("自动版本：%s", time.Now().Format("2006-01-02 15:04")), VersionSourceTimer, settings)
	if errors.Is(err, ErrVersionClean) {
		return VersionAutoResult{Skipped: true, Reason: "工作区无变更"}, nil
	}
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

func timedVersionRetryAfterTimestamp(createdAt string, intervalMinutes int, now time.Time) time.Duration {
	if createdAt == "" {
		return 0
	}
	if intervalMinutes <= 0 {
		intervalMinutes = DefaultTimedVersionIntervalMinutes
	}
	t, err := time.Parse(time.RFC3339, createdAt)
	if err != nil {
		return 0
	}
	retryAfter := time.Duration(intervalMinutes)*time.Minute - now.Sub(t)
	if retryAfter <= 0 {
		return 0
	}
	return retryAfter
}
