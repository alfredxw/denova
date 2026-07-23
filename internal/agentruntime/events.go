package agentruntime

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

type QueueCancelledEvent struct {
	CommandID CommandID
	Reason    string
}

func (QueueCancelledEvent) eventPayload() {}

type UserMessageCommittedEvent struct{ Message Message }

func (UserMessageCommittedEvent) eventPayload() {}

type AssistantMessageCommittedEvent struct{ Message Message }

func (AssistantMessageCommittedEvent) eventPayload() {}

type CycleStartedEvent struct {
	OperationID OperationID
	Cycle       int
	SnapshotID  SnapshotID
}

func (CycleStartedEvent) eventPayload() {}

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
	CallID string
	Name   string
	// Result is a legacy replay field. New events persist ResultDescriptor only.
	Result           string            `json:"result,omitempty"`
	ResultDescriptor PayloadDescriptor `json:"result_descriptor,omitempty"`
	IsError          bool
	RetrySafety      RetrySafety
	HostEffects      []HostEffect `json:"host_effects,omitempty"`
}

func (ToolCallFinishedEvent) eventPayload() {}

// HostEffectAcknowledgedEvent is the durable deletion marker for one host
// outbox item. Unknown acknowledgements are rejected by the reducer.
type HostEffectAcknowledgedEvent struct {
	ID HostEffectID `json:"id"`
}

func (HostEffectAcknowledgedEvent) eventPayload() {}

// HostEffectAbandonedEvent closes an outbox item when its exact cycle never
// obtained an output-domain commit receipt. The host must never observe these
// effects: without the receipt, the cycle did not cross its canonical output
// boundary and therefore has no authority to trigger downstream automation.
type HostEffectAbandonedEvent struct {
	ID     HostEffectID `json:"id"`
	Reason string       `json:"reason"`
}

func (HostEffectAbandonedEvent) eventPayload() {}

type ToolProgressEvent struct {
	CallID string
	Delta  string
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
	Delta       string
}

func (AssistantDeltaEvent) eventPayload() {}

type ThinkingDeltaEvent struct {
	OperationID OperationID
	Delta       string
}

func (ThinkingDeltaEvent) eventPayload() {}

type Observation struct {
	Snapshot StateSnapshot
	Events   <-chan Event
	Errors   <-chan error
}
