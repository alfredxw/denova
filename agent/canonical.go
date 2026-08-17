package agent

import (
	"context"
	"encoding/json"
)

type CommitStage string

const (
	CommitInput  CommitStage = "input"
	CommitOutput CommitStage = "output"
)

// CommitIdentity is the exact idempotency boundary shared by Agent and a
// product store. Adapters must never substitute a display or request ID.
type CommitIdentity struct {
	Session   SessionKey
	CommandID string
	RunID     string
	Cycle     int
	Stage     CommitStage
}

type InputCommitRequest struct {
	Identity CommitIdentity
	Hash     string
	Input    Input
}

type OutputCommitRequest struct {
	Identity CommitIdentity
	Hash     string
	// Message is the exact provider-neutral final output. Hosts may persist
	// continuation metadata and usage beside their product projection without
	// depending on a provider SDK type.
	Message Message
}

type CommitReceipt struct{ Revision string }

// OutputProjection optionally replaces the provider output retained in the
// Agent transcript. It supports host protocols such as structured plan cards:
// the raw output remains the canonical commit identity, while only the
// product-approved projection becomes future model context.
type OutputProjection struct {
	Content  string
	Thinking string
}

type OutputCommitReceipt struct {
	Revision   string
	Transcript *OutputProjection
}

type Effect struct {
	Kind string          `json:"kind"`
	Data json.RawMessage `json:"data"`
}

type EffectRequest struct {
	ID       string
	Identity CommitIdentity
	CallID   string
	Index    int
	Effect   Effect
}

type EffectResult struct {
	ID       string
	Revision string
	Error    string
}

// CanonicalAdapter coordinates direct idempotent writes to a host's product
// store. ApplyEffects returns one result per item.
type CanonicalAdapter interface {
	Identity() CapabilityIdentity
	MaterializeInput(context.Context, InputCommitRequest) (CommitReceipt, error)
	CommitOutput(context.Context, OutputCommitRequest) (OutputCommitReceipt, error)
	ApplyEffects(context.Context, []EffectRequest) ([]EffectResult, error)
}

// CanonicalAdapterFuncs is the compact adapter form for hosts whose product
// store already exposes idempotent commit functions.
type CanonicalAdapterFuncs struct {
	CapabilityIdentity CapabilityIdentity
	MaterializeInputFn func(context.Context, InputCommitRequest) (CommitReceipt, error)
	CommitOutputFn     func(context.Context, OutputCommitRequest) (OutputCommitReceipt, error)
	ApplyEffectsFn     func(context.Context, []EffectRequest) ([]EffectResult, error)
}

func (adapter CanonicalAdapterFuncs) Identity() CapabilityIdentity {
	return adapter.CapabilityIdentity
}

func (adapter CanonicalAdapterFuncs) MaterializeInput(ctx context.Context, request InputCommitRequest) (CommitReceipt, error) {
	if adapter.MaterializeInputFn == nil {
		return CommitReceipt{}, ErrCapabilityUnsupported
	}
	return adapter.MaterializeInputFn(ctx, request)
}

func (adapter CanonicalAdapterFuncs) CommitOutput(ctx context.Context, request OutputCommitRequest) (OutputCommitReceipt, error) {
	if adapter.CommitOutputFn == nil {
		return OutputCommitReceipt{}, ErrCapabilityUnsupported
	}
	return adapter.CommitOutputFn(ctx, request)
}

func (adapter CanonicalAdapterFuncs) ApplyEffects(ctx context.Context, requests []EffectRequest) ([]EffectResult, error) {
	if adapter.ApplyEffectsFn == nil {
		return nil, ErrCapabilityUnsupported
	}
	return adapter.ApplyEffectsFn(ctx, requests)
}
