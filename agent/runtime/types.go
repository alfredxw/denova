package runtime

import (
	"encoding/json"
	"maps"
)

type Cursor uint64

type CommandID string

type OperationID string

type SnapshotID string

// BindingRef is the stable, engine-facing identity of a harness. Kind and
// Profile are deliberately open strings: applications own their taxonomy,
// while runtime only validates that the persisted identity is bounded. Key is
// the application-local identity within Kind/Profile. Labels carry bounded
// routing metadata and participate in the durable journal identity.
type BindingRef struct {
	Kind    string            `json:"kind"`
	Profile string            `json:"profile,omitempty"`
	Key     string            `json:"key"`
	Labels  map[string]string `json:"labels,omitempty"`
}

// Clone returns an identity whose labels are not shared with the caller.
func (ref BindingRef) Clone() BindingRef {
	ref.Labels = maps.Clone(ref.Labels)
	return ref
}

// Equal reports whether two complete durable identities are identical.
func (ref BindingRef) Equal(other BindingRef) bool {
	return ref.Kind == other.Kind && ref.Profile == other.Profile && ref.Key == other.Key && maps.Equal(ref.Labels, other.Labels)
}

// Label returns one application-owned routing label.
func (ref BindingRef) Label(name string) string {
	return ref.Labels[name]
}

// BindingSelector identifies a lifecycle scope. Every non-zero field is an
// exact-match predicate; at least one field is required so a local eviction
// cannot accidentally become a Runtime-wide shutdown.
type BindingSelector struct {
	Kind    string
	Profile string
	Key     string
	Labels  map[string]string
}

func (selector BindingSelector) clone() BindingSelector {
	selector.Labels = maps.Clone(selector.Labels)
	return selector
}

func (selector BindingSelector) matches(ref BindingRef) bool {
	if selector.Kind != "" && selector.Kind != ref.Kind ||
		selector.Profile != "" && selector.Profile != ref.Profile ||
		selector.Key != "" && selector.Key != ref.Key {
		return false
	}
	for name, value := range selector.Labels {
		if ref.Labels[name] != value {
			return false
		}
	}
	return true
}

// ContextRef identifies bounded context assembled by an adapter. The harness
// records identity and limits; it never expands a reference by itself.
type ContextRef struct {
	Source    string `json:"source"`
	Resource  string `json:"resource"`
	Selector  string `json:"selector,omitempty"`
	Revision  string `json:"revision,omitempty"`
	ByteLimit int    `json:"byte_limit"`
}

type UserInput struct {
	Text        string       `json:"text"`
	ContextRefs []ContextRef `json:"context_refs,omitempty"`
	// TurnSpecRef points to bounded, typed adapter state for this cycle. The
	// runtime persists the reference, never an opaque prompt or unbounded object.
	TurnSpecRef string `json:"turn_spec_ref,omitempty"`
	// RestoreDescriptor is a bounded, versioned adapter descriptor used only to
	// rebuild process-local state after a crash. Runtime treats it as opaque and
	// never projects it into display history or model context.
	RestoreDescriptor json.RawMessage `json:"restore_descriptor,omitempty"`
}

type Command interface {
	command()
	commandID() CommandID
}

type StartTurn struct {
	ID    CommandID
	Input UserInput
}

func (StartTurn) command() {}

func (c StartTurn) commandID() CommandID { return c.ID }

type Steer struct {
	ID          CommandID
	OperationID OperationID
	Input       UserInput
}

func (Steer) command() {}

func (c Steer) commandID() CommandID { return c.ID }

type FollowUp struct {
	ID          CommandID
	OperationID OperationID
	Input       UserInput
}

func (FollowUp) command() {}

func (c FollowUp) commandID() CommandID { return c.ID }

type NextTurn struct {
	ID               CommandID
	AfterOperationID OperationID
	Input            UserInput
}

func (NextTurn) command() {}

func (c NextTurn) commandID() CommandID { return c.ID }

type Abort struct {
	ID          CommandID
	OperationID OperationID
	Reason      string
}

func (Abort) command() {}

func (c Abort) commandID() CommandID { return c.ID }

// ContextCompactionRef is the bounded durable envelope for a structural
// context operation. SpecRef resolves process-local preparation/commit code;
// Source and Purpose make every model-visible input accountable; Resource and
// ExpectedRevision bind the command to one immutable canonical snapshot.
type ContextCompactionRef struct {
	SpecRef          string `json:"spec_ref"`
	Source           string `json:"source"`
	Purpose          string `json:"purpose"`
	Resource         string `json:"resource"`
	ExpectedRevision string `json:"expected_revision"`
	CompactionID     string `json:"compaction_id,omitempty"`
	Force            bool   `json:"force,omitempty"`
	// RestoreDescriptor is the bounded, versioned host plan needed to rebuild
	// process-local structural commit code after a restart. The runtime treats
	// it as opaque identity data and never executes or projects its contents.
	RestoreDescriptor json.RawMessage `json:"restore_descriptor,omitempty"`
}

