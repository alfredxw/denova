package automation

import (
	"encoding/json"
	"strings"
	"time"
)

const (
	ScopeUser      = "user"
	ScopeWorkspace = "workspace"

	TargetKindUser      = "user"
	TargetKindWorkspace = "workspace"

	TemplateMemoryConsolidation = "memory_consolidation"
	TemplateReview              = "review"
	TemplateContinueWriting     = "continue_writing"
	TemplateCustomPrompt        = "custom_prompt"

	WritePolicyReadOnly              = "read_only"
	WritePolicyAllowLoreWrite        = "allow_lore_write"
	WritePolicyAllowFileWrite        = "allow_file_write"
	WritePolicyAllowLoreAndFileWrite = "allow_lore_and_file_write"

	WriteModeReadOnly     = "read_only"
	WriteModeConfirmWrite = "confirm_write"
	WriteModeAutoWrite    = "auto_write"

	WriteScopeNone        = "none"
	WriteScopeLore        = "lore"
	WriteScopeFile        = "file"
	WriteScopeLoreAndFile = "lore_and_file"

	OutputPolicyRunRecordOnly = "run_record_only"
	OutputPolicyOptionalFile  = "optional_file"

	RunStatusRunning = "running"
	RunStatusSuccess = "success"
	RunStatusFailed  = "failed"
	RunStatusAborted = "aborted"

	TriggerManual            = "manual"
	TriggerSchedule          = "schedule"
	TriggerCondition         = "condition"
	TriggerInboxConfirmation = "inbox_confirmation"
	TriggerWriteConfirmation = "write_confirmation"

	TriggerTypeManual       = "manual"
	TriggerTypeSchedule     = "schedule"
	TriggerTypeSemantic     = "semantic"
	TriggerTypeChapterBatch = "chapter_batch"

	ActionPolicyConfirm    = "confirm"
	ActionPolicyAutoRun    = "auto_run"
	ActionPolicyNotifyOnly = "notify_only"

	NotifyPolicyInbox  = "inbox"
	NotifyPolicySilent = "silent"

	InboxStatusPending   = "pending"
	InboxStatusDismissed = "dismissed"
	InboxStatusConfirmed = "confirmed"
	InboxStatusAutoRun   = "auto_run"

	InboxPurposeTrigger           = "trigger"
	InboxPurposeWriteConfirmation = "write_confirmation"
)

// ExecutionTarget identifies the context in which an automation executes.
// Every task is user-managed; workspace is an explicit target rather than an
// implicit dependency on whichever book happens to be open.
type ExecutionTarget struct {
	Kind        string `json:"kind"`
	WorkspaceID string `json:"workspace_id,omitempty"`
	Workspace   string `json:"workspace,omitempty"`
}

const (
	MaxRecentRuns = 20
	MaxInboxItems = 100
)

// Task describes one bounded, permission-aware automation definition.
type Task struct {
	ID                  string                  `json:"id"`
	CatalogID           string                  `json:"catalog_id,omitempty"`
	Scope               string                  `json:"scope"`
	Target              ExecutionTarget         `json:"target"`
	Enabled             bool                    `json:"enabled"`
	Name                string                  `json:"name"`
	Template            string                  `json:"template"`
	Prompt              string                  `json:"prompt"`
	ModelProfileID      string                  `json:"model_profile_id,omitempty"`
	Schedule            Schedule                `json:"schedule"`
	Triggers            []TriggerDefinition     `json:"triggers"`
	DefaultActionPolicy string                  `json:"default_action_policy"`
	TriggerState        map[string]TriggerState `json:"trigger_state,omitempty"`
	WriteMode           string                  `json:"write_mode"`
	WriteScope          string                  `json:"write_scope"`
	OutputPolicy        string                  `json:"output_policy"`
	OutputPath          string                  `json:"output_path"`
	LastRun             *RunRecord              `json:"last_run,omitempty"`
	RecentRuns          []RunRecord             `json:"recent_runs"`
	CreatedAt           time.Time               `json:"created_at"`
	UpdatedAt           time.Time               `json:"updated_at"`
}

// TaskTemplate is an immutable creation recipe. Selecting a template copies
// Defaults into a new user-owned task; saved tasks never stay linked to it.
type TaskTemplate struct {
	ID          string               `json:"id"`
	Version     int                  `json:"version"`
	Description string               `json:"description"`
	TargetKinds []string             `json:"target_kinds"`
	Defaults    TaskTemplateDefaults `json:"defaults"`
}

// TaskTemplateDefaults contains only editable task-definition fields. Runtime
// identity, execution history, target, and timestamps are created when the user
// explicitly saves the draft.
type TaskTemplateDefaults struct {
	Enabled             bool                `json:"enabled"`
	Name                string              `json:"name"`
	Template            string              `json:"template"`
	Prompt              string              `json:"prompt"`
	ModelProfileID      string              `json:"model_profile_id,omitempty"`
	Schedule            Schedule            `json:"schedule"`
	Triggers            []TriggerDefinition `json:"triggers"`
	DefaultActionPolicy string              `json:"default_action_policy"`
	WriteMode           string              `json:"write_mode"`
	WriteScope          string              `json:"write_scope"`
	OutputPolicy        string              `json:"output_policy"`
	OutputPath          string              `json:"output_path"`
}

