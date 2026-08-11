package agent

import (
	"encoding/json"
	"errors"
	"fmt"

	agentsession "github.com/alfredxw/denova/agent/session"
)

type SessionKey = agentsession.Key
type SessionSelector = agentsession.Selector

func NamedSession(id string) SessionKey { return agentsession.Named(id) }

type HostData struct {
	Type    string          `json:"type"`
	Version uint16          `json:"version"`
	Data    json.RawMessage `json:"data"`
}

type Input struct {
	Text           string
	IdempotencyKey string
	Context        []ContextFragment
	Goal           *GoalMutation
	HostData       *HostData
}

func Text(value string) Input { return Input{Text: value} }

// CommandReceipt is the durable admission result of one caller command.
// Idempotency retries return the original receipt with Replayed set.
type CommandReceipt struct {
	CommandID string
	RunID     string
	Cursor    Cursor
	Replayed  bool
}

// AbortRequest describes one explicit Run termination command. Callers that
// may retry across a transport boundary should provide a stable IdempotencyKey.
type AbortRequest struct {
	Reason         string
	IdempotencyKey string
}

// QueueControlRequest describes a command targeting an accepted queued input.
// Reason is used by cancellation and ignored by interruption.
type QueueControlRequest struct {
	IdempotencyKey string
	Reason         string
}

type Cursor uint64

type EventDurability string

const (
	DurableEvent   EventDurability = "durable"
	EphemeralEvent EventDurability = "ephemeral"
)

type Event struct {
	Cursor     Cursor
	Durability EventDurability
	RunID      string
	Payload    EventPayload
}

type EventPayload interface{ eventPayload() }

type RunAccepted struct{ CommandID string }

func (RunAccepted) eventPayload() {}

type RunStarted struct{ Cycle int }

func (RunStarted) eventPayload() {}

// EventSource identifies the Agent invocation that produced a live event.
// Path is ordered from the root Agent to Source.Name.
type EventSource struct {
	Name string
	Path []string
}

type AssistantDelta struct {
	Source      EventSource
	Delta       string
	DisplayOnly bool
}

func (AssistantDelta) eventPayload() {}

type ThinkingDelta struct {
	Source      EventSource
	Delta       string
	DisplayOnly bool
}

func (ThinkingDelta) eventPayload() {}

// ModelCompleted reports one root model response, including provider-neutral
// cache usage. It is display/trace metadata and never enters model context.
type ModelCompleted struct {
	Usage          TokenUsage
	FinishReason   string
	RequestedTools []string
	Source         EventSource
}

func (ModelCompleted) eventPayload() {}

type AssistantFinal struct {
	Content  string
	Thinking string
}

func (AssistantFinal) eventPayload() {}

type ToolStarted struct {
	CallID    string
	Name      string
	Arguments json.RawMessage
	Source    EventSource
}

func (ToolStarted) eventPayload() {}

type ToolProgress struct {
	CallID string
	Delta  string
	Source EventSource
}

func (ToolProgress) eventPayload() {}

type ToolFinished struct {
	CallID     string
	Name       string
	IsError    bool
	Result     string
	Projection *ToolResult
	Source     EventSource
}

func (ToolFinished) eventPayload() {}

type ArtifactProduced struct {
	CallID   string
	Artifact ToolArtifactRef
	Source   EventSource
}

func (ArtifactProduced) eventPayload() {}

type RecoveryRequired struct{ Reason string }

func (RecoveryRequired) eventPayload() {}

type RecoveryResumed struct{}

func (RecoveryResumed) eventPayload() {}

type RecoveryActionKind string

const (
	RecoveryResumeInput      RecoveryActionKind = "resume_input"
	RecoveryResumeCompaction RecoveryActionKind = "resume_compaction"
	RecoveryAbortRun         RecoveryActionKind = "abort_run"
)

// RecoveryAction is a current, opaque authority derived from Session state.
// Callers persist only ID if they need a transport round trip; internal command
// and operation identities never cross the public Agent boundary.
type RecoveryAction struct {
	ID         string
	Kind       RecoveryActionKind
	RunID      string
	Delivery   RecoveryInputDelivery
	Compaction RecoveryCompactionAction
}

// RecoveryInputDelivery identifies the user-visible queue semantic without
// exposing the runtime command used to authorize a recovery choice.
type RecoveryInputDelivery string

const (
	RecoveryDeliverySteer    RecoveryInputDelivery = "steer"
	RecoveryDeliveryFollowUp RecoveryInputDelivery = "follow_up"
	RecoveryDeliveryNextTurn RecoveryInputDelivery = "next_turn"
)

type RecoveryCompactionAction string

const (
	RecoveryCompactionCreate RecoveryCompactionAction = "compact"
	RecoveryCompactionRemove RecoveryCompactionAction = "remove"
)

