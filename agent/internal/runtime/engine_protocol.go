package runtime

import (
	"context"
	"encoding/json"
	"time"
)

type TurnSnapshot struct {
	ID          SnapshotID
	Binding     BindingRef
	CommandID   CommandID
	OperationID OperationID
	Cycle       int
	// StartedAt is the durable cycle-start timestamp. It is stable across
	// interaction and process recovery and is never recomputed by the Engine.
	StartedAt time.Time
	// Delivery is the exact accepted input semantic for this cycle. It must not
	// be inferred from Cycle: both Steer and FollowUp continue the same Run,
	// while NextTurn starts a distinct Run at cycle one.
	Delivery DeliveryKind
	// Autonomous distinguishes an Agent-owned continuation from host FollowUp.
	Autonomous bool
	Input      UserInput
	// ContextCursor identifies the durable boundary against which the profile's
	// The Definition ContextSource assembles bounded model-visible history and state.
	ContextCursor Cursor
	// State is the latest opaque Engine checkpoint committed by a completed
	// cycle. Runtime persists and bounds it but never projects it to display or
	// interprets it as model context. Generic Agent engines use it for the full
	// provider-neutral transcript; product engines may leave it empty.
	State json.RawMessage
	// Capabilities contains bounded, typed Agent capability state (for example
	// goal or todo). Runtime owns its CAS/durability but never interprets it.
	Capabilities map[string]json.RawMessage
	// Interactions contains exact durable requests and any normalized response
	// needed to resume a suspended tool after process recovery.
	Interactions []InteractionSnapshot
	// InputCommit is the exact acknowledged canonical-input outbox receipt. It
	// lets Engine.Run prove that admission used the same Canonical identity/hash
	// without replaying the product write after Runtime already closed it.
	InputCommit *DomainCommitState
}

type EngineControlKind string

const (
	EngineControlPreempt             EngineControlKind = "preempt"
	EngineControlAbort               EngineControlKind = "abort"
	EngineControlInteractionResolved EngineControlKind = "interaction_resolved"
)

type EngineControl struct {
	Kind          EngineControlKind
	InteractionID string
	Response      json.RawMessage
}

// InteractionSnapshot is one exact durable request/optional resolution in the
// active cycle. Runtime owns lifecycle and bounds; Engine owns both schemas.
type InteractionSnapshot struct {
	ID          string
	OperationID OperationID
	Cycle       int
	ToolCallID  string
	Request     json.RawMessage
	Response    json.RawMessage
	Resolved    bool
}

type EngineRequest struct {
	Binding  BindingRef
	Snapshot TurnSnapshot
	Controls <-chan EngineControl
	// Recovery is true only when Runtime is re-entering the same durable cycle
	// after a crash/reopen or resolved-interaction pause. Snapshot.Delivery still
	// carries the original accepted input semantic.
	Recovery bool
}

// StructuralEngineRequest executes a durable, non-chat operation on the same
// exact binding lane as model turns. Controls carry Abort/Close; structural
// operations are never steerable and never append display chat messages.
type StructuralEngineRequest struct {
	Binding      BindingRef
	Snapshot     StructuralOperationSnapshot
	State        json.RawMessage
	Capabilities map[string]json.RawMessage
	Controls     <-chan EngineControl
}

type EngineEvent interface{ engineEvent() }

// EventSource identifies one nested Agent invocation without coupling Runtime
// to a concrete Agent implementation. Path is ordered from root to source.
type EventSource struct {
	Name           string   `json:"name,omitempty"`
	Path           []string `json:"path,omitempty"`
	InvocationID   string   `json:"invocation_id,omitempty"`
	InvocationType string   `json:"invocation_type,omitempty"`
}

type EngineAssistantDelta struct {
	Source      EventSource
	Delta       string
	DisplayOnly bool
}

func (EngineAssistantDelta) engineEvent() {}

type EngineThinkingDelta struct {
	Source      EventSource
	Delta       string
	DisplayOnly bool
}

func (EngineThinkingDelta) engineEvent() {}

// EngineNestedEvent carries one already-owned child Agent event through the
// parent live stream. Runtime never interprets or journals the child payload;
// it only validates the bounded envelope and assigns the outer parent event
// identity. ChildDurability records the source event's durability without
// turning it into a second durable authority in the parent Session.
type EngineNestedEvent struct {
	Source          EventSource
	SessionID       string
	ChildCursor     Cursor
	ChildDurability EventDurability
	ChildRunID      string
	PayloadType     string
	Payload         json.RawMessage
}

func (EngineNestedEvent) engineEvent() {}

// ModelUsage is provider-neutral accounting for one completed model response.
// It is a live display/trace projection and never changes the Engine transcript.
type ModelUsage struct {
	PromptTokens       int
	CachedPromptTokens int
	CompletionTokens   int
	ReasoningTokens    int
	TotalTokens        int
}

type EngineModelCompleted struct {
	Usage          ModelUsage
	FinishReason   string
	RequestedTools []string
	Source         EventSource
}

func (EngineModelCompleted) engineEvent() {}

