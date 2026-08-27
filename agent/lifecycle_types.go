package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

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
	Attachments    []Attachment
	IdempotencyKey string
	Context        []ContextFragment
	Goal           *GoalMutation
	HostData       *HostData
}

func Text(value string) Input { return Input{Text: value} }

// CommandReceipt identifies a command accepted by the current process.
type CommandReceipt struct {
	CommandID string
	RunID     string
	Cursor    Cursor
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

type Event struct {
	Cursor  Cursor
	RunID   string
	Payload EventPayload
}

type EventPayload interface{ eventPayload() }

type RunAccepted struct{ CommandID string }

func (RunAccepted) eventPayload() {}

type RunStarted struct {
	Cycle     int
	CommandID string
	Delivery  string
	// StartedAt is the immutable activation time for the whole Run. Later
	// cycles keep the same value so product UIs can display one stable elapsed
	// time across steering and queued continuation cycles.
	StartedAt time.Time
}

func (RunStarted) eventPayload() {}

// EventSource identifies the Agent invocation that produced a live event.
// Path is ordered from the root Agent to Source.Name.
type EventSource struct {
	Name           string
	Path           []string
	InvocationID   string
	InvocationType string
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

// ContextNormalized is bounded telemetry for provider-protocol repairs made
// immediately before fixed context maintenance. Message content is never
// exposed through this event.
type ContextNormalized struct {
	RepairCount    int
	MessagesBefore int
	MessagesAfter  int
}

func (ContextNormalized) eventPayload() {}

type AssistantFinal struct {
	Content  string
	Thinking string
}

func (AssistantFinal) eventPayload() {}

// ToolInputStarted marks the point at which the model has identified a tool
// call. It precedes ToolStarted and does not authorize tool execution.
type ToolInputStarted struct {
	CallID         string
	ProviderCallID string
	ParentCallID   string
	Name           string
	Index          int
	Descriptor     *ToolDescriptor
	Source         EventSource
}

func (ToolInputStarted) eventPayload() {}

// ToolInputDelta carries one append-only fragment of model-generated tool
// arguments while the assistant response is still streaming.
type ToolInputDelta struct {
	CallID         string
	ProviderCallID string
	Name           string
	Delta          string
	Source         EventSource
}

func (ToolInputDelta) eventPayload() {}

// ToolStarted is emitted only after the complete model response, when Agent
// establishes the paired tool lifecycle. Concrete execution normally follows;
// a synthetic preflight failure instead emits ToolFinished immediately.
type ToolStarted struct {
	CallID         string
	ProviderCallID string
	Name           string
	Index          int
	Arguments      json.RawMessage
	Descriptor     *ToolDescriptor
	Source         EventSource
}

func (ToolStarted) eventPayload() {}

type ToolProgress struct {
	CallID         string
	ProviderCallID string
	Name           string
	Index          int
	Delta          string
	Descriptor     *ToolDescriptor
	Source         EventSource
}

func (ToolProgress) eventPayload() {}

type ToolFinished struct {
	CallID         string
	ProviderCallID string
	Name           string
	Index          int
	IsError        bool
	Result         string
	Descriptor     *ToolDescriptor
	Projection     *ToolResult
	Source         EventSource
}

func (ToolFinished) eventPayload() {}

type ArtifactProduced struct {
	CallID   string
	Artifact ToolArtifactRef
	Source   EventSource
}

func (ArtifactProduced) eventPayload() {}

// EventStreamGap reports that a bounded live observer discarded older display
// events to stay detached from authoritative execution. Callers can rehydrate
// from Session.Snapshot and reconnect with Session.Observe after ResumeAfter.
// A gap never changes Run settlement.
type EventStreamGap struct {
	Dropped     int
	ResumeAfter Cursor
}

func (EventStreamGap) eventPayload() {}

// InputDelivery identifies how accepted input relates to the active Run.
type InputDelivery string

const (
	DeliverySteer    InputDelivery = "steer"
	DeliveryFollowUp InputDelivery = "follow_up"
	DeliveryNextTurn InputDelivery = "next_turn"
)

type GoalUpdated struct {
	State   GoalState
	Present bool
}

func (GoalUpdated) eventPayload() {}

const GoalEvaluationFailedCode = "agent_runtime.goal_evaluation_failed"

// GoalEvaluationFailed reports a bounded post-run evaluator failure without
// changing the active Goal or failing the successfully completed primary turn.
type GoalEvaluationFailed struct {
	GoalID       string
	GoalRevision uint64
	Code         string
	Detail       string
}

func (GoalEvaluationFailed) eventPayload() {}

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
	ID        string
	Remove    bool
	Automatic bool
	Metrics   CompactionMetrics
}

func (CompactionStarted) eventPayload() {}

type CompactionCommitted struct {
	State     CompactionState
	Automatic bool
}

func (CompactionCommitted) eventPayload() {}

type CompactionRemoved struct {
	ID       string
	Revision uint64
}

func (CompactionRemoved) eventPayload() {}

type CompactionFailed struct {
	ID                  string
	Reason              string
	Automatic           bool
	ConsecutiveFailures int
	FailureFuseOpen     bool
	Metrics             CompactionMetrics
}

func (CompactionFailed) eventPayload() {}

type CompactionSkipped struct {
	ID                  string
	Reason              string
	Automatic           bool
	ConsecutiveFailures int
	FailureFuseOpen     bool
	Metrics             CompactionMetrics
}

func (CompactionSkipped) eventPayload() {}

type CleanupStarted struct {
	ID        string
	Reason    string
	Automatic bool
	Transient bool
	Metrics   CleanupMetrics
}

func (CleanupStarted) eventPayload() {}

type CleanupCompleted struct {
	ID        string
	Reason    string
	Automatic bool
	Transient bool
	Metrics   CleanupMetrics
}

func (CleanupCompleted) eventPayload() {}

type CleanupFailed struct {
	ID        string
	Reason    string
	Automatic bool
	Metrics   CleanupMetrics
}

func (CleanupFailed) eventPayload() {}

type CleanupSkipped struct {
	ID        string
	Reason    string
	Automatic bool
	Metrics   CleanupMetrics
}

func (CleanupSkipped) eventPayload() {}

type CleanupCommitted struct {
	State     CleanupState
	Automatic bool
}

func (CleanupCommitted) eventPayload() {}

type SessionCleared struct{ Revision uint64 }

func (SessionCleared) eventPayload() {}

type TranscriptSynchronized struct{ State TranscriptSyncState }

func (TranscriptSynchronized) eventPayload() {}

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
	Key                 SessionKey
	Cursor              Cursor
	RetentionStart      Cursor
	ActiveRunID         string
	ActiveCommandID     string
	ActiveReceiptCursor Cursor
	ActiveCycle         int
	ActiveOutput        ActiveOutputSnapshot
	QueuedRuns          []QueuedRunSnapshot
	OpenTools           []OpenToolSnapshot
	RecentRuns          []RunSummary
	Goal                *GoalState
	Todo                *TodoState
	Cleanup             *CleanupState
	Compaction          *CompactionState
	TranscriptSync      *TranscriptSyncState
	ClearRevision       uint64
	PendingInteractions []InteractionRequest
	MessagesTruncated   bool
}