type GoalUpdated struct {
	State   GoalState
	Present bool
}

func (GoalUpdated) eventPayload() {}

type TodoStatus string

const (
	TodoPending    TodoStatus = "pending"
	TodoInProgress TodoStatus = "in_progress"
	TodoCompleted  TodoStatus = "completed"
)

type TodoItem struct {
	ID     string     `json:"id"`
	Text   string     `json:"text"`
	Status TodoStatus `json:"status"`
}

type TodoState struct {
	Revision uint64     `json:"revision"`
	Items    []TodoItem `json:"items"`
}

type TodoUpdated struct{ State TodoState }

func (TodoUpdated) eventPayload() {}

type InteractionRequested struct{ Request InteractionRequest }

func (InteractionRequested) eventPayload() {}

type InteractionResolved struct {
	ID         string
	Resolution InteractionResolution
}

func (InteractionResolved) eventPayload() {}

type CompactionStarted struct {
	ID     string
	Remove bool
}

func (CompactionStarted) eventPayload() {}

type CompactionCommitted struct{ State CompactionState }

func (CompactionCommitted) eventPayload() {}

type CompactionRemoved struct {
	ID       string
	Revision uint64
}

func (CompactionRemoved) eventPayload() {}

type SessionCleared struct{ Revision uint64 }

func (SessionCleared) eventPayload() {}

type ContextLimitReached struct {
	Scope string
	Limit int64
}

func (ContextLimitReached) eventPayload() {}

type ResultStatus string

const (
	ResultCompleted  ResultStatus = "completed"
	ResultFailed     ResultStatus = "failed"
	ResultAborted    ResultStatus = "aborted"
	ResultIncomplete ResultStatus = "incomplete"
	ResultBlocked    ResultStatus = "blocked"
)

type RunSettled struct {
	Status ResultStatus
	Reason string
}

func (RunSettled) eventPayload() {}

type Result struct {
	Status ResultStatus
	Reason string
}

type ActiveOutputSnapshot struct {
	Content           string
	Thinking          string
	ContentTruncated  bool
	ThinkingTruncated bool
	RehydrateRequired bool
}

type SessionSnapshot struct {
	Key             SessionKey
	Cursor          Cursor
	RetentionStart  Cursor
	ActiveRunID     string
	ActiveCommandID string
	// ActiveCommandFingerprint is opaque durable admission identity for hosts
	// that reconcile a product ledger with Agent state after a crash.
	ActiveCommandFingerprint string
	ActiveReceiptCursor      Cursor
	ActiveCycle              int
	RecoveryPending          bool
	RecoveryPaused           bool
	RecoveryActions          []RecoveryAction
	ActiveOutput             ActiveOutputSnapshot
	QueuedRuns               []QueuedRunSnapshot
	OpenTools                []ToolStarted
	RecentRuns               []RunSummary
	Goal                     *GoalState
	Todo                     *TodoState
	Compaction               *CompactionState
	ClearRevision            uint64
	PendingInteractions      []InteractionRequest
	MessagesTruncated        bool
}

type RunSummary struct {
	ID                 string
	CommandID          string
	CommandFingerprint string
	ReceiptCursor      Cursor
	Status             ResultStatus
	Reason             string
}

type QueuedRunSnapshot struct {
	ID            string
	CommandID     string
	ReceiptCursor Cursor
	Delivery      RecoveryInputDelivery
}

type Observation struct {
	Snapshot SessionSnapshot
	Events   <-chan Event
	Errors   <-chan error
}

var (
	ErrAgentClosed           = errors.New("agent is closed")
	ErrSessionBusy           = errors.New("agent session is busy")
	ErrSessionClosed         = errors.New("agent session is closed")
	ErrRunSettled            = errors.New("agent run is settled")
	ErrNoActiveRun           = errors.New("agent session has no active run")
	ErrDefinitionUnavailable = errors.New("agent Definition is unavailable")
	ErrDefinitionMismatch    = errors.New("agent Definition does not match durable state")
	ErrCursorExpired         = errors.New("agent event cursor expired")
	ErrCapabilityUnsupported = errors.New("agent capability is unsupported")
	ErrInteractionStale      = errors.New("agent interaction is stale")
	ErrPermissionDenied      = errors.New("agent permission denied")
	ErrContextLimit          = errors.New("agent context limit reached")
	ErrRecoveryRequired      = errors.New("agent recovery action is required")
	ErrRecoveryStale         = errors.New("agent recovery action is stale")
)

type RunError struct {
	Result Result
}

func (err *RunError) Error() string {
	if err == nil {
		return "agent run failed"
	}
	return fmt.Sprintf("agent run %s: %s", err.Result.Status, err.Result.Reason)
}