// EngineStateCheckpoint durably replaces opaque Engine continuation state
// without committing assistant output. It is used when a cycle is preempted or
// aborted so the accepted user input remains part of the next model context.
type EngineStateCheckpoint struct{ State json.RawMessage }

func (EngineStateCheckpoint) engineEvent() {}

// EngineCapabilityState atomically replaces or deletes one opaque capability
// value at the Engine snapshot revision.
type EngineCapabilityState struct {
	Capability string
	Expected   PayloadDescriptor
	State      json.RawMessage
	Delete     bool
}

func (EngineCapabilityState) engineEvent() {}

// EngineContextNormalized reports a bounded presentation repair immediately
// before fixed context maintenance and the provider call. It is ephemeral and
// deliberately contains no message bodies.
type EngineContextNormalized struct {
	RepairCount    int
	MessagesBefore int
	MessagesAfter  int
}

func (EngineContextNormalized) engineEvent() {}

type EngineCleanupStarted struct {
	ID        string
	Reason    string
	Automatic bool
	Transient bool
	Metrics   CleanupMetrics
}

func (EngineCleanupStarted) engineEvent() {}

type EngineCleanupCompleted struct {
	ID        string
	Reason    string
	Automatic bool
	Transient bool
	Metrics   CleanupMetrics
}

func (EngineCleanupCompleted) engineEvent() {}

type EngineCleanupFailed struct {
	ID        string
	Reason    string
	Automatic bool
	Metrics   CleanupMetrics
}

func (EngineCleanupFailed) engineEvent() {}

type EngineCleanupSkipped struct {
	ID        string
	Reason    string
	Automatic bool
	Metrics   CleanupMetrics
}

func (EngineCleanupSkipped) engineEvent() {}

// CleanupMetrics is the runtime-owned, provider-neutral event vocabulary.
// Agent maps its public cleanup measurements without importing product types.
type CleanupMetrics struct {
	EstimatedTokensBefore      int
	LocalProjectedTokens       int
	ObservedPromptTokens       int
	EffectiveTokens            int
	EstimatedTokensAfter       int
	ReclaimedTokens            int
	ContextWindowTokens        int
	PressureBefore             float64
	PressureAfter              float64
	BodyPressureBefore         float64
	BodyPressureAfter          float64
	StablePrefixTokens         int
	CandidateTokens            int
	CacheViableCandidateTokens int
	SkippedBelowMinimumCount   int
	SkippedWarmSuffixCount     int
	EagerCandidateCount        int
	EagerSelectedCount         int
	SupersededCandidateCount   int
	DiscardableCandidateCount  int
	MinimumCleanupTokens       int
	ProtectedResults           int
	EarliestChanged            int
	WarmSuffixTokens           int
	PlaceholderTokens          int
	ReplacementCount           int
	EagerOnly                  bool
	PressureScope              string
	ProviderCacheState         string
	ExecutionMode              string
	RendererVersion            string
}

// EngineCompactionStarted is the live edge for an automatic model-step
// checkpoint. Explicit structural compaction already publishes its durable
// OperationStarted event.
type EngineCompactionStarted struct {
	ID        string
	Automatic bool
	Metrics   CompactionMetrics
}

func (EngineCompactionStarted) engineEvent() {}

// EngineCompactionFailed reports a recoverable automatic checkpoint failure.
// The primary model request continues unchanged after this event.
type EngineCompactionFailed struct {
	ID                  string
	Reason              string
	Automatic           bool
	ConsecutiveFailures int
	FailureFuseOpen     bool
	Metrics             CompactionMetrics
}

func (EngineCompactionFailed) engineEvent() {}

// EngineCompactionSkipped explains a deliberate automatic preflight skip,
// such as the durable consecutive-failure fuse.
type EngineCompactionSkipped struct {
	ID                  string
	Reason              string
	Automatic           bool
	ConsecutiveFailures int
	FailureFuseOpen     bool
	Metrics             CompactionMetrics
}

func (EngineCompactionSkipped) engineEvent() {}

// CompactionMetrics is runtime's provider-neutral projection of checkpoint
// pressure, post-validation health, and cache evidence. It contains no
// checkpoint body, raw tool result, or product-specific storage type.
type CompactionMetrics struct {
	EstimatedTokensBefore     int
	ObservedPromptTokens      int
	ObservedEstimateTokens    int
	EstimatedTokensAfter      int
	ProjectedTokensBefore     int
	ProjectedTokensAfter      int
	ReservedTokens            int
	ContextWindowTokens       int
	Threshold                 float64
	RecoveryBand              float64
	RecoveryTargetTokens      int
	RecoveryBandMet           bool
	Degraded                  bool
	StablePrefixTokens        int
	SourceMessageCount        int
	MessageCountBefore        int
	MessageCountAfter         int
	CacheExpectedPrefixTokens int
	CacheReadTokens           int
	CandidateFingerprint      string
	CandidateGeneration       uint64
}

// EngineInteractionRequested establishes a durable waiter before a host may
// answer it. ID must be deterministic for an exact tool execution.
type EngineInteractionRequested struct {
	ID         string
	ToolCallID string
	Request    json.RawMessage
}

