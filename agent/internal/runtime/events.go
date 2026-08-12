package runtime

import (
	"encoding/json"
	"time"
)

type EventDurability string

const (
	EventDurable   EventDurability = "durable"
	EventEphemeral EventDurability = "ephemeral"
)

type Event struct {
	Cursor     Cursor
	Durability EventDurability
	Payload    EventPayload
}

type EventPayload interface {
	eventPayload()
}

type CommandAcceptedEvent struct {
	CommandID   CommandID
	CommandKind string
	OperationID OperationID
	Fingerprint string
}

func (CommandAcceptedEvent) eventPayload() {}

type OperationStartedEvent struct {
	OperationID OperationID                  `json:"operation_id"`
	Phase       Phase                        `json:"phase,omitempty"`
	Structural  *StructuralOperationSnapshot `json:"structural,omitempty"`
}

func (OperationStartedEvent) eventPayload() {}

type QueueEnqueuedEvent struct{ Item QueuedInput }

func (QueueEnqueuedEvent) eventPayload() {}

type QueueConsumedEvent struct {
	CommandID CommandID
	Delivery  DeliveryKind
}

func (QueueConsumedEvent) eventPayload() {}

// QueueSteerRequestedEvent durably binds an engine preemption to one already
// accepted FollowUp without mutating that command's input or restore identity.
type QueueSteerRequestedEvent struct {
	CommandID CommandID
}

func (QueueSteerRequestedEvent) eventPayload() {}

type QueueCancelledEvent struct {
	CommandID CommandID
	Reason    string
}

func (QueueCancelledEvent) eventPayload() {}

type UserMessageCommittedEvent struct{ Message Message }

func (UserMessageCommittedEvent) eventPayload() {}

type AssistantMessageCommittedEvent struct{ Message Message }

func (AssistantMessageCommittedEvent) eventPayload() {}

// EngineStateCommittedEvent carries private, opaque continuation state. It is
// durable Engine input, not a display event and never appears in StateSnapshot.
type EngineStateCommittedEvent struct {
	State      json.RawMessage   `json:"state"`
	Descriptor PayloadDescriptor `json:"descriptor"`
}

func (EngineStateCommittedEvent) eventPayload() {}

// CapabilityStateCommittedEvent is a generic durable slot. Capability
// packages own the JSON schema; Runtime enforces identity, CAS, and bounds.
type CapabilityStateCommittedEvent struct {
	Capability  string            `json:"capability"`
	Expected    PayloadDescriptor `json:"expected"`
	State       json.RawMessage   `json:"state,omitempty"`
	Deleted     bool              `json:"deleted,omitempty"`
	OperationID OperationID       `json:"operation_id,omitempty"`
	Cycle       int               `json:"cycle,omitempty"`
}

func (CapabilityStateCommittedEvent) eventPayload() {}

type ContextNormalizedEvent struct {
	OperationID    OperationID
	Cycle          int
	RepairCount    int
	MessagesBefore int
	MessagesAfter  int
}

func (ContextNormalizedEvent) eventPayload() {}

type CompactionStartedEvent struct {
	OperationID OperationID
	Cycle       int
	ID          string
	Automatic   bool
	Metrics     CompactionMetrics
}

type CleanupStartedEvent struct {
	OperationID OperationID
	Cycle       int
	ID          string
	Reason      string
	Automatic   bool
	Transient   bool
	Metrics     CleanupMetrics
}

func (CleanupStartedEvent) eventPayload() {}

type CleanupCompletedEvent struct {
	OperationID OperationID
	Cycle       int
	ID          string
	Reason      string
	Automatic   bool
	Transient   bool
	Metrics     CleanupMetrics
}

func (CleanupCompletedEvent) eventPayload() {}

type CleanupFailedEvent struct {
	OperationID OperationID
	Cycle       int
	ID          string
	Reason      string
	Automatic   bool
	Metrics     CleanupMetrics
}

func (CleanupFailedEvent) eventPayload() {}

type CleanupSkippedEvent struct {
	OperationID OperationID
	Cycle       int
	ID          string
	Reason      string
	Automatic   bool
	Metrics     CleanupMetrics
}