// CompactIfNeeded is the only durable command that may publish a new context
// checkpoint. Force selects manual compaction; automatic callers leave it
// false and let the bounded preparation policy decide whether work is needed.
type CompactIfNeeded struct {
	ID  CommandID            `json:"id"`
	Ref ContextCompactionRef `json:"ref"`
}

func (CompactIfNeeded) command() {}

func (c CompactIfNeeded) commandID() CommandID { return c.ID }

// RemoveCompaction soft-disables exactly the compaction named by Ref. Raw
// canonical history remains intact and becomes model-visible again.
type RemoveCompaction struct {
	ID  CommandID            `json:"id"`
	Ref ContextCompactionRef `json:"ref"`
}

func (RemoveCompaction) command() {}

func (c RemoveCompaction) commandID() CommandID { return c.ID }

type Receipt struct {
	CommandID   CommandID
	OperationID OperationID
	Cursor      Cursor
	// Replayed is true when Submit returned the receipt of an already accepted
	// command. It is process-local response metadata and is never journaled.
	Replayed bool
}

type Phase string

const (
	PhaseIdle       Phase = "idle"
	PhaseRunning    Phase = "running"
	PhaseCompacting Phase = "compacting"
)

type StructuralOperationKind string

const (
	StructuralCompactContext   StructuralOperationKind = "compact_context"
	StructuralRemoveCompaction StructuralOperationKind = "remove_compaction"
)

// StructuralOperationSnapshot is the immutable engine input reconstructed
// from the durable operation.started event. It contains references and bounds,
// never the full source transcript or generated checkpoint.
type StructuralOperationSnapshot struct {
	Binding       BindingRef
	CommandID     CommandID
	OperationID   OperationID
	Cycle         int
	Kind          StructuralOperationKind
	Ref           ContextCompactionRef
	ContextCursor Cursor
}

type MessageRole string

const (
	RoleUser      MessageRole = "user"
	RoleAssistant MessageRole = "assistant"
)

type Message struct {
	ID        string      `json:"id"`
	Role      MessageRole `json:"role"`
	Content   string      `json:"content"`
	Thinking  string      `json:"thinking,omitempty"`
	Input     UserInput   `json:"input,omitempty"`
	Operation OperationID `json:"operation"`
}

type DeliveryKind string

const (
	DeliverySteer    DeliveryKind = "steer"
	DeliveryFollowUp DeliveryKind = "follow_up"
	DeliveryNextTurn DeliveryKind = "next_turn"
)

type QueuedInput struct {
	CommandID          CommandID    `json:"command_id"`
	OperationID        OperationID  `json:"operation_id"`
	Delivery           DeliveryKind `json:"delivery"`
	Input              UserInput    `json:"input"`
	InputTextTruncated bool         `json:"-"`
}

// InputMaterializationRecovery is the safe identity of a selected queued
// cycle whose canonical input outbox has not completed. UserInput and its
// private restore descriptor stay actor-owned.
type InputMaterializationRecovery struct {
	CommandID   CommandID
	OperationID OperationID
	Cycle       int
	Delivery    DeliveryKind
}

type ToolCallState struct {
	CallID string `json:"call_id"`
	Name   string `json:"name"`
	// Arguments is accepted only for replaying journals written before payload
	// descriptors were introduced. New events clear it before persistence.
	Arguments           json.RawMessage   `json:"arguments,omitempty"`
	ArgumentsDescriptor PayloadDescriptor `json:"arguments_descriptor,omitempty"`
	OperationID         OperationID       `json:"operation_id"`
	Cycle               int               `json:"cycle"`
}

type HostEffectID string
type HostEffectKind string

const HostEffectToolMutationCommitted HostEffectKind = "tool_mutation_committed"

// HostEffect is a stable durable outbox item produced by a completed tool.
// Payload is retained only until the host acknowledges idempotent reconciliation;
// display projections expose PayloadDescriptor instead of the raw envelope.
type HostEffect struct {
	ID                HostEffectID      `json:"id"`
	Kind              HostEffectKind    `json:"kind"`
	OperationID       OperationID       `json:"operation_id"`
	Cycle             int               `json:"cycle"`
	CallID            string            `json:"call_id"`
	Index             int               `json:"index"`
	Payload           json.RawMessage   `json:"payload,omitempty"`
	PayloadDescriptor PayloadDescriptor `json:"payload_descriptor"`
}

// HostEffectSnapshot is safe display/recovery evidence for one unacknowledged
// host effect. The private durable payload is delivered only to the reconciler.
type HostEffectSnapshot struct {
	ID                HostEffectID
	Kind              HostEffectKind
	OperationID       OperationID
	Cycle             int
	CallID            string
	Index             int
	PayloadDescriptor PayloadDescriptor
}

// PayloadDescriptor is the durable evidence for a potentially sensitive or
// very large tool payload. It proves identity and size without retaining raw
// arguments/results in the runtime control journal.
type PayloadDescriptor struct {
	Bytes  int    `json:"bytes"`
	SHA256 string `json:"sha256"`
}