func (EngineInteractionRequested) engineEvent() {}

type EngineAssistantFinal struct {
	Content  string
	Thinking string
	// State atomically replaces the opaque Engine checkpoint alongside the
	// durable assistant message. A nil value leaves the previous checkpoint
	// unchanged; JSON null is an explicit empty checkpoint.
	State json.RawMessage
	// CapabilityUpdates are committed in the same journal transaction as State
	// and the final assistant message. This is the settlement seam for staged
	// projections that must never survive a failed, aborted, or preempted run.
	CapabilityUpdates []EngineCapabilityState
	// CleanupCompleted is admitted atomically with its capability update and
	// final assistant output. It must describe that update, not a transient
	// projection used only to recover an overflowing request.
	CleanupCompleted *EngineCleanupCompleted
	// Continuation is an Engine-authorized next cycle in the same durable
	// operation. Runtime admits it atomically with the final assistant message,
	// so a crash cannot expose a completed cycle while losing autonomous work.
	Continuation *EngineContinuation
}

func (EngineAssistantFinal) engineEvent() {}

// EngineContinuation is bounded by the same command-envelope limits as host
// FollowUp input. CommandID must be deterministic for an exact completed
// cycle. Autonomous marks the queue entry as Engine-owned recovery authority;
// it cannot be supplied through the public command surface.
type EngineContinuation struct {
	CommandID  CommandID
	Input      UserInput
	Autonomous bool
}

// EngineToolInputStarted and EngineToolInputDelta are live projections of the
// model constructing a tool call. They never establish execution authority or
// enter the durable journal.
type EngineToolInputStarted struct {
	CallID         string
	ProviderCallID string
	Name           string
	Index          int
	// Metadata is bounded, Engine-owned JSON for live host projection. Runtime
	// validates it but deliberately does not interpret or persist it.
	Metadata json.RawMessage
	Source   EventSource
}

func (EngineToolInputStarted) engineEvent() {}

type EngineToolInputDelta struct {
	CallID         string
	ProviderCallID string
	Name           string
	Delta          string
	Source         EventSource
}

func (EngineToolInputDelta) engineEvent() {}

type EngineToolStarted struct {
	CallID         string
	ProviderCallID string
	Name           string
	Index          int
	Arguments      json.RawMessage
	Metadata       json.RawMessage
	Source         EventSource
	// ExecutionAuthorized distinguishes the real concrete-execution edge from
	// a zero-side-effect durable pairing record for denied/invalid preflight.
	// Runtime publishes ToolStarted only for the former.
	ExecutionAuthorized bool
}

func (EngineToolStarted) engineEvent() {}

type EngineToolProgress struct {
	CallID         string
	ProviderCallID string
	Name           string
	Index          int
	Delta          string
	Metadata       json.RawMessage
	Source         EventSource
}

func (EngineToolProgress) engineEvent() {}

type EngineArtifactProduced struct {
	CallID   string
	Artifact json.RawMessage
}

func (EngineArtifactProduced) engineEvent() {}

type EngineToolFinished struct {
	CallID         string
	ProviderCallID string
	Name           string
	Index          int
	Result         string
	IsError        bool
	RetrySafety    RetrySafety
	Metadata       json.RawMessage
	Source         EventSource
	// Projection is the bounded, effect-free ToolResult used only for live
	// product display. Runtime journals retain only Result's descriptor.
	Projection json.RawMessage
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
	// Snapshot is populated for chat input/output commits and carries the exact
	// accepted Input needed to resolve a provider-free canonical input adapter.
	Snapshot TurnSnapshot
	// State and Capabilities are the exact bounded Engine checkpoint at the
	// recovery boundary. They let dynamic Definitions restore the same Adapter
	// identity instead of guessing from a binding alone.
	State        json.RawMessage
	Capabilities map[string]json.RawMessage
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
type HostEffectReconcileRequest struct {
	Effect       HostEffect
	State        json.RawMessage
	Capabilities map[string]json.RawMessage
}

type EngineHostEffectReconciler interface {
	ReconcileHostEffect(context.Context, HostEffectReconcileRequest) error
}

// InteractionResolveRequest lets an Engine validate and normalize a response
// (and durably persist permission rules) before Runtime commits the resolution.
type InteractionResolveRequest struct {
	Snapshot    TurnSnapshot
	Interaction InteractionSnapshot
	Response    json.RawMessage
}

type EngineInteractionResolver interface {
	ResolveInteraction(context.Context, InteractionResolveRequest) (json.RawMessage, error)
}

// TurnAdmissionRequest is evaluated inside the single-writer actor before the
// first-cycle acceptance batch is committed. It lets capability managers turn
// bounded Input intent into CAS mutations atomically with Run admission.
type TurnAdmissionRequest struct {
	Snapshot TurnSnapshot
}

type EngineAdmissionPreparer interface {
	PrepareAdmission(context.Context, TurnAdmissionRequest) ([]EngineCapabilityState, error)
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
