package automation

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ErrRunIdentityConflict means a deterministic run ID already names a run
// with different execution semantics. Durable callers must surface the
// conflict instead of allocating a replacement identity and duplicating work.
var ErrRunIdentityConflict = errors.New("automation run identity conflict")

var (
	// ErrTaskArchived rejects new definition/runtime admissions after a user
	// deleted a task while preserving its recovery ledger.
	ErrTaskArchived = errors.New("automation task is archived")
	// ErrTaskHasActiveRun prevents a delete request from hiding live runtime
	// control while an accepted operation still needs reconciliation.
	ErrTaskHasActiveRun = errors.New("automation task has an active run")
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

	// SessionStrategyPerRun isolates every trigger occurrence in its own
	// project conversation. SessionStrategyPerTask keeps one task-owned
	// conversation so later runs can intentionally reuse prior context.
	SessionStrategyPerRun  = "per_run"
	SessionStrategyPerTask = "per_task"

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

// ExecutionTarget identifies the Project in which an automation executes.
// TargetKindUser remains readable only for Beta definitions created before
// Automations became strictly Project-owned.
type ExecutionTarget struct {
	Kind      string `json:"kind"`
	ProjectID string `json:"project_id,omitempty"`
	Workspace string `json:"workspace,omitempty"`
}

// UnmarshalJSON keeps released automation definitions readable while making
// project_id the only canonical field written by current versions.
func (target *ExecutionTarget) UnmarshalJSON(data []byte) error {
	if target == nil {
		return errors.New("automation execution target is nil")
	}
	var decoded struct {
		Kind              string `json:"kind"`
		ProjectID         string `json:"project_id,omitempty"`
		LegacyWorkspaceID string `json:"workspace_id,omitempty"`
		Workspace         string `json:"workspace,omitempty"`
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	projectID := strings.TrimSpace(decoded.ProjectID)
	legacyID := strings.TrimSpace(decoded.LegacyWorkspaceID)
	if projectID != "" && legacyID != "" && projectID != legacyID {
		return fmt.Errorf("automation target has conflicting project_id and legacy workspace_id")
	}
	if projectID == "" {
		projectID = legacyID
	}
	*target = ExecutionTarget{
		Kind:      decoded.Kind,
		ProjectID: projectID,
		Workspace: decoded.Workspace,
	}
	return nil
}

const (
	MaxRecentRuns = 20
	MaxInboxItems = 100
)

// Task describes one Project-owned automation definition. Its Prompt runs with
// the owning Project Agent's configured capabilities; task configuration never
// adds a second permission or output policy layer.
type Task struct {
	ID                  string                  `json:"id"`
	CatalogID           string                  `json:"catalog_id,omitempty"`
	Revision            string                  `json:"revision,omitempty"`
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
	SessionStrategy     string                  `json:"session_strategy"`
	LastRun             *RunRecord              `json:"last_run,omitempty"`
	RecentRuns          []RunRecord             `json:"recent_runs"`
	CreatedAt           time.Time               `json:"created_at"`
	UpdatedAt           time.Time               `json:"updated_at"`
	// ArchivedAt is a tombstone. Archived definitions disappear from normal
	// catalogs but remain addressable by recovery and completion outbox code.
	ArchivedAt *time.Time `json:"archived_at,omitempty"`
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
	SessionStrategy     string              `json:"session_strategy"`
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
	// Evaluation is the durable trigger coordinator record. Semantic triggers
	// retain their bounded model input; schedule and chapter triggers retain the
	// canonical match they derived. Every trigger type therefore resumes the
	// same claimed -> decided -> completed protocol after a restart.
	Evaluation *TriggerEvaluationRecord `json:"evaluation,omitempty"`
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
	ID              string `json:"id"`
	TaskID          string `json:"task_id"`
	TaskRevision    string `json:"task_revision,omitempty"`
	SessionID       string `json:"session_id,omitempty"`
	SessionStrategy string `json:"session_strategy,omitempty"`
	// TurnID is the immutable root AgentChat command anchor. SessionID locates
	// the conversation; TurnID locates this exact run inside a reused session.
	TurnID          string            `json:"turn_id,omitempty"`
	ProjectID       string            `json:"project_id,omitempty"`
	Scope           string            `json:"scope"`
	Workspace       string            `json:"workspace,omitempty"`
	Trigger         string            `json:"trigger"`
	SourceRunID     string            `json:"source_run_id,omitempty"`
	TriggerEvidence []TriggerEvidence `json:"trigger_evidence,omitempty"`
	// RootRuntime* is the immutable StartTurn receipt. Runtime* is the current
	// operation receipt exposed to clients and advances when a follow-up is
	// accepted, so Stop always targets the live operation without erasing the
	// root admission proof used by trigger/inbox coordinators.
	RootRuntimeCommandID      string `json:"root_runtime_command_id,omitempty"`
	RootRuntimeOperationID    string `json:"root_runtime_operation_id,omitempty"`
	RootRuntimeReceiptCursor  uint64 `json:"root_runtime_receipt_cursor,omitempty"`
	RuntimeCommandID          string `json:"runtime_command_id,omitempty"`
	RuntimeOperationID        string `json:"runtime_operation_id,omitempty"`
	RuntimeReceiptCursor      uint64 `json:"runtime_receipt_cursor,omitempty"`
	RuntimeCommandFingerprint string `json:"runtime_command_fingerprint,omitempty"`
	RuntimeIntentHash         string `json:"runtime_intent_hash,omitempty"`
	// PendingRuntime* is the write-ahead successor intent. It closes the crash
	// window between persisting a follow-up command identity and receiving its
	// durable runtime receipt; startup reconciliation promotes the matching
	// operation into Runtime* without ever changing RootRuntime*.
	PendingRuntimeCommandID          string `json:"pending_runtime_command_id,omitempty"`
	PendingRuntimeIntentHash         string `json:"pending_runtime_intent_hash,omitempty"`
	PendingRuntimeCommandFingerprint string `json:"pending_runtime_command_fingerprint,omitempty"`
	// RuntimeSuccessorConflict records why a pending successor was discarded
	// without promotion. It lets the run ledger distinguish a verified runtime
	// rejection from an unsafe caller clearing accepted work.
	RuntimeSuccessorConflict string `json:"runtime_successor_conflict,omitempty"`
	// RuntimeAdmissionPending is the write-ahead side of the initial StartTurn
	// boundary. It is persisted before Runtime acceptance and cleared only by an
	// exact receipt or by recovery proving that no command was accepted.
	RuntimeAdmissionPending bool `json:"runtime_admission_pending,omitempty"`
	// RuntimeRecoveryRequired is a durable obligation, not a display hint. A
	// cold accepted StartTurn remains pending until an owned recovery observer
	// sees an explicit control action or terminal runtime reconciliation.
	RuntimeRecoveryRequired bool `json:"runtime_recovery_required,omitempty"`
	// CompletionEffectsPending makes terminal post-effects restartable. Those
	// effects use deterministic downstream identities before this flag clears.
	CompletionEffectsPending     bool   `json:"completion_effects_pending,omitempty"`
	CompletionEffectsCompleted   bool   `json:"completion_effects_completed,omitempty"`
	CompletionEffectsOperationID string `json:"completion_effects_operation_id,omitempty"`
	WriteConfirmationRequired    bool   `json:"write_confirmation_required,omitempty"`
	// WriteConfirmationPolicyCaptured distinguishes an intentionally false
	// admission policy from legacy records that predate persisted effect plans.
	WriteConfirmationPolicyCaptured bool     `json:"write_confirmation_policy_captured,omitempty"`
	CompletionMutationPaths         []string `json:"completion_mutation_paths,omitempty"`
	// CompletionMutationEffectIDs preserve the Runtime outbox identities that
	// transferred ownership of mutation-trigger work into this run ledger.
	// They are never inferred from display history or tool result previews.
	CompletionMutationEffectIDs []string           `json:"completion_mutation_effect_ids,omitempty"`
	Status                      string             `json:"status"`
	StartedAt                   time.Time          `json:"started_at"`
	FinishedAt                  time.Time          `json:"finished_at,omitempty"`
	Summary                     string             `json:"summary"`
	Error                       string             `json:"error,omitempty"`
	OutputPath                  string             `json:"output_path,omitempty"`
	ToolManifest                []ToolManifestItem `json:"tool_manifest"`
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
	ProjectID    string            `json:"project_id,omitempty"`
	Workspace    string            `json:"workspace,omitempty"`
	Status       string            `json:"status"`
	ActionPolicy string            `json:"action_policy"`
	NotifyPolicy string            `json:"notify_policy"`
	Title        string            `json:"title"`
	Summary      string            `json:"summary"`
	ActionError  string            `json:"action_error,omitempty"`
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
