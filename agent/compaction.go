package agent

import (
	"context"
	"time"
)

const compactionCapability = "agent.compaction"

type CompactionAction string

const (
	CompactionNone   CompactionAction = "none"
	CompactionCreate CompactionAction = "compact"
)

type CompactionRequest struct {
	Force            bool
	IdempotencyKey   string
	ExpectedID       string
	ExpectedRevision uint64
}

type CompactionRemoveRequest struct {
	ID               string
	ExpectedRevision uint64
	IdempotencyKey   string
}

type CompactionState struct {
	ID              string    `json:"id"`
	Revision        uint64    `json:"revision"`
	SourceRevision  string    `json:"source_revision"`
	SourceHash      string    `json:"source_hash"`
	Summary         string    `json:"summary"`
	TokenEstimate   int       `json:"token_estimate,omitempty"`
	ReplacementFrom int       `json:"replacement_from"`
	ReplacementTo   int       `json:"replacement_to"`
	CreatedAt       time.Time `json:"created_at"`
	Removed         bool      `json:"removed,omitempty"`
	// ContextData is optional, product-neutral metadata used by a custom
	// ContextSource to apply this checkpoint to host-owned context. Agent keeps
	// it durable and opaque; it is never injected into the model automatically.
	ContextData *HostData `json:"context_data,omitempty"`
}

type CompactionPlanRequest struct {
	Session      SessionView
	Run          RunView
	Messages     []*Message
	ModelRequest []*Message
	Force        bool
	Current      CompactionState
	Present      bool
}

type CompactionPlan struct {
	Action         CompactionAction
	SourceFrom     int
	SourceTo       int
	SourceRevision string
	SourceHash     string
}

type CompactionCompactRequest struct {
	Session      SessionView
	Run          RunView
	Messages     []*Message
	ModelRequest []*Message
	Plan         CompactionPlan
	Current      CompactionState
	Present      bool
}

type CompactionCheckpoint struct {
	Summary       string
	TokenEstimate int
	ContextData   *HostData
}

// CompactionManager plans and generates checkpoints. Agent owns durable CAS,
// raw-history retention, effective markers, recovery, and Event publication.
type CompactionManager interface {
	Identity() CapabilityIdentity
	Plan(context.Context, CompactionPlanRequest) (CompactionPlan, error)
	Compact(context.Context, CompactionCompactRequest) (CompactionCheckpoint, error)
}

type CompactionResult struct {
	Changed bool
	State   CompactionState
}
