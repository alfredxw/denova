package book

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf8"
)

const (
	DefaultTimedVersionIntervalMinutes = 10
	DefaultAgentVersionCharThreshold   = 3000
	DefaultAutoVersionRetention        = 100

	versionIndexVersion = 1
)

const (
	VersionSourceManual         = "manual"
	VersionSourceTimer          = "timer"
	VersionSourceAgent          = "agent"
	VersionSourceRollbackBackup = "rollback_backup"
)

var (
	ErrVersionNotFound = errors.New("版本不存在")
	ErrVersionClean    = errors.New("当前工作区没有可保存的变更")
)

// VersionEntry 表示一本书的一次原生快照版本。
type VersionEntry struct {
	ID           string   `json:"id"`
	Message      string   `json:"message"`
	CreatedAt    string   `json:"created_at"`
	Source       string   `json:"source"`
	FileCount    int      `json:"file_count"`
	TotalBytes   int64    `json:"total_bytes"`
	ChangedPaths []string `json:"changed_paths"`
}

type VersionIndex struct {
	Version   int            `json:"version"`
	CurrentID string         `json:"current_id,omitempty"`
	Items     []VersionEntry `json:"items"`
}

type VersionStatus struct {
	HasVersions bool            `json:"has_versions"`
	Clean       bool            `json:"clean"`
	Changes     []VersionChange `json:"changes"`
	Latest      *VersionEntry   `json:"latest,omitempty"`
	Auto        VersionAutoInfo `json:"auto"`
}

type VersionAutoInfo struct {
	TimedEnabled         bool   `json:"timed_enabled"`
	TimedIntervalMinutes int    `json:"timed_interval_minutes"`
	AgentEnabled         bool   `json:"agent_enabled"`
	AgentCharThreshold   int    `json:"agent_char_threshold"`
	Retention            int    `json:"retention"`
	LastAutoAt           string `json:"last_auto_at,omitempty"`
}

type VersionChange struct {
	Path   string `json:"path"`
	Status string `json:"status"`
}

type VersionCommandResult struct {
	Message string         `json:"message"`
	Version *VersionEntry  `json:"version,omitempty"`
	Status  *VersionStatus `json:"status,omitempty"`
}

type VersionDiff struct {
	Version            VersionEntry    `json:"version"`
	Changes            []VersionChange `json:"changes"`
	Path               string          `json:"path,omitempty"`
	Original           string          `json:"original,omitempty"`
	Modified           string          `json:"modified,omitempty"`
	Text               bool            `json:"text"`
	Binary             bool            `json:"binary"`
	MissingInVersion   bool            `json:"missing_in_version,omitempty"`
	MissingInWorkspace bool            `json:"missing_in_workspace,omitempty"`
}

type VersionAutoSettings struct {
	TimedEnabled         bool
	TimedIntervalMinutes int
	AgentEnabled         bool
	AgentCharThreshold   int
	Retention            int
}

type VersionAutoResult struct {
	Skipped bool
	Reason  string
	Chars   int
	Version *VersionEntry
}

type VersionWorkspaceState struct {
	Files map[string]VersionFileState
}

type VersionFileState struct {
	Hash  string
	Size  int64
	Chars int
	Text  bool
}

type versionFileData struct {
	Path  string
	Abs   string
	Hash  string
	Size  int64
	Chars int
	Text  bool
}

// VersionService 管理当前书籍 workspace 的 Nova 原生快照版本。
type VersionService struct {
	workspace string
	mu        sync.Mutex
}

func NewVersionService(workspace string) *VersionService {
	return &VersionService{workspace: workspace}
}

func DefaultVersionAutoSettings() VersionAutoSettings {
	return VersionAutoSettings{
		TimedEnabled:         true,
		TimedIntervalMinutes: DefaultTimedVersionIntervalMinutes,
		AgentEnabled:         true,
		AgentCharThreshold:   DefaultAgentVersionCharThreshold,
		Retention:            DefaultAutoVersionRetention,
	}
}

func (s *VersionService) Status(settings VersionAutoSettings) (VersionStatus, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.statusLocked(settings)
}