func (CleanupSkippedEvent) eventPayload() {}

func (CompactionStartedEvent) eventPayload() {}

type CompactionFailedEvent struct {
	OperationID         OperationID
	Cycle               int
	ID                  string
	Reason              string
	Automatic           bool
	ConsecutiveFailures int
	FailureFuseOpen     bool
	Metrics             CompactionMetrics
}

func (CompactionFailedEvent) eventPayload() {}

type CompactionSkippedEvent struct {
	OperationID         OperationID
	Cycle               int
	ID                  string
	Reason              string
	Automatic           bool
	ConsecutiveFailures int
	FailureFuseOpen     bool
	Metrics             CompactionMetrics
}

func (CompactionSkippedEvent) eventPayload() {}

// InteractionRequestedEvent is committed before publication or response
// admission. Request is Engine-owned typed JSON and is retained for recovery.
type InteractionRequestedEvent struct {
	ID          string            `json:"id"`
	OperationID OperationID       `json:"operation_id"`
	Cycle       int               `json:"cycle"`
	ToolCallID  string            `json:"tool_call_id"`
	Request     json.RawMessage   `json:"request"`
	Descriptor  PayloadDescriptor `json:"descriptor"`
}

func (InteractionRequestedEvent) eventPayload() {}

// InteractionResolvedEvent contains the Engine-normalized response. Any
// remembered permission rule has already been committed by the Engine hook.
type InteractionResolvedEvent struct {
	ID                 string            `json:"id"`
	OperationID        OperationID       `json:"operation_id"`
	Cycle              int               `json:"cycle"`
	Response           json.RawMessage   `json:"response"`
	ResponseDescriptor PayloadDescriptor `json:"response_descriptor"`
}

func (InteractionResolvedEvent) eventPayload() {}

// InteractionRecoveryResumedEvent makes an explicit same-cycle engine resume
// visible after a response resolves a recovery-paused interaction.
type InteractionRecoveryResumedEvent struct {
	ID          string      `json:"id"`
	OperationID OperationID `json:"operation_id"`
	Cycle       int         `json:"cycle"`
}

func (InteractionRecoveryResumedEvent) eventPayload() {}

type CycleStartedEvent struct {
	OperationID OperationID
	Cycle       int
	SnapshotID  SnapshotID
	// StartedAt is captured once in the same durable transaction that starts
	// the cycle. Context sources may safely expose it without making recovery
	// depend on the process wall clock.
	StartedAt time.Time `json:"started_at,omitempty"`
	// CommandID and Delivery identify the exact input consumed for this cycle.
	// They are required because one Run can contain multiple Steer/FollowUp
	// cycles while OperationID remains stable.
	CommandID  CommandID `json:"command_id,omitempty"`
	Delivery   string    `json:"delivery,omitempty"`
	Autonomous bool      `json:"autonomous,omitempty"`
}

func (CycleStartedEvent) eventPayload() {}

func newCycleStartedEvent(
	operationID OperationID,
	cycle int,
	snapshotID SnapshotID,
	commandID CommandID,
	delivery string,
	autonomous bool,
) CycleStartedEvent {
	return CycleStartedEvent{
		OperationID: operationID, Cycle: cycle, SnapshotID: snapshotID,
		StartedAt: time.Now().UTC(), CommandID: commandID,
		Delivery: delivery, Autonomous: autonomous,
	}
}

// OperationRecoveryPausedEvent closes an unfinished execution attempt without
// settling its operation. It is emitted only when accepted work still exists:
// a queued Steer/FollowUp, or the exact structural command whose canonical CAS
// has not committed. Exact command replay is then the sole resume authority.
type OperationRecoveryPausedEvent struct {
	OperationID OperationID `json:"operation_id"`
	Cycle       int         `json:"cycle"`
	Reason      string      `json:"reason"`
}

func (OperationRecoveryPausedEvent) eventPayload() {}

