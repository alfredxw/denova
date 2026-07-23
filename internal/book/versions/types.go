package versions

import (
	"errors"
	"time"
)

const (
	DefaultTimedVersionIntervalMinutes = 10
	DefaultAutoVersionRetention        = 100
)

const (
	VersionSourceManual         = "manual"
	VersionSourceTimer          = "timer"
	VersionSourceAgent          = "agent"
	VersionSourceRollbackBackup = "rollback_backup"
)

const (
	VersionRestoreScopeWorkspace = "workspace"
	VersionRestoreScopePaths     = "paths"
)

var (
	ErrVersionNotFound = errors.New("版本不存在")
	ErrVersionClean    = errors.New("当前工作区没有可保存的变更")
)

// VersionEntry 表示一本书的一次本地版本库提交。
type VersionEntry struct {
	ID           string   `json:"id"`
	Message      string   `json:"message"`
	CreatedAt    string   `json:"created_at"`
	Source       string   `json:"source"`
	FileCount    int      `json:"file_count"`
	TotalBytes   int64    `json:"total_bytes"`
	ChangedPaths []string `json:"changed_paths"`
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

type VersionRestorePlan struct {
	Target           VersionEntry           `json:"target"`
	Scope            string                 `json:"scope"`
	Paths            []string               `json:"paths"`
	Changes          []VersionRestoreChange `json:"changes"`
	WillCreateBackup bool                   `json:"will_create_backup"`
	CurrentDirty     bool                   `json:"current_dirty"`
	BackupMessage    string                 `json:"backup_message,omitempty"`
	Warnings         []string               `json:"warnings,omitempty"`
}

type VersionRestoreResult struct {
	Message       string         `json:"message"`
	Target        VersionEntry   `json:"target"`
	Version       *VersionEntry  `json:"version,omitempty"`
	BackupVersion *VersionEntry  `json:"backup_version,omitempty"`
	RestoredPaths []string       `json:"restored_paths"`
	Scope         string         `json:"scope"`
	Status        *VersionStatus `json:"status,omitempty"`
}

type VersionRestoreChange struct {
	Path               string `json:"path"`
	Status             string `json:"status"`
	Text               bool   `json:"text"`
	Binary             bool   `json:"binary"`
	MissingInVersion   bool   `json:"missing_in_version,omitempty"`
	MissingInWorkspace bool   `json:"missing_in_workspace,omitempty"`
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
	Retention            int
}

type VersionAutoResult struct {
	Skipped    bool
	Reason     string
	Version    *VersionEntry
	RetryAfter time.Duration
}

type versionFileData struct {
	Path  string
	Abs   string
	Hash  string
	Size  int64
	Chars int
	Text  bool
}
