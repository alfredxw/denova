// Package taskcenter defines the stable API contract for resumable background
// work. Producers keep their own detailed logs and payloads; this package only
// exposes the identity, actionable state, and recovery destination shared by
// every long-running workflow.
package taskcenter

import "time"

type TaskType string

const (
	TaskTypeAgent            TaskType = "agent"
	TaskTypeAutomation       TaskType = "automation"
	TaskTypeInteractiveStory TaskType = "interactive_story"
	TaskTypeImageGeneration  TaskType = "image_generation"
	TaskTypeImportExport     TaskType = "import_export"
)

type Status string

const (
	StatusRunning     Status = "running"
	StatusWaitingUser Status = "waiting_user"
	StatusFailed      Status = "failed"
	StatusCompleted   Status = "completed"
	StatusStopped     Status = "stopped"
)

type RecoveryKind string

const (
	RecoveryAutomationRun    RecoveryKind = "automation_run"
	RecoveryAutomationInbox  RecoveryKind = "automation_inbox"
	RecoveryAgentSession     RecoveryKind = "agent_session"
	RecoveryConfigManager    RecoveryKind = "config_manager"
	RecoveryInteractiveStory RecoveryKind = "interactive_story"
	RecoveryImageGeneration  RecoveryKind = "image_generation"
	RecoveryImportExport     RecoveryKind = "import_export"
)

type ProjectRef struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

type RecoveryTarget struct {
	Kind       RecoveryKind `json:"kind"`
	Workspace  string       `json:"workspace"`
	TaskID     string       `json:"task_id,omitempty"`
	SessionID  string       `json:"session_id,omitempty"`
	Origin     string       `json:"origin,omitempty"`
	ResourceID string       `json:"resource_id,omitempty"`
	StoryID    string       `json:"story_id,omitempty"`
	BranchID   string       `json:"branch_id,omitempty"`
	RunID      string       `json:"run_id,omitempty"`
	InboxID    string       `json:"inbox_id,omitempty"`
}

type Task struct {
	ID        string         `json:"id"`
	Type      TaskType       `json:"type"`
	Status    Status         `json:"status"`
	Title     string         `json:"title"`
	Project   ProjectRef     `json:"project"`
	StartedAt time.Time      `json:"started_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	Recovery  RecoveryTarget `json:"recovery"`
	Error     string         `json:"error,omitempty"`
}

type ListResult struct {
	Tasks               []Task `json:"tasks"`
	ActionRequiredCount int    `json:"action_required_count"`
}

func IsActionRequired(status Status) bool {
	return status == StatusWaitingUser || status == StatusFailed
}