// InputMaterializationRecoveryPendingEvent marks a queued cycle that has been
// durably selected but has not crossed its idempotent canonical-input outbox.
// The safe command identity remains recoverable until the matching resumed
// event is durable; no Engine may start while this marker is active.
type InputMaterializationRecoveryPendingEvent struct {
	OperationID OperationID  `json:"operation_id"`
	Cycle       int          `json:"cycle"`
	CommandID   CommandID    `json:"command_id"`
	Delivery    DeliveryKind `json:"delivery"`
	Autonomous  bool         `json:"autonomous,omitempty"`
}

func (InputMaterializationRecoveryPendingEvent) eventPayload() {}

type InputMaterializationRecoveryResumedEvent struct {
	OperationID OperationID `json:"operation_id"`
	Cycle       int         `json:"cycle"`
}

func (InputMaterializationRecoveryResumedEvent) eventPayload() {}

type ToolCallStartedEvent struct{ Call ToolCallState }

func (ToolCallStartedEvent) eventPayload() {}

type RetrySafety string

const (
	RetrySafe    RetrySafety = "safe"
	RetryUnsafe  RetrySafety = "unsafe"
	RetryUnknown RetrySafety = "unknown"
)

type ToolCallFinishedEvent struct {
	CallID           string
	Name             string
	ResultDescriptor PayloadDescriptor `json:"result_descriptor,omitempty"`
	IsError          bool
	RetrySafety      RetrySafety
	HostEffects      []HostEffect `json:"host_effects,omitempty"`
}

func (ToolCallFinishedEvent) eventPayload() {}

type ArtifactProducedEvent struct {
	OperationID OperationID     `json:"operation_id"`
	Cycle       int             `json:"cycle"`
	CallID      string          `json:"call_id"`
	Artifact    json.RawMessage `json:"artifact"`
}

func (ArtifactProducedEvent) eventPayload() {}

// HostEffectAcknowledgedEvent is the durable deletion marker for one host
// outbox item. Unknown acknowledgements are rejected by the reducer.
type HostEffectAcknowledgedEvent struct {
	ID HostEffectID `json:"id"`
}

func (HostEffectAcknowledgedEvent) eventPayload() {}

// HostEffectAbandonedEvent closes an outbox item when its exact cycle never
// obtained an output-domain commit receipt. The host must never observe these
// effects: without the receipt, the cycle did not cross its canonical output
// boundary and therefore has no authority to trigger downstream side effects.
type HostEffectAbandonedEvent struct {
	ID     HostEffectID `json:"id"`
	Reason string       `json:"reason"`
}

func (HostEffectAbandonedEvent) eventPayload() {}

type ToolProgressEvent struct {
	CallID         string
	ProviderCallID string
	Name           string
	Index          int
	Delta          string
	Metadata       json.RawMessage
	Source         EventSource
}

func (ToolProgressEvent) eventPayload() {}

// ByteBudgetExceededEvent is ephemeral typed evidence that a foreground
// provider/tool stream became incomplete. The terminal OperationSettledEvent
// durably records OperationIncomplete.
type ByteBudgetExceededEvent struct {
	OperationID OperationID
	Scope       ByteBudgetScope
	Current     int64
	Incoming    int64
	Limit       int64
}

func (ByteBudgetExceededEvent) eventPayload() {}

type AbortRequestedEvent struct {
	OperationID OperationID
	Reason      string
}

func (AbortRequestedEvent) eventPayload() {}

type SavePointCommittedEvent struct {
	OperationID OperationID
	Cycle       int
}

func (SavePointCommittedEvent) eventPayload() {}

type DomainCommitIntentAcceptedEvent struct {
	Identity DomainCommitIdentity `json:"identity"`
	Hash     string               `json:"hash"`
}

func (DomainCommitIntentAcceptedEvent) eventPayload() {}

// DomainCommitReconciliationAbandonedEvent records an authoritative
// not-found answer for a previously ambiguous canonical commit. It closes the
// authorization fence without inventing a receipt, making explicit Abort and
// Close usable at the recovery pause.
type DomainCommitReconciliationAbandonedEvent struct {
	Identity DomainCommitIdentity `json:"identity"`
	Hash     string               `json:"hash"`
	Reason   string               `json:"reason"`
}

func (DomainCommitReconciliationAbandonedEvent) eventPayload() {}