type RunSummary struct {
	ID              string
	CommandID       string
	ReceiptCursor   Cursor
	Status          ResultStatus
	Reason          string
	ReasonTruncated bool
	// Output is the settled assistant text retained with the transcript. Live
	// event history remains process-local and is intentionally not replayed.
	Output string
}

type QueuedRunSnapshot struct {
	ID            string
	CommandID     string
	ReceiptCursor Cursor
	Delivery      InputDelivery
	Text          string
	TextTruncated bool
	// InterruptRequested means this accepted FollowUp has already been promoted
	// to the active Run's next safe preemption boundary.
	InterruptRequested bool
}

// OpenToolSnapshot describes an unfinished tool in the live process. It
// deliberately excludes arguments and results; those remain in live Events.
type OpenToolSnapshot struct {
	CallID string
	Name   string
	RunID  string
	Cycle  int
	Source EventSource
}

type Observation struct {
	Snapshot SessionSnapshot
	Events   <-chan Event
	Errors   <-chan error
}

var (
	ErrAgentClosed                = errors.New("agent is closed")
	ErrInvalidInput               = errors.New("agent input is invalid")
	ErrSessionBusy                = errors.New("agent session is busy")
	ErrSessionClosed              = errors.New("agent session is closed")
	ErrRunSettled                 = errors.New("agent run is settled")
	ErrNoActiveRun                = errors.New("agent session has no active run")
	ErrDefinitionUnavailable      = errors.New("agent Definition is unavailable")
	ErrDefinitionMismatch         = errors.New("agent Definition does not match the active transcript")
	ErrCursorExpired              = errors.New("agent event cursor expired")
	ErrCapabilityUnsupported      = errors.New("agent capability is unsupported")
	ErrInteractionStale           = errors.New("agent interaction is stale")
	ErrPermissionDenied           = errors.New("agent permission denied")
	ErrPermissionArgumentsChanged = errors.New("agent tool arguments changed after authorization")
	ErrContextLimit               = errors.New("agent context limit reached")
	ErrIdleTimeout                = errors.New("agent execution idle timeout")
	ErrCanonicalCommitRejected    = errors.New("agent canonical commit was rejected")
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
