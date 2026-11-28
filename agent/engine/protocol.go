package engine

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
	// interprets it as model context. Engine uses it for the full
	// provider-neutral transcript or a canonical history locator.
	State json.RawMessage
	// Capabilities contains typed Agent capability state such as goal or todo.
	Capabilities map[string]json.RawMessage
	// InputCommit lets Engine.Run verify that admission used the same canonical
	// identity and hash.
	InputCommit *DomainCommitState
	// OutputCommit prevents a resumed cycle from generating a second final
	// output when the product transaction already committed before shutdown.
	OutputCommit *DomainCommitState
}

type ControlKind string

const (
	ControlPreempt             ControlKind = "preempt"
	ControlAbort               ControlKind = "abort"
	ControlSuspend             ControlKind = "suspend"
	ControlInteractionResolved ControlKind = "interaction_resolved"
)

type Control struct {
	Kind          ControlKind
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

// Request executes one admitted cycle. Snapshot identities and recovery state
// must come from the same Session; the caller serializes cycles for that Session.
type Request struct {
	// Journal is the lifecycle-owned commit and recovery port for this cycle.
	Journal  CanonicalHost
	Binding  BindingRef
	Snapshot TurnSnapshot
	Controls <-chan Control
}

// StructuralRequest executes a non-chat operation on the same Session as
// model turns. Controls carry Abort/Close; structural
// operations are never steerable and never append display chat messages.
type StructuralRequest struct {
	Binding      BindingRef
	Snapshot     StructuralOperationSnapshot
	State        json.RawMessage
	Capabilities map[string]json.RawMessage
	Controls     <-chan Control
}

// Event is the closed set of execution updates consumed by the lifecycle owner.
type Event interface{ engineEvent() }

// EventSource identifies one nested Agent invocation without coupling Session
// to a concrete Agent implementation. Path is ordered from root to source.
type EventSource struct {
	Name           string   `json:"name,omitempty"`
	Path           []string `json:"path,omitempty"`
	InvocationID   string   `json:"invocation_id,omitempty"`
	InvocationType string   `json:"invocation_type,omitempty"`
}

type AssistantDelta struct {
	ResponseOrdinal int
	Source          EventSource
	Delta           string
	DisplayOnly     bool
}

func (AssistantDelta) engineEvent() {}

type ThinkingDelta struct {
	ResponseOrdinal int
	Source          EventSource
	Delta           string
	DisplayOnly     bool
}

func (ThinkingDelta) engineEvent() {}

// NestedEvent carries one already-owned child Agent event through the
// parent live stream. The parent never interprets or persists the child
// payload; it only validates the envelope and assigns its own event identity.
type NestedEvent struct {
	Source       EventSource
	ParentCallID string
	SessionID    string
	ChildCursor  Cursor
	ChildRunID   string
	PayloadType  string
	Payload      json.RawMessage
}

func (NestedEvent) engineEvent() {}

// ModelUsage is provider-neutral accounting for one completed model response.
// It is a live display/trace projection and never changes the Runner transcript.
type ModelUsage struct {
	PromptTokens           int
	CachedPromptTokens     int
	CacheWritePromptTokens int
	CompletionTokens       int
	ReasoningTokens        int
	TotalTokens            int
}

type ModelCompleted struct {
	Usage          ModelUsage
	FinishReason   string
	RequestedTools []string
	Source         EventSource
}

func (ModelCompleted) engineEvent() {}

type ModelRetry struct {
	Source          EventSource
	Attempt         int
	MaxAttempts     int
	ResponseOrdinal int
	OutputState     string
	Delay           time.Duration
	Reason          string
}

func (ModelRetry) engineEvent() {}

// TranscriptUpdated checkpoints the transcript used by later execution.
// TaskCompletionIDs include completion delivery receipts at a safe boundary.
type TranscriptUpdated struct {
	State             json.RawMessage
	TaskCompletionIDs []string
	// CapabilityStates commits context-dependent capability values in the same
	// journal transaction as State, so recovery cannot mix their generations.
	CapabilityStates map[string]json.RawMessage
}

func (TranscriptUpdated) engineEvent() {}

// CapabilityState replaces or deletes one Session capability value.
// CompareCurrent makes the event a revision fence against the durable Session
// value. CheckOnly validates that fence without publishing a mutation.
type CapabilityState struct {
	Capability      string
	State           json.RawMessage
	Delete          bool
	CompareCurrent  bool
	ExpectedPresent bool
	ExpectedState   json.RawMessage
	CheckOnly       bool
}

func (CapabilityState) engineEvent() {}

// ContextNormalized reports a bounded presentation repair immediately
// before fixed context maintenance and the provider call. It is ephemeral and
// deliberately contains no message bodies.
type ContextNormalized struct {
	RepairCount    int
	MessagesBefore int
	MessagesAfter  int
}

func (ContextNormalized) engineEvent() {}

// CompactionStarted is the live edge for automatic compaction.
type CompactionStarted struct {
	ID        string
	Automatic bool
	Metrics   CompactionMetrics
}

func (CompactionStarted) engineEvent() {}

// CompactionFailed reports an automatic compaction failure.
// The primary model request continues unchanged after this event.
type CompactionFailed struct {
	ID                  string
	Reason              string
	Automatic           bool
	ConsecutiveFailures int
	FailureFuseOpen     bool
	Metrics             CompactionMetrics
}

func (CompactionFailed) engineEvent() {}

// CompactionSkipped explains a deliberate automatic preflight skip.
type CompactionSkipped struct {
	ID                  string
	Reason              string
	Automatic           bool
	ConsecutiveFailures int
	FailureFuseOpen     bool
	Metrics             CompactionMetrics
}

func (CompactionSkipped) engineEvent() {}

// GoalEvaluationFailed makes post-run evaluator failures observable
// while preserving the completed primary result and active Goal state.
type GoalEvaluationFailed struct {
	GoalID       string
	GoalRevision uint64
	Code         string
	Detail       string
}

func (GoalEvaluationFailed) engineEvent() {}

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

// InteractionRequested establishes an in-process waiter before a host
// may answer it.
type InteractionRequested struct {
	ID         string
	ToolCallID string
	Request    json.RawMessage
}

func (InteractionRequested) engineEvent() {}

type AssistantFinal struct {
	Content  string
	Thinking string
	// State replaces the opaque Engine transcript snapshot. A nil value leaves
	// the previous snapshot unchanged; JSON null explicitly clears it.
	State json.RawMessage
	// CapabilityUpdates become visible before the final assistant event.
	CapabilityUpdates []CapabilityState
	// Continuation is an Engine-authorized next cycle in the same Run.
	Continuation *Continuation
}

func (AssistantFinal) engineEvent() {}

// Continuation is bounded by the same input limits as host FollowUp.
// CommandID must be stable for the completed cycle. Autonomous distinguishes
// it from host-supplied FollowUp input.
type Continuation struct {
	CommandID  CommandID
	Input      UserInput
	Autonomous bool
}

// ToolInputStarted and ToolInputDelta are live projections of the
// model constructing a tool call. They never establish execution authority.
type ToolInputStarted struct {
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

func (ToolInputStarted) engineEvent() {}

type ToolInputDelta struct {
	CallID         string
	ProviderCallID string
	Name           string
	Delta          string
	Source         EventSource
}

func (ToolInputDelta) engineEvent() {}

type ToolStarted struct {
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

func (ToolStarted) engineEvent() {}

type ToolProgress struct {
	CallID         string
	ProviderCallID string
	Name           string
	Index          int
	Delta          string
	Metadata       json.RawMessage
	Source         EventSource
}

func (ToolProgress) engineEvent() {}

type ArtifactProduced struct {
	CallID   string
	Artifact json.RawMessage
}

func (ArtifactProduced) engineEvent() {}

type ToolFinished struct {
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

func (ToolFinished) engineEvent() {}

type Status string

const (
	Completed  Status = "completed"
	Incomplete Status = "incomplete"
	Preempted  Status = "preempted"
	Suspended  Status = "suspended"
	Aborted    Status = "aborted"
)

type Result struct {
	Status Status
	Reason string
}

// EventSink synchronously accepts an execution update. Return nil only after
// required durable state has committed; an error stops execution. The caller
// must persist opaque transcript and capability updates together when supplied.
type EventSink func(Event) error

// Runner executes Native cycles under caller-owned admission and persistence.
// Runner implements this contract; it is not a runtime-selection interface.
type Runner interface {
	Run(context.Context, Request, EventSink) (Result, error)
}

// Input materialization is a direct host boundary executed before the model.
// The receipt is passed to Runner.Run only to prove that the selected
// canonical adapter handled this exact input.
type InputMaterializationRequest struct {
	Journal  CanonicalHost
	Binding  BindingRef
	Snapshot TurnSnapshot
}

type InputMaterializationPlan struct {
	Required bool
	Hash     string
}

type InputMaterializationReceipt struct{ Revision string }

type InputMaterializer interface {
	PlanInputMaterialization(context.Context, InputMaterializationRequest) (InputMaterializationPlan, error)
	MaterializeInput(context.Context, InputMaterializationRequest, InputMaterializationPlan) (InputMaterializationReceipt, error)
}

// StructuralRunner is an optional extension for context compaction.
type StructuralRunner interface {
	RunStructural(context.Context, StructuralRequest, EventSink) (Result, error)
}

// InteractionResolveRequest lets an Runner validate and normalize a response
// and persist remembered permission rules before the Run accepts it.
type InteractionResolveRequest struct {
	Snapshot    TurnSnapshot
	Interaction InteractionSnapshot
	Response    json.RawMessage
}

type InteractionResolver interface {
	ResolveInteraction(context.Context, InteractionResolveRequest) (json.RawMessage, error)
}

// TurnAdmissionRequest lets capability managers apply Input intent before the
// first model request.
type TurnAdmissionRequest struct {
	Snapshot TurnSnapshot
}

type AdmissionPreparer interface {
	PrepareAdmission(context.Context, TurnAdmissionRequest) ([]CapabilityState, error)
}