func (s *VersionService) statusLocked(settings VersionAutoSettings) (VersionStatus, error) {
	index, err := s.loadIndex()
	if err != nil {
		return VersionStatus{}, err
	}
	current := currentVersion(index.Items, index.CurrentID)
	changes := []VersionChange{}
	if current != nil {
		changes, err = s.diffChanges(*current)
		if err != nil {
			return VersionStatus{}, err
		}
	} else {
		files, err := s.collectVisibleFiles()
		if err != nil {
			return VersionStatus{}, err
		}
		changes = make([]VersionChange, 0, len(files))
		for _, file := range files {
			changes = append(changes, VersionChange{Path: file.Path, Status: "added"})
		}
	}
	settings = normalizeVersionAutoSettings(settings)
	return VersionStatus{
		HasVersions: len(index.Items) > 0,
		Clean:       len(changes) == 0,
		Changes:     changes,
		Latest:      current,
		Auto: VersionAutoInfo{
			TimedEnabled:         settings.TimedEnabled,
			TimedIntervalMinutes: settings.TimedIntervalMinutes,
			AgentEnabled:         settings.AgentEnabled,
			AgentCharThreshold:   settings.AgentCharThreshold,
			Retention:            settings.Retention,
			LastAutoAt:           lastAutoVersionAt(index.Items),
		},
	}, nil
}

func (s *VersionService) History(limit int) ([]VersionEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if limit <= 0 {
		limit = 30
	}
	if limit > 200 {
		limit = 200
	}
	index, err := s.loadIndex()
	if err != nil {
		return nil, err
	}
	items := append([]VersionEntry(nil), index.Items...)
	sortVersionsDesc(items)
	if len(items) > limit {
		items = items[:limit]
	}
	return items, nil
}

func (s *VersionService) Create(message, source string, settings VersionAutoSettings) (VersionCommandResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.createLocked(message, source, settings)
}