// taskWithoutUnmarshal mirrors Task so UnmarshalJSON can decode without
// recursing into itself. It exists purely to break the method-set loop that
// defining UnmarshalJSON on Task otherwise creates.
type taskWithoutUnmarshal Task

// UnmarshalJSON decodes Task JSON and migrates the legacy write_policy field
// into write_mode/write_scope. write_policy stopped being written, but persisted
// tasks created before this change still carry it; decoding them here keeps the
// single representation (write_mode + write_scope) authoritative without losing
// older permissions.
func (t *Task) UnmarshalJSON(data []byte) error {
	aux := struct {
		taskWithoutUnmarshal
		LegacyWritePolicy string `json:"write_policy,omitempty"`
	}{}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	*t = Task(aux.taskWithoutUnmarshal)
	if strings.TrimSpace(t.WriteMode) == "" && strings.TrimSpace(t.WriteScope) == "" && strings.TrimSpace(aux.LegacyWritePolicy) != "" {
		t.WriteMode, t.WriteScope = writeModeScopeFromLegacyPolicy(aux.LegacyWritePolicy)
	}
	return nil
}

// TriggerDefinition describes one condition that can cause an automation task to notify or run.
type TriggerDefinition struct {
	ID                string   `json:"id"`
	Type              string   `json:"type"`
	Enabled           bool     `json:"enabled"`
	Name              string   `json:"name,omitempty"`
	ActionPolicy      string   `json:"action_policy,omitempty"`
	NotifyPolicy      string   `json:"notify_policy,omitempty"`
	Schedule          Schedule `json:"schedule,omitempty"`
	SemanticCondition string   `json:"semantic_condition,omitempty"`
	ChapterBatchSize  int      `json:"chapter_batch_size,omitempty"`
}

// TriggerState stores persisted, per-trigger evaluation state used for dedupe.
type TriggerState struct {
	LastCheckedAt              time.Time `json:"last_checked_at,omitempty"`
	LastMatchedAt              time.Time `json:"last_matched_at,omitempty"`
	LastEvidenceFingerprint    string    `json:"last_evidence_fingerprint,omitempty"`
	LastObservationFingerprint string    `json:"last_observation_fingerprint,omitempty"`
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
	ID              string             `json:"id"`
	TaskID          string             `json:"task_id"`
	SessionID       string             `json:"session_id,omitempty"`
	Scope           string             `json:"scope"`
	Workspace       string             `json:"workspace,omitempty"`
	Trigger         string             `json:"trigger"`
	SourceRunID     string             `json:"source_run_id,omitempty"`
	TriggerEvidence []TriggerEvidence  `json:"trigger_evidence,omitempty"`
	Status          string             `json:"status"`
	StartedAt       time.Time          `json:"started_at"`
	FinishedAt      time.Time          `json:"finished_at,omitempty"`
	Summary         string             `json:"summary"`
	Error           string             `json:"error,omitempty"`
	OutputPath      string             `json:"output_path,omitempty"`
	ToolManifest    []ToolManifestItem `json:"tool_manifest"`
}

type TriggerEvidence struct {
	Source  string `json:"source"`
	Title   string `json:"title"`
	Ref     string `json:"ref,omitempty"`
	Snippet string `json:"snippet,omitempty"`
}

type TriggerInboxItem struct {
	ID           string            `json:"id"`
	TaskID       string            `json:"task_id"`
	TriggerID    string            `json:"trigger_id"`
	Purpose      string            `json:"purpose,omitempty"`
	Scope        string            `json:"scope"`
	Workspace    string            `json:"workspace,omitempty"`
	Status       string            `json:"status"`
	ActionPolicy string            `json:"action_policy"`
	NotifyPolicy string            `json:"notify_policy"`
	Title        string            `json:"title"`
	Summary      string            `json:"summary"`
	Evidence     []TriggerEvidence `json:"evidence"`
	Fingerprint  string            `json:"fingerprint"`
	RunID        string            `json:"run_id,omitempty"`
	SourceRunID  string            `json:"source_run_id,omitempty"`
	CreatedAt    time.Time         `json:"created_at"`
	UpdatedAt    time.Time         `json:"updated_at"`
	ReadAt       *time.Time        `json:"read_at,omitempty"`
	HandledAt    *time.Time        `json:"handled_at,omitempty"`
}

type TriggerMatch struct {
	TaskID      string            `json:"task_id"`
	TriggerID   string            `json:"trigger_id"`
	Title       string            `json:"title"`
	Summary     string            `json:"summary"`
	Evidence    []TriggerEvidence `json:"evidence"`
	Fingerprint string            `json:"fingerprint"`
}

type InboxListResult struct {
	Items []TriggerInboxItem `json:"items"`
}

type InboxActionResult struct {
	Item TriggerInboxItem `json:"item"`
	Run  *RunRecord       `json:"run,omitempty"`
}

// ToolManifestItem records the effective tool permission used by one automation run.
type ToolManifestItem struct {
	Source  string `json:"source"`
	Allowed bool   `json:"allowed"`
}

type ListResult struct {
	Tasks []Task `json:"tasks"`
}

type TemplateListResult struct {
	Templates []TaskTemplate `json:"templates"`
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
