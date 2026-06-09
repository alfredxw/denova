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
		return VersionAutoResult{Skipped: true, Reason: "定时版本已关闭"}, nil
	}
	items, err := s.loadVersions()
	if err != nil {
		return VersionAutoResult{}, err
	}
	if !shouldCreateTimedVersion(items, settings.TimedIntervalMinutes) {
		return VersionAutoResult{Skipped: true, Reason: "未到定时保存间隔"}, nil
	}
	status, err := s.statusLocked(settings)
	if err != nil {
		return VersionAutoResult{}, err
	}
	if status.Clean {
		return VersionAutoResult{Skipped: true, Reason: "工作区无变更"}, nil
	}
	result, err := s.createLocked(fmt.Sprintf("定时自动保存：%s", time.Now().Format("2006-01-02 15:04")), VersionSourceTimer, settings)
	if err != nil {
		return VersionAutoResult{}, err
	}
	return VersionAutoResult{Version: result.Version}, nil
}

func (s *Service) CaptureState() (VersionWorkspaceState, error) {
	files, err := s.collectVisibleFiles()
	if err != nil {
		return VersionWorkspaceState{}, err
	}
	state := VersionWorkspaceState{Files: make(map[string]VersionFileState, len(files))}
	for _, file := range files {
		state.Files[file.Path] = VersionFileState{
			Hash:  file.Hash,
			Size:  file.Size,
			Chars: file.Chars,
			Text:  file.Text,
		}
	}
	return state, nil
}

func (s *Service) MaybeCreateAgent(before VersionWorkspaceState, settings VersionAutoSettings) (VersionAutoResult, error) {
	settings = normalizeVersionAutoSettings(settings)
	if !settings.AgentEnabled {
		return VersionAutoResult{Skipped: true, Reason: "Agent 自动版本已关闭"}, nil
	}
	after, err := s.CaptureState()
	if err != nil {
		return VersionAutoResult{}, err
	}
	chars := changedTextChars(before, after)
	if chars < settings.AgentCharThreshold {
		return VersionAutoResult{Skipped: true, Reason: "Agent 写入字数未达阈值", Chars: chars}, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	result, err := s.createLocked(fmt.Sprintf("Agent 自动保存：%s（约 %d 字变更）", time.Now().Format("2006-01-02 15:04"), chars), VersionSourceAgent, settings)
	if errors.Is(err, ErrVersionClean) {
		return VersionAutoResult{Skipped: true, Reason: "工作区无变更", Chars: chars}, nil
	}
	if err != nil {
		return VersionAutoResult{}, err
	}
	return VersionAutoResult{Chars: chars, Version: result.Version}, nil
}

func normalizeVersionAutoSettings(settings VersionAutoSettings) VersionAutoSettings {
	defaults := DefaultAutoSettings()
	if settings.TimedIntervalMinutes <= 0 {
		settings.TimedIntervalMinutes = defaults.TimedIntervalMinutes
	}
	if settings.AgentCharThreshold <= 0 {
		settings.AgentCharThreshold = defaults.AgentCharThreshold
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

func shouldCreateTimedVersion(items []VersionEntry, intervalMinutes int) bool {
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
		return true
	}
	t, err := time.Parse(time.RFC3339, latest.CreatedAt)
	if err != nil {
		return true
	}
	return time.Since(t) >= time.Duration(intervalMinutes)*time.Minute
}

func changedTextChars(before, after VersionWorkspaceState) int {
	total := 0
	seen := map[string]bool{}
	for path, next := range after.Files {
		seen[path] = true
		prev, ok := before.Files[path]
		if ok && prev.Hash == next.Hash {
			continue
		}
		if !next.Text && !(ok && prev.Text) {
			continue
		}
		if !ok {
			total += next.Chars
			continue
		}
		total += changedCharEstimate(prev.Chars, next.Chars)
	}
	for path, prev := range before.Files {
		if seen[path] || !prev.Text {
			continue
		}
		total += prev.Chars
	}
	return total
}

func changedCharEstimate(beforeChars, afterChars int) int {
	if beforeChars < 0 {
		beforeChars = 0
	}
	if afterChars < 0 {
		afterChars = 0
	}
	diff := afterChars - beforeChars
	if diff < 0 {
		diff = -diff
	}
	if diff > 0 {
		return diff
	}
	if afterChars > beforeChars {
		return afterChars
	}
	return beforeChars
}
