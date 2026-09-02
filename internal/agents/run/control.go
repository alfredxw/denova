package agentrun

import (
	"encoding/json"
	"errors"

	agent "github.com/alfredxw/denova/agent"
)

// CommandID is the caller-owned idempotency identity of one Agent command.
// Callers must reuse it while command acceptance is uncertain.
type CommandID string

// OperationID identifies the durable operation selected for an accepted command.
type OperationID string

// Cursor is a durable observation position. It is display/recovery state and
// never participates in model context construction.
type Cursor uint64

// HostEffectID is the idempotency identity of one reconciled tool side effect.
type HostEffectID string

const HostEffectToolMutationCommitted = "tool_mutation_committed"

// CommandReceipt is the stable result of durable command admission.
type CommandReceipt struct {
	CommandID   CommandID
	OperationID OperationID
	Cursor      Cursor
}

// Receipt and StatusSnapshot keep the control vocabulary compact for callers.
// Both names refer to Denova-owned projections, never private runtime types.
type Receipt = CommandReceipt

// RunPhase is the closed product projection of durable Agent execution.
type RunPhase string

const (
	RunPhaseIdle       RunPhase = "idle"
	RunPhaseRunning    RunPhase = "running"
	RunPhaseCompacting RunPhase = "compacting"
)

const (
	PhaseIdle       = RunPhaseIdle
	PhaseRunning    = RunPhaseRunning
	PhaseCompacting = RunPhaseCompacting
)

// OperationStatus is the terminal state of one accepted Agent operation.
type OperationStatus string

const (
	OperationSucceeded   OperationStatus = "succeeded"
	OperationFailed      OperationStatus = "failed"
	OperationAborted     OperationStatus = "aborted"
	OperationInterrupted OperationStatus = "interrupted"
	OperationIncomplete  OperationStatus = "incomplete"
)

// DeliveryKind describes how queued input follows an active Agent operation.
type DeliveryKind string

const (
	DeliverySteer    DeliveryKind = "steer"
	DeliveryFollowUp DeliveryKind = "follow_up"
	DeliveryNextTurn DeliveryKind = "next_turn"
)

// ContextCompactionRef identifies one bounded structural context mutation.
// Descriptor is private recovery input and is never projected to HTTP.
type ContextCompactionRef struct {
	SpecRef          string
	Source           string
	Purpose          string
	Resource         string
	ExpectedRevision string
	CompactionID     string
	Force            bool
	Descriptor       json.RawMessage
}

// QueuedCommand is the bounded display projection of one accepted successor.
type QueuedCommand struct {
	CommandID        CommandID
	OperationID      OperationID
	Delivery         DeliveryKind
	Message          string
	MessageTruncated bool
	SteerRequested   bool
}

// OpenToolCall is display-only evidence for a tool that has not settled yet.
type OpenToolCall struct {
	CallID      string
	Name        string
	OperationID OperationID
	Cycle       int
}

// ActiveOutput is the bounded in-process stream projection used for reconnects.
type ActiveOutput struct {
	OperationID       OperationID
	Cycle             int
	Content           string
	Thinking          string
	ContentTruncated  bool
	ThinkingTruncated bool
	RehydrateRequired bool
}

// OperationSummary is the bounded terminal evidence retained for retries.
type OperationSummary struct {
	OperationID        OperationID
	CommandID          CommandID
	CommandFingerprint string
	ReceiptCursor      Cursor
	Status             OperationStatus
	Reason             string
	ReasonTruncated    bool
}

// DomainCommitStage names a canonical input/output write performed by the
// Agent Run.
type DomainCommitStage string

const (
	DomainCommitInput  DomainCommitStage = "input"
	DomainCommitOutput DomainCommitStage = "output"
)

// DomainCommitIdentity correlates one canonical write with an Agent cycle.
type DomainCommitIdentity struct {
	CommandID   CommandID
	OperationID OperationID
	Cycle       int
	Stage       DomainCommitStage
}

// RuntimeStatus is Denova's bounded, process-local Agent control projection.
// It deliberately omits tool arguments, raw results, prompts, and internal
// engine state so app callers cannot depend on execution details.
type RuntimeStatus struct {
	Binding                  RuntimeBinding
	Cursor                   Cursor
	Phase                    RunPhase
	ActiveCommandID          CommandID
	ActiveCommandFingerprint string
	ActiveReceiptCursor      Cursor
	ActiveOperation          OperationID
	ActiveCycle              int
	ActiveOutput             ActiveOutput
	Queue                    []QueuedCommand
	OpenToolCalls            []OpenToolCall
	LastOperation            *OperationSummary
	RecentOperations         []OperationSummary
	// Cleanup and Compaction are read-only projections of the public Agent
	// Session capabilities. Product stores may display them, but must never
	// persist competing maintenance state.
	Cleanup             *agent.CleanupState
	Compaction          *AgentCompactionState
	PendingInteractions []agent.InteractionRequest
}

// AgentCompactionState is the bounded public Session checkpoint projection.
// Product stores may render it but must not persist a competing checkpoint.
type AgentCompactionState struct {
	ID              string
	Revision        uint64
	Summary         string
	TokenEstimate   int
	ReplacementFrom int
	ReplacementTo   int
	ContextData     *RestoreData
}

type StatusSnapshot = RuntimeStatus

type InputMaterializationPlan struct {
	Required bool
	Hash     string
}

type InputMaterializationReceipt struct {
	Revision string
}

var (
	ErrInvalidCommand       = agent.ErrInvalidInput
	ErrInvalidBinding       = errors.New("invalid Denova Agent binding")
	ErrStaleOperation       = agent.ErrRunSettled
	ErrQueueConflict        = errors.New("Denova Agent queue conflict")
	ErrBusy                 = agent.ErrSessionBusy
	ErrDomainCommitRejected = agent.ErrCanonicalCommitRejected
)

// ValidateCommandID applies the exact durable command envelope without
// exposing runtime configuration to app or transport packages.
func ValidateCommandID(commandID string) error {
	return agent.ValidateIdempotencyKey(commandID)
}

// ValidateRecoveryIdentity validates the caller-owned identities required by
// an explicit recovery action.
func ValidateRecoveryIdentity(commandID, operationID string) error {
	if err := ValidateCommandID(commandID); err != nil {
		return err
	}
	return agent.ValidateIdempotencyKey(operationID)
}
