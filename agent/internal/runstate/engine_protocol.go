package runstate

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
	// StartedAt is stable for the cycle and is not recomputed during an
	// interaction.
	StartedAt time.Time
	// Delivery is the exact accepted input semantic for this cycle. It must not
	// be inferred from Cycle: both Steer and FollowUp continue the same Run,
	// while NextTurn starts a distinct Run at cycle one.
	Delivery DeliveryKind
	// Autonomous distinguishes an Agent-owned continuation from host FollowUp.
	Autonomous bool
	Input      UserInput
	// ContextCursor identifies the Session transcript revision used to assemble
	// bounded model-visible history and state.
	ContextCursor Cursor
	// State is the latest opaque Engine transcript snapshot. Session persists
	// and bounds it but never projects it to display or
	// interprets it as model context. Generic Agent engines use it for the full
	// provider-neutral transcript; product engines may leave it empty.
	State json.RawMessage
	// Capabilities contains typed Agent capability state such as goal or todo.
	Capabilities map[string]json.RawMessage
	// InputCommit lets Engine.Run verify that admission used the same canonical
	// identity and hash.
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

// InteractionSnapshot identifies one request waiting in the current Run.
type InteractionSnapshot struct {
	ID          string
	OperationID OperationID
	Cycle       int
	ToolCallID  string
	Request     json.RawMessage
}

type EngineRequest struct {
	Binding  BindingRef
	Snapshot TurnSnapshot
	Controls <-chan EngineControl
}

// StructuralEngineRequest executes a non-chat operation on the same Session as
// model turns. Controls carry Abort/Close; structural
// operations are never steerable and never append display chat messages.
type StructuralEngineRequest struct {
	Binding      BindingRef
	Snapshot     StructuralOperationSnapshot
	State        json.RawMessage
	Capabilities map[string]json.RawMessage
	Controls     <-chan EngineControl
}

type EngineEvent interface{ engineEvent() }