type DomainCommitReceiptEvent struct {
	Identity DomainCommitIdentity `json:"identity"`
	Hash     string               `json:"hash"`
	Revision string               `json:"revision"`
}

func (DomainCommitReceiptEvent) eventPayload() {}

type OperationStatus string

const (
	OperationSucceeded   OperationStatus = "succeeded"
	OperationFailed      OperationStatus = "failed"
	OperationAborted     OperationStatus = "aborted"
	OperationInterrupted OperationStatus = "interrupted"
	// OperationIncomplete means a byte boundary rejected part of a foreground
	// stream. No final assistant message is committed for that operation.
	OperationIncomplete OperationStatus = "incomplete"
)

type OperationSettledEvent struct {
	OperationID OperationID
	Status      OperationStatus
	Reason      string `json:"reason,omitempty"`
}

func (OperationSettledEvent) eventPayload() {}

type OperationInterruptedEvent struct {
	OperationID OperationID
	Reason      string
}

func (OperationInterruptedEvent) eventPayload() {}

type AssistantDeltaEvent struct {
	OperationID OperationID
	Source      EventSource
	Delta       string
	DisplayOnly bool
}

func (AssistantDeltaEvent) eventPayload() {}

type ThinkingDeltaEvent struct {
	OperationID OperationID
	Source      EventSource
	Delta       string
	DisplayOnly bool
}

func (ThinkingDeltaEvent) eventPayload() {}

// NestedEventEvent is an ephemeral parent projection of one child lifecycle
// event. Payload is typed and decoded by the public Agent layer; Runtime keeps
// the child vocabulary opaque.
type NestedEventEvent struct {
	OperationID     OperationID
	Source          EventSource
	SessionID       string
	ChildCursor     Cursor
	ChildDurability EventDurability
	ChildRunID      string
	PayloadType     string
	Payload         json.RawMessage
}

func (NestedEventEvent) eventPayload() {}

// ModelCompletedEvent is ephemeral per-response accounting. The provider
// transcript already carries the same usage for canonical persistence.
type ModelCompletedEvent struct {
	OperationID    OperationID
	Cycle          int
	Usage          ModelUsage
	FinishReason   string
	RequestedTools []string
	Source         EventSource
}

func (ModelCompletedEvent) eventPayload() {}

// ToolInputStartedEvent and ToolInputDeltaEvent project model-generated tool
// input before execution. They are ephemeral and never enter restart state.
type ToolInputStartedEvent struct {
	OperationID    OperationID
	Cycle          int
	CallID         string
	ProviderCallID string
	Name           string
	Index          int
	Metadata       json.RawMessage
	Source         EventSource
}

func (ToolInputStartedEvent) eventPayload() {}

type ToolInputDeltaEvent struct {
	OperationID    OperationID
	Cycle          int
	CallID         string
	ProviderCallID string
	Name           string
	Delta          string
	Source         EventSource
}

func (ToolInputDeltaEvent) eventPayload() {}

// ToolStartedEvent is the live, bounded parameter projection published only
// after the matching durable ToolCallStartedEvent commits. Arguments stay out
// of journals and restart snapshots by design.
type ToolStartedEvent struct {
	OperationID    OperationID
	Cycle          int
	CallID         string
	ProviderCallID string
	Name           string
	Index          int
	Arguments      json.RawMessage
	Metadata       json.RawMessage
	Source         EventSource
}

func (ToolStartedEvent) eventPayload() {}

// ToolOutputEvent is the live display projection paired with a durable tool
// completion. Rich result bytes stay ephemeral; the journal retains their
// descriptor and the Agent transcript retains bounded model context.
type ToolOutputEvent struct {
	OperationID    OperationID
	Cycle          int
	CallID         string
	ProviderCallID string
	Name           string
	Index          int
	Result         string
	IsError        bool
	Metadata       json.RawMessage
	Source         EventSource
	Projection     json.RawMessage
}

func (ToolOutputEvent) eventPayload() {}

type Observation struct {
	Snapshot StateSnapshot
	Events   <-chan Event
	Errors   <-chan error
}