func (s *VersionService) createLocked(message, source string, settings VersionAutoSettings) (VersionCommandResult, error) {
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

func (s *VersionService) MaybeCreateTimed(settings VersionAutoSettings) (VersionAutoResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	settings = normalizeVersionAutoSettings(settings)
	if !settings.TimedEnabled {
		return VersionAutoResult{Skipped: true, Reason: "定时版本已关闭"}, nil
	}
	index, err := s.loadIndex()
	if err != nil {
		return VersionAutoResult{}, err
	}
	if !shouldCreateTimedVersion(index.Items, settings.TimedIntervalMinutes) {
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

func (s *VersionService) CaptureState() (VersionWorkspaceState, error) {
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

func (s *VersionService) MaybeCreateAgent(before VersionWorkspaceState, settings VersionAutoSettings) (VersionAutoResult, error) {
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

func (s *VersionService) Diff(id, path string) (VersionDiff, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	version, err := s.findVersion(id)
	if err != nil {
		return VersionDiff{}, err
	}
	changes, err := s.diffChanges(version)
	if err != nil {
		return VersionDiff{}, err
	}
	diff := VersionDiff{Version: version, Changes: changes}
	path = strings.TrimSpace(path)
	if path == "" {
		return diff, nil
	}
	if _, err := SafePath(s.workspace, path); err != nil {
		return VersionDiff{}, err
	}
	diff.Path = filepath.ToSlash(filepath.Clean(filepath.FromSlash(path)))
	versionPath := filepath.Join(s.snapshotDir(version.ID), filepath.FromSlash(diff.Path))
	workspacePath := filepath.Join(s.workspace, filepath.FromSlash(diff.Path))
	original, originalErr := os.ReadFile(versionPath)
	modified, modifiedErr := os.ReadFile(workspacePath)
	if errors.Is(originalErr, os.ErrNotExist) {
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

func (s *VersionService) Restore(id string, settings VersionAutoSettings) (VersionCommandResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	version, err := s.findVersion(id)
	if err != nil {
		return VersionCommandResult{}, err
	}
	settings = normalizeVersionAutoSettings(settings)
	status, err := s.statusLocked(settings)
	if err != nil {
		return VersionCommandResult{}, err
	}
	if !status.Clean {
		if _, err := s.createLocked("回滚前自动备份", VersionSourceRollbackBackup, settings); err != nil && !errors.Is(err, ErrVersionClean) {
			return VersionCommandResult{}, fmt.Errorf("创建回滚前自动备份失败: %w", err)
		}
	}
	if err := s.restoreSnapshot(version); err != nil {
		return VersionCommandResult{}, err
	}
	index, err := s.loadIndex()
	if err != nil {
		return VersionCommandResult{}, err
	}
	index.CurrentID = version.ID
	if err := s.saveIndex(index); err != nil {
		return VersionCommandResult{}, err
	}
	nextStatus, statusErr := s.statusLocked(settings)
	result := VersionCommandResult{Message: "已恢复到所选版本", Version: &version}
	if statusErr == nil {
		result.Status = &nextStatus
	}
	return result, nil
}

func (s *VersionService) createSnapshot(message, source string, base *VersionEntry) (VersionEntry, error) {
	files, err := s.collectVisibleFiles()
	if err != nil {
		return VersionEntry{}, err
	}
	now := time.Now()
	id := "v" + now.Format("20060102150405") + "-" + randomVersionSuffix(files)
	dstRoot := s.snapshotDir(id)
	if err := os.MkdirAll(dstRoot, 0o755); err != nil {
		return VersionEntry{}, err
	}
	var total int64
	for _, file := range files {
		total += file.Size
		dst := filepath.Join(dstRoot, filepath.FromSlash(file.Path))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return VersionEntry{}, err
		}
		if err := copyVersionFile(file.Abs, dst); err != nil {
			return VersionEntry{}, err
		}
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
	version := VersionEntry{
		ID:           id,
		Message:      message,
		CreatedAt:    now.Format(time.RFC3339),
		Source:       source,
		FileCount:    len(files),
		TotalBytes:   total,
		ChangedPaths: changed,
	}
	if err := s.saveManifest(version); err != nil {
		return VersionEntry{}, err
	}
	return version, nil
}

func (s *VersionService) diffChanges(version VersionEntry) ([]VersionChange, error) {
	currentFiles, err := s.collectVisibleFiles()
	if err != nil {
		return nil, err
	}
	current := make(map[string]string, len(currentFiles))
	for _, file := range currentFiles {
		current[file.Path] = file.Hash
	}
	snapshot, err := s.collectSnapshotFiles(version.ID)
	if err != nil {
		return nil, err
	}
	changes := make([]VersionChange, 0)
	seen := map[string]bool{}
	for path, hash := range current {
		seen[path] = true
		oldHash, ok := snapshot[path]
		if !ok {
			changes = append(changes, VersionChange{Path: path, Status: "added"})
			continue
		}
		if oldHash != hash {
			changes = append(changes, VersionChange{Path: path, Status: "modified"})
		}
	}
	for path := range snapshot {
		if !seen[path] {
			changes = append(changes, VersionChange{Path: path, Status: "deleted"})
		}
	}
	sort.SliceStable(changes, func(i, j int) bool { return changes[i].Path < changes[j].Path })
	return changes, nil
}

func (s *VersionService) restoreSnapshot(version VersionEntry) error {
	files, err := s.collectVisibleFiles()
	if err != nil {
		return err
	}
	for _, file := range files {
		if err := os.Remove(file.Abs); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	if err := s.removeEmptyVisibleDirs(); err != nil {
		return err
	}
	srcRoot := s.snapshotDir(version.ID)
	return filepath.WalkDir(srcRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == srcRoot || entry.IsDir() {
			return nil
		}
		if entry.Name() == "manifest.json" && filepath.Dir(path) == srcRoot {
			return nil
		}
		rel, err := filepath.Rel(srcRoot, path)
		if err != nil {
			return err
		}
		dst := filepath.Join(s.workspace, rel)
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		return copyVersionFile(path, dst)
	})
}

func (s *VersionService) collectVisibleFiles() ([]versionFileData, error) {
	return collectVersionFiles(s.workspace, s.workspace)
}

func (s *VersionService) removeEmptyVisibleDirs() error {
	dirs := []string{}
	err := filepath.WalkDir(s.workspace, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if path == s.workspace {
			return nil
		}
		if !entry.IsDir() {
			return nil
		}
		if strings.HasPrefix(entry.Name(), ".") {
			return filepath.SkipDir
		}
		dirs = append(dirs, path)
		return nil
	})
	if err != nil {
		return err
	}
	sort.SliceStable(dirs, func(i, j int) bool { return len(dirs[i]) > len(dirs[j]) })
	for _, dir := range dirs {
		if err := os.Remove(dir); err != nil && !errors.Is(err, os.ErrNotExist) {
			if errors.Is(err, syscall.ENOTEMPTY) || errors.Is(err, syscall.EEXIST) {
				continue
			}
			return err
		}
	}
	return nil
}

func (s *VersionService) collectSnapshotFiles(id string) (map[string]string, error) {
	files, err := collectVersionFiles(s.snapshotDir(id), s.snapshotDir(id))
	if err != nil {
		return nil, err
	}
	result := make(map[string]string, len(files))
	for _, file := range files {
		if file.Path == "manifest.json" {
			continue
		}
		result[file.Path] = file.Hash
	}
	return result, nil
}

func collectVersionFiles(root, base string) ([]versionFileData, error) {
	files := []versionFileData{}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if path == root {
			return nil
		}
		name := entry.Name()
		if strings.HasPrefix(name, ".") {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil || info.Mode()&os.ModeSymlink != 0 {
			return nil
		}
		rel, err := filepath.Rel(base, path)
		if err != nil {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		hashBytes := sha256.Sum256(data)
		text := isTextBytes(data)
		chars := 0
		if text {
			chars = utf8.RuneCount(data)
		}
		files = append(files, versionFileData{
			Path:  filepath.ToSlash(rel),
			Abs:   path,
			Hash:  hex.EncodeToString(hashBytes[:]),
			Size:  info.Size(),
			Chars: chars,
			Text:  text,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.SliceStable(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, nil
}

func (s *VersionService) loadIndex() (VersionIndex, error) {
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
	return index, nil
}

func (s *VersionService) saveIndex(index VersionIndex) error {
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

func (s *VersionService) saveManifest(version VersionEntry) error {
	data, err := json.MarshalIndent(version, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(s.snapshotDir(version.ID), "manifest.json"), append(data, '\n'), 0o644)
}

func (s *VersionService) findVersion(id string) (VersionEntry, error) {
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

func (s *VersionService) pruneAutoVersions(retention int) error {
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
			_ = os.RemoveAll(s.snapshotDir(item.ID))
			continue
		}
		next = append(next, item)
	}
	index.Items = next
	return s.saveIndex(index)
}

func (s *VersionService) versionsDir() string {
	return filepath.Join(s.workspace, ".nova", "versions")
}

func (s *VersionService) indexPath() string {
	return filepath.Join(s.versionsDir(), "index.json")
}

func (s *VersionService) snapshotDir(id string) string {
	return filepath.Join(s.versionsDir(), "snapshots", id)
}

func normalizeVersionAutoSettings(settings VersionAutoSettings) VersionAutoSettings {
	defaults := DefaultVersionAutoSettings()
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

func isPrunableAutoVersion(source string) bool {
	return source == VersionSourceTimer || source == VersionSourceAgent
}

func isTextBytes(data []byte) bool {
	if len(data) == 0 {
		return true
	}
	if !utf8.Valid(data) {
		return false
	}
	for _, b := range data {
		if b == 0 {
			return false
		}
	}
	return true
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

func randomVersionSuffix(files []versionFileData) string {
	h := sha256.New()
	_, _ = io.WriteString(h, time.Now().Format(time.RFC3339Nano))
	for _, file := range files {
		_, _ = io.WriteString(h, file.Path)
		_, _ = io.WriteString(h, file.Hash)
	}
	sum := h.Sum(nil)
	return hex.EncodeToString(sum[:])[:8]
}

func copyVersionFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}
