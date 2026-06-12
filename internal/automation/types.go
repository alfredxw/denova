package automation

import "time"

const (
	ScopeUser      = "user"
	ScopeWorkspace = "workspace"

	TemplateMemoryConsolidation = "memory_consolidation"
	TemplateReview              = "review"
	TemplateContinueWriting     = "continue_writing"
	TemplateCustomPrompt        = "custom_prompt"

	WritePolicyReadOnly              = "read_only"
	WritePolicyAllowLoreWrite        = "allow_lore_write"
	WritePolicyAllowFileWrite        = "allow_file_write"
	WritePolicyAllowLoreAndFileWrite = "allow_lore_and_file_write"

	OutputPolicyRunRecordOnly = "run_record_only"
	OutputPolicyOptionalFile  = "optional_file"

	RunStatusRunning = "running"
	RunStatusSuccess = "success"
	RunStatusFailed  = "failed"
	RunStatusAborted = "aborted"

	TriggerManual   = "manual"
	TriggerSchedule = "schedule"
)

const (
	MaxRecentRuns = 20
)

// Task describes one bounded, permission-aware automation definition.
type Task struct {
	ID           string      `json:"id"`
	Scope        string      `json:"scope"`
	Enabled      bool        `json:"enabled"`
	Name         string      `json:"name"`
	Template     string      `json:"template"`
	Prompt       string      `json:"prompt"`
	Schedule     Schedule    `json:"schedule"`
	WritePolicy  string      `json:"write_policy"`
	OutputPolicy string      `json:"output_policy"`
	OutputPath   string      `json:"output_path"`
	LastRun      *RunRecord  `json:"last_run,omitempty"`
	RecentRuns   []RunRecord `json:"recent_runs"`
	CreatedAt    time.Time   `json:"created_at"`
	UpdatedAt    time.Time   `json:"updated_at"`
}

// Schedule stores a user-editable cron-style cadence without requiring raw cron input.
type Schedule struct {
	Kind       string `json:"kind"`
	EveryHours int    `json:"every_hours,omitempty"`
	Weekday    int    `json:"weekday,omitempty"`
	DayOfMonth int    `json:"day_of_month,omitempty"`
	Hour       int    `json:"hour"`
	Minute     int    `json:"minute"`
	Cron       string `json:"cron"`
}

// RunRecord is a persisted, bounded execution summary.
type RunRecord struct {
	ID           string             `json:"id"`
	TaskID       string             `json:"task_id"`
	SessionID    string             `json:"session_id,omitempty"`
	Scope        string             `json:"scope"`
	Workspace    string             `json:"workspace,omitempty"`
	Trigger      string             `json:"trigger"`
	Status       string             `json:"status"`
	StartedAt    time.Time          `json:"started_at"`
	FinishedAt   time.Time          `json:"finished_at,omitempty"`
	Summary      string             `json:"summary"`
	Error        string             `json:"error,omitempty"`
	OutputPath   string             `json:"output_path,omitempty"`
	ToolManifest []ToolManifestItem `json:"tool_manifest"`
}

// ToolManifestItem records the effective tool permission used by one automation run.
type ToolManifestItem struct {
	Source  string `json:"source"`
	Allowed bool   `json:"allowed"`
}

type ListResult struct {
	Tasks []Task `json:"tasks"`
}

type RunResult struct {
	Task Task      `json:"task"`
	Run  RunRecord `json:"run"`
}

type ActiveRun struct {
	Run    RunRecord `json:"run"`
	TaskID string    `json:"task_id"`
}

type ActiveRunsResult struct {
	Runs []ActiveRun `json:"runs"`
}
