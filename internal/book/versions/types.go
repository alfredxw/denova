package versions

import "errors"

const (
	DefaultTimedVersionIntervalMinutes = 10
	DefaultAgentVersionCharThreshold   = 3000
	DefaultAutoVersionRetention        = 100
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