// EventSource identifies one nested Agent invocation without coupling Session
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
// parent live stream. The parent never interprets or persists the child
// payload; it only validates the envelope and assigns its own event identity.
type EngineNestedEvent struct {
	Source       EventSource
	ParentCallID string
	SessionID    string
	ChildCursor  Cursor
	ChildRunID   string
	PayloadType  string
	Payload      json.RawMessage
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

// EngineTranscriptUpdated replaces the in-process transcript used by later
// cycles in this Run. Session persists only a final or deliberately aborted
// transcript, never an unfinished model/tool boundary.
type EngineTranscriptUpdated struct{ State json.RawMessage }

func (EngineTranscriptUpdated) engineEvent() {}

// EngineCapabilityState replaces or deletes one Session capability value.
// CompareCurrent makes the event a revision fence against the durable Session
// value. CheckOnly validates that fence without publishing a mutation.
type EngineCapabilityState struct {
	Capability      string
	State           json.RawMessage
	Delete          bool
	CompareCurrent  bool
	ExpectedPresent bool
	ExpectedState   json.RawMessage
	CheckOnly       bool
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

// CleanupMetrics is the Agent-owned, provider-neutral event vocabulary.
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

// EngineCompactionStarted is the live edge for automatic compaction.
type EngineCompactionStarted struct {
	ID        string
	Automatic bool
	Metrics   CompactionMetrics
}

func (EngineCompactionStarted) engineEvent() {}

// EngineCompactionFailed reports an automatic compaction failure.
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

// EngineCompactionSkipped explains a deliberate automatic preflight skip.
type EngineCompactionSkipped struct {
	ID                  string
	Reason              string
	Automatic           bool
	ConsecutiveFailures int
	FailureFuseOpen     bool
	Metrics             CompactionMetrics
}

func (EngineCompactionSkipped) engineEvent() {}

// EngineGoalEvaluationFailed makes post-run evaluator failures observable
// while preserving the completed primary result and active Goal state.
type EngineGoalEvaluationFailed struct {
	GoalID       string
	GoalRevision uint64
	Code         string
	Detail       string
}

func (EngineGoalEvaluationFailed) engineEvent() {}

// CompactionMetrics is a provider-neutral projection of context pressure,
// post-validation health, and cache evidence.
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

// EngineInteractionRequested establishes an in-process waiter before a host
// may answer it.
type EngineInteractionRequested struct {
	ID         string
	ToolCallID string
	Request    json.RawMessage
}

func (EngineInteractionRequested) engineEvent() {}

type EngineAssistantFinal struct {
	Content  string
	Thinking string
	// State replaces the opaque Engine transcript snapshot. A nil value leaves
	// the previous snapshot unchanged; JSON null explicitly clears it.
	State json.RawMessage
	// CapabilityUpdates become visible before the final assistant event.
	CapabilityUpdates []EngineCapabilityState
	// CleanupCompleted describes the cleanup capability update associated with
	// the final assistant output.
	CleanupCompleted *EngineCleanupCompleted
	// Continuation is an Engine-authorized next cycle in the same Run.
	Continuation *EngineContinuation
}

func (EngineAssistantFinal) engineEvent() {}

// EngineContinuation is bounded by the same input limits as host FollowUp.
// CommandID must be stable for the completed cycle. Autonomous distinguishes
// it from host-supplied FollowUp input.
type EngineContinuation struct {
	CommandID  CommandID
	Input      UserInput
	Autonomous bool
}

// EngineToolInputStarted and EngineToolInputDelta are live projections of the
// model constructing a tool call. They never establish execution authority.
type EngineToolInputStarted struct {
	CallID         string
	ProviderCallID string
	ParentCallID   string
	Name           string
	Index          int
	// Metadata is bounded, Engine-owned JSON for live host projection. Agent
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
	// ExecutionAuthorized distinguishes real execution from denied or invalid
	// preflight. Run publishes ToolStarted only for real execution.
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
	Metadata       json.RawMessage
	Source         EventSource
	// Projection is the bounded, effect-free ToolResult used only for live
	// product display.
	Projection json.RawMessage
}

func (EngineToolFinished) engineEvent() {}

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

// Input materialization is a direct host boundary executed before the model.
// The receipt is passed to Engine.Run only to prove that the selected
// canonical adapter handled this exact input.
type InputMaterializationRequest struct {
	Binding  BindingRef
	Snapshot TurnSnapshot
}

type InputMaterializationPlan struct {
	Required bool
	Hash     string
}

type InputMaterializationReceipt struct{ Revision string }

type EngineInputMaterializer interface {
	PlanInputMaterialization(context.Context, InputMaterializationRequest) (InputMaterializationPlan, error)
	MaterializeInput(context.Context, InputMaterializationRequest, InputMaterializationPlan) (InputMaterializationReceipt, error)
}

// StructuralEngine is an optional extension for context compaction.
type StructuralEngine interface {
	RunStructural(context.Context, StructuralEngineRequest, EngineEventSink) (EngineResult, error)
}

// InteractionResolveRequest lets an Engine validate and normalize a response
// and persist remembered permission rules before the Run accepts it.
type InteractionResolveRequest struct {
	Snapshot    TurnSnapshot
	Interaction InteractionSnapshot
	Response    json.RawMessage
}

type EngineInteractionResolver interface {
	ResolveInteraction(context.Context, InteractionResolveRequest) (json.RawMessage, error)
}

// TurnAdmissionRequest lets capability managers apply Input intent before the
// first model request.
type TurnAdmissionRequest struct {
	Snapshot TurnSnapshot
}

type EngineAdmissionPreparer interface {
	PrepareAdmission(context.Context, TurnAdmissionRequest) ([]EngineCapabilityState, error)
}

// EngineFactory builds the engine for one Session binding.
type EngineFactory interface {
	NewEngine(context.Context, BindingRef) (Engine, error)
}

type EngineFactoryFunc func(context.Context, BindingRef) (Engine, error)

func (f EngineFactoryFunc) NewEngine(ctx context.Context, binding BindingRef) (Engine, error) {
	return f(ctx, binding)
}