type StateSnapshot struct {
	Binding          BindingRef
	Cursor           Cursor
	Phase            Phase
	ActiveOperation  OperationID
	ActiveCycle      int
	RecoveryPaused   bool
	InputRecovery    *InputMaterializationRecovery
	ActiveStructural *StructuralOperationSnapshot
	ActiveOutput     ActiveOutputSnapshot
	// Messages reconstructs the durable display timeline. It is deliberately
	// absent from TurnSnapshot so UI recovery never becomes implicit model input.
	Messages           []Message
	Queue              []QueuedInput
	OpenToolCalls      []ToolCallState
	PendingHostEffects []HostEffectSnapshot
	LastOperation      *OperationSummary
	// RecentOperations is a bounded terminal index used to answer an exact
	// command replay even after a newer operation has settled. It is display and
	// recovery state only; it never enters TurnSnapshot/model context.
	RecentOperations    []OperationSummary
	LastDomainCommit    *DomainCommitState
	DomainCommits       []DomainCommitState
	TimelineStartCursor Cursor
	MessagesTruncated   bool
	Memory              BindingMemorySnapshot
}

// StatusSnapshot is the bounded, display-only runtime projection. Unlike
// StateSnapshot it never materializes the durable message timeline.
type StatusSnapshot struct {
	Binding         BindingRef
	Cursor          Cursor
	Phase           Phase
	ActiveCommandID CommandID
	// ActiveCommandFingerprint and ActiveReceiptCursor identify the exact
	// durable acceptance event; Cursor is the later projection position.
	ActiveCommandFingerprint string
	ActiveReceiptCursor      Cursor
	ActiveOperation          OperationID
	ActiveCycle              int
	RecoveryPaused           bool
	// RecoveryPending is set only by a conservative, actor-free journal
	// projection when durable state still needs canonical reconciliation. It
	// lets callers decide whether opening a recovery actor is necessary without
	// parsing the human-readable LastOperation reason.
	RecoveryPending    bool
	InputRecovery      *InputMaterializationRecovery
	ActiveStructural   *StructuralOperationSnapshot
	ActiveOutput       ActiveOutputSnapshot
	Queue              []QueuedInput
	OpenToolCalls      []ToolCallState
	PendingHostEffects []HostEffectSnapshot
	LastOperation      *OperationSummary
	RecentOperations   []OperationSummary
	LastDomainCommit   *DomainCommitState
	DomainCommits      []DomainCommitState
	Memory             BindingMemorySnapshot
}

// BindingMemorySnapshot exposes bounded actor-owned payload usage without
// exposing private restore descriptors. It supports diagnostics and exact
// boundary tests; values are conservative logical payload accounting.
type BindingMemorySnapshot struct {
	RetainedBytes          int64
	PendingInputBytes      int64
	ActiveOutputBytes      int64
	PendingHostEffectBytes int64
	PendingHostEffects     int
	Limits                 BindingMemoryLimits
}

type OperationSummary struct {
	OperationID        OperationID     `json:"operation_id"`
	CommandID          CommandID       `json:"command_id"`
	CommandFingerprint string          `json:"command_fingerprint,omitempty"`
	ReceiptCursor      Cursor          `json:"receipt_cursor,omitempty"`
	Status             OperationStatus `json:"status"`
	Reason             string          `json:"reason,omitempty"`
	ReasonTruncated    bool            `json:"-"`
}

type DomainCommitStage string

const (
	DomainCommitInput  DomainCommitStage = "input"
	DomainCommitOutput DomainCommitStage = "output"
)

type DomainCommitIdentity struct {
	CommandID   CommandID         `json:"command_id"`
	OperationID OperationID       `json:"operation_id"`
	Cycle       int               `json:"cycle"`
	Stage       DomainCommitStage `json:"stage"`
}

// DomainCommitState is the bounded recovery receipt for a canonical application
// write performed outside the control journal. An empty Revision means
// authorization was durable but the canonical commit was not acknowledged.
type DomainCommitState struct {
	Identity  DomainCommitIdentity `json:"identity"`
	Hash      string               `json:"hash"`
	Revision  string               `json:"revision,omitempty"`
	Abandoned bool                 `json:"abandoned,omitempty"`
	Reason    string               `json:"reason,omitempty"`
}

// ActiveOutputSnapshot is an in-process recovery projection for stream
// reconnects. It is display-only, is never placed in TurnSnapshot, and is
// replaced by the final durable assistant message when the cycle completes.
type ActiveOutputSnapshot struct {
	OperationID       OperationID
	Cycle             int
	Content           string
	Thinking          string
	ContentTruncated  bool
	ThinkingTruncated bool
	// RehydrateRequired marks a retained prefix restored from an unfinished
	// stream checkpoint. It must never be presented as a final assistant reply.
	RehydrateRequired bool
}
