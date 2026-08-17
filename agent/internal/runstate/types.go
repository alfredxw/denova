package runstate

import (
	"encoding/json"
	"errors"
	"maps"
)

var ErrDomainCommitRejected = errors.New("Agent domain commit rejected")

type Cursor uint64
type CommandID string
type OperationID string
type SnapshotID string

// BindingRef is the engine-facing identity of one Session.
type BindingRef struct {
	Kind   string            `json:"kind"`
	Key    string            `json:"key"`
	Labels map[string]string `json:"labels,omitempty"`
}

func (ref BindingRef) Clone() BindingRef {
	ref.Labels = maps.Clone(ref.Labels)
	return ref
}

// ContextRef identifies one bounded context fragment assembled by a Source.
type ContextRef struct {
	Source    string `json:"source"`
	Resource  string `json:"resource"`
	Selector  string `json:"selector,omitempty"`
	Revision  string `json:"revision,omitempty"`
	ByteLimit int    `json:"byte_limit"`
}

type UserInput struct {
	Text        string          `json:"text"`
	ContextRefs []ContextRef    `json:"context_refs,omitempty"`
	Envelope    json.RawMessage `json:"envelope,omitempty"`
}

type DeliveryKind string

const (
	DeliveryStart    DeliveryKind = "start"
	DeliverySteer    DeliveryKind = "steer"
	DeliveryFollowUp DeliveryKind = "follow_up"
	DeliveryNextTurn DeliveryKind = "next_turn"
)

type StructuralOperationKind string

const (
	StructuralCompactContext   StructuralOperationKind = "compact_context"
	StructuralRemoveCompaction StructuralOperationKind = "remove_compaction"
)

type ContextCompactionRef struct {
	SpecRef          string          `json:"spec_ref"`
	Source           string          `json:"source"`
	Purpose          string          `json:"purpose"`
	Resource         string          `json:"resource"`
	ExpectedRevision string          `json:"expected_revision"`
	CompactionID     string          `json:"compaction_id,omitempty"`
	Force            bool            `json:"force,omitempty"`
	Envelope         json.RawMessage `json:"envelope,omitempty"`
}

type StructuralOperationSnapshot struct {
	Binding       BindingRef
	CommandID     CommandID
	OperationID   OperationID
	Cycle         int
	Kind          StructuralOperationKind
	Ref           ContextCompactionRef
	ContextCursor Cursor
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

// DomainCommitState carries the direct canonical-input receipt into Engine.Run.
type DomainCommitState struct {
	Identity DomainCommitIdentity `json:"identity"`
	Hash     string               `json:"hash"`
	Revision string               `json:"revision,omitempty"`
}
