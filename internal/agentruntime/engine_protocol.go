package agentruntime

import (
	"context"
	"encoding/json"
)

type TurnSnapshot struct {
	ID          SnapshotID
	Binding     BindingRef
	CommandID   CommandID
	OperationID OperationID
	Cycle       int
	Input       UserInput
	// ContextCursor identifies the durable boundary against which the profile's
	// ContextProjector assembles bounded model-visible history and state.
	ContextCursor Cursor
}

type EngineControlKind string

const (
	EngineControlPreempt EngineControlKind = "preempt"
	EngineControlAbort   EngineControlKind = "abort"
)

type EngineControl struct{ Kind EngineControlKind }

type EngineRequest struct {
	Binding  BindingRef
	Snapshot TurnSnapshot
	Controls <-chan EngineControl
}

// StructuralEngineRequest executes a durable, non-chat operation on the same
// exact binding lane as model turns. Controls carry Abort/Close; structural
// operations are never steerable and never append display chat messages.
type StructuralEngineRequest struct {
	Binding  BindingRef
	Snapshot StructuralOperationSnapshot
	Controls <-chan EngineControl
}

type EngineEvent interface{ engineEvent() }

type EngineAssistantDelta struct{ Delta string }

func (EngineAssistantDelta) engineEvent() {}

type EngineThinkingDelta struct{ Delta string }

func (EngineThinkingDelta) engineEvent() {}

type EngineAssistantFinal struct {
	Content  string
	Thinking string
}

func (EngineAssistantFinal) engineEvent() {}

type EngineToolStarted struct {
	CallID    string
	Name      string
	Arguments json.RawMessage
}

func (EngineToolStarted) engineEvent() {}

type EngineToolProgress struct {
	CallID string
	Delta  string
}

func (EngineToolProgress) engineEvent() {}

type EngineToolFinished struct {
	CallID      string
	Name        string
	Result      string
	IsError     bool
	RetrySafety RetrySafety
	// HostEffects enter the durable outbox in the exact same transaction as
	// ToolCallFinished. Adapters construct stable IDs with NewToolHostEffect;
	// Harness transfers them only after the exact cycle's output-domain receipt.
	HostEffects []HostEffect
}

func (EngineToolFinished) engineEvent() {}

// EngineHostEffectAcknowledged closes one durable host-effect outbox item only
// after the adapter has idempotently reconciled it in the host domain.
type EngineHostEffectAcknowledged struct{ ID HostEffectID }

func (EngineHostEffectAcknowledged) engineEvent() {}

// EngineDomainCommitIntent requests actor-linearized authorization immediately
// before an Engine writes canonical domain state. A nil sink result is the
// authorization; after Abort/Close it returns ErrDomainCommitRejected.
type EngineDomainCommitIntent struct {
	Identity DomainCommitIdentity
	Hash     string
}

func (EngineDomainCommitIntent) engineEvent() {}

// EngineDomainCommitReceipt acknowledges the exact authorized canonical write.
// An Engine that requested an intent cannot settle successfully without it.
type EngineDomainCommitReceipt struct {
	Identity DomainCommitIdentity
	Hash     string
	Revision string
}

func (EngineDomainCommitReceipt) engineEvent() {}

type EngineStatus string

const (
	EngineCompleted EngineStatus = "completed"
	EnginePreempted EngineStatus = "preempted"
	EngineAborted   EngineStatus = "aborted"
)

type EngineResult struct{ Status EngineStatus }

type EngineEventSink func(EngineEvent) error

type Engine interface {
	Run(context.Context, EngineRequest, EngineEventSink) (EngineResult, error)
}

// StructuralEngine is an optional closed extension implemented by adapters
// that support context compaction. Harness rejects structural commands before
// effects when the selected binding engine does not implement this interface.
type StructuralEngine interface {
	RunStructural(context.Context, StructuralEngineRequest, EngineEventSink) (EngineResult, error)
}

// EnginePendingInputReleaser is an optional lifecycle seam for adapter state
// referenced by a queued UserInput. The Harness invokes it exactly when a
// durable QueueCancelled event has been reduced, before publishing that event.
// Implementations must be bounded and idempotent; the durable commit cannot be
// rolled back and therefore this cleanup seam deliberately has no error result.
type EnginePendingInputReleaser interface {
	ReleasePendingInput(context.Context, UserInput)
}

// EnginePendingInputRestorer is the optional adapter seam for process-local
// dependencies referenced by an accepted durable Steer, FollowUp, or NextTurn.
// Harness calls it before consuming every queued command, including after
// journal recovery. A non-nil error leaves that input durable and visible in
// Queue so an exact command replay or later binding open can retry it without
// duplicating acceptance. Engines whose TurnSnapshot is fully reconstructible
// from UserInput do not need to implement this interface.
type EnginePendingInputRestorer interface {
	RestorePendingInput(context.Context, QueuedInput) error
}

// EngineRecoveryContextBinder installs process-local routing values for the
// complete explicitly reattached chain. Implementations must copy only their
// own bounded values from ctx; they must not retain caller cancellation or use
// this seam as authority to perform model, tool, or canonical mutation effects.
type EngineRecoveryContextBinder interface {
	BindRecoveryContext(context.Context)
}

// EngineRecoveryContextUnbinder releases process-local routing installed by
// the matching recovery observation. Implementations must ignore a stale
// owner so closing an older observer cannot clear a newer recovery route.
type EngineRecoveryContextUnbinder interface {
	UnbindRecoveryContext(context.Context)
}

// EngineStructuralOperationRestorer pins the process-local implementation
// registered by an exact CompactIfNeeded/RemoveCompaction replay before the
// Harness starts its recovered structural goroutine. Implementations must not
// perform model, tool, or canonical mutation effects.
type EngineStructuralOperationRestorer interface {
	RestoreStructuralOperation(context.Context, StructuralOperationSnapshot) error
}

// DomainCommitReconcileRequest identifies one authorized canonical write whose
// receipt was not appended to the coordinator journal before process exit.
// Reconciliation is a pure query: implementations must require an exact match
// of both Commit.Identity and Commit.Hash and must not replay the write.
type DomainCommitReconcileRequest struct {
	Binding    BindingRef
	Commit     DomainCommitState
	Structural *StructuralOperationSnapshot
}

// DomainCommitReconcileResult reports whether the exact canonical write is
// already durable. Revision must be non-empty when Found is true.
type DomainCommitReconcileResult struct {
	Found    bool
	Revision string
}

// EngineDomainCommitReconciler is the optional read-only adapter seam used
// while opening an unfinished binding. Query errors keep the journal operation
// unfinished so a later open can retry without manufacturing a terminal state.
type EngineDomainCommitReconciler interface {
	ReconcileDomainCommit(context.Context, DomainCommitReconcileRequest) (DomainCommitReconcileResult, error)
}

// EngineHostEffectReconciler is the crash-recovery seam for durable tool host
// effects whose live acknowledgement was not journaled. Implementations must
// be idempotent by HostEffect.ID. A nil result means the host state is durable
// and authorizes the Harness to append HostEffectAcknowledgedEvent.
type EngineHostEffectReconciler interface {
	ReconcileHostEffect(context.Context, HostEffect) error
}

// EngineFactory is the adapter seam between durable harness policy and a
// binding-specific agent graph/model/tool implementation.
type EngineFactory interface {
	NewEngine(context.Context, BindingRef) (Engine, error)
}

type EngineFactoryFunc func(context.Context, BindingRef) (Engine, error)

func (f EngineFactoryFunc) NewEngine(ctx context.Context, binding BindingRef) (Engine, error) {
	return f(ctx, binding)
}
