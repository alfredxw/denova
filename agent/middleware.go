package agent

import (
	"context"
	"errors"
	"time"
)

// ToolCallEndpoint is the single middleware seam for structured tool calls.
type ToolCallEndpoint func(context.Context, string, ...ToolOption) (ToolResult, error)

// ToolContext identifies one concrete tool call.
type ToolContext struct {
	Index          int
	Name           string
	ExecutionID    string
	ProviderCallID string
	Definition     ToolDefinitionSnapshot
}

// ModelContext contains read-only metadata for a model invocation.
type ModelContext struct {
	Tools []*ToolInfo
	Retry *RetryConfig
	// Iteration is the zero-based model step within the current Agent run.
	Iteration int
}

// ModelCall is the final provider-neutral request immediately before the
// configured model adapter is invoked. At this seam every transcript rewrite,
// tool-schema decision, and per-call option must already be present.
//
// Middleware may replace Messages or Options, but it must preserve Model unless
// it intentionally installs another adapter with equivalent provider semantics.
type ModelCall struct {
	Model     BaseChatModel
	Messages  []*Message
	Options   []ModelOption
	Streaming bool
}

// ModelRequestSnapshot is an immutable side-fork handle over one final model
// call. It deliberately keeps the concrete model adapter opaque: executing a
// fork therefore reuses the same provider, model, endpoint, thinking settings,
// cache routing, and provider compatibility wrappers as the primary call.
//
// The current adapter contract does not expose a provider-private serialized
// request object. Snapshot guarantees exact reuse of the final provider-neutral
// inputs; adapters must assemble those inputs deterministically.
type ModelRequestSnapshot struct {
	model     BaseChatModel
	messages  []*Message
	options   []ModelOption
	streaming bool
}

// Snapshot freezes a detached copy of this final model call.
func (call *ModelCall) Snapshot() *ModelRequestSnapshot {
	if call == nil {
		return nil
	}
	return &ModelRequestSnapshot{
		model: call.Model, messages: cloneMessages(call.Messages),
		options: append([]ModelOption(nil), call.Options...), streaming: call.Streaming,
	}
}

// Messages returns a detached copy of the snapshot's model-visible messages.
func (snapshot *ModelRequestSnapshot) Messages() []*Message {
	if snapshot == nil {
		return nil
	}
	return cloneMessages(snapshot.messages)
}

// ResolvedOptions returns a defensive, provider-neutral view of the exact
// call options captured by this snapshot.
func (snapshot *ModelRequestSnapshot) ResolvedOptions() *Options {
	if snapshot == nil {
		return &Options{}
	}
	return GetCommonOptions(&Options{}, snapshot.options...)
}

// Append returns a detached fork whose only request-input change is an ordered
// message suffix. The primary snapshot is never mutated.
func (snapshot *ModelRequestSnapshot) Append(messages ...*Message) *ModelRequestSnapshot {
	if snapshot == nil {
		return nil
	}
	appended := cloneMessages(snapshot.messages)
	appended = append(appended, cloneMessages(messages)...)
	return &ModelRequestSnapshot{
		model:    snapshot.model,
		messages: appended,
		options:  append([]ModelOption(nil), snapshot.options...), streaming: snapshot.streaming,
	}
}

// Generate executes exactly one non-streaming model request from the snapshot.
func (snapshot *ModelRequestSnapshot) Generate(ctx context.Context) (*Message, error) {
	if snapshot == nil || snapshot.model == nil {
		return nil, errors.New("model request snapshot is unavailable")
	}
	return snapshot.model.Generate(ctx, cloneMessages(snapshot.messages), snapshot.options...)
}

// Stream executes exactly one streaming model request from the snapshot.
func (snapshot *ModelRequestSnapshot) Stream(ctx context.Context) (*StreamReader[*Message], error) {
	if snapshot == nil || snapshot.model == nil {
		return nil, errors.New("model request snapshot is unavailable")
	}
	return snapshot.model.Stream(ctx, cloneMessages(snapshot.messages), snapshot.options...)
}

// Streaming reports the primary call mode captured by the snapshot.
func (snapshot *ModelRequestSnapshot) Streaming() bool {
	return snapshot != nil && snapshot.streaming
}

// RunContext is mutable once at the beginning of a run.
type RunContext struct {
	Instruction string
	Tools       []ToolDefinition
}

// RunState is the in-memory transcript for one run.
type RunState struct {
	Messages  []*Message
	ToolInfos []*ToolInfo
	Extra     map[string]any
}

// RetryContext describes the current model attempt and its result.
type RetryContext struct {
	Attempt       int
	Messages      []*Message
	OutputMessage *Message
	Err           error
	Options       []ModelOption
}

// RetryDecision determines whether and how a model attempt is repeated.
type RetryDecision struct {
	Retry        bool
	Messages     []*Message
	Options      []ModelOption
	Backoff      time.Duration
	RejectReason any
}

// RetryConfig enables an explicit, bounded number of model retries.
// No retries or backoff are implicit when this value is nil.
type RetryConfig struct {
	MaxRetries  int
	ShouldRetry func(context.Context, *RetryContext) *RetryDecision
	IsRetryable func(context.Context, error) bool
	BackoffFunc func(context.Context, int) time.Duration
}

// Middleware customizes the native loop without owning it.
type Middleware interface {
	BeforeAgent(context.Context, *RunContext) (context.Context, *RunContext, error)
	AfterAgent(context.Context, *RunState) (context.Context, error)
	BeforeModelRewriteState(context.Context, *RunState, *ModelContext) (context.Context, *RunState, error)
	AfterModelRewriteState(context.Context, *RunState, *ModelContext) (context.Context, *RunState, error)
	WrapModel(context.Context, BaseChatModel, *ModelContext) (BaseChatModel, error)
	BeforeModelCall(context.Context, *ModelCall, *ModelContext) (context.Context, *ModelCall, error)
	WrapToolCall(context.Context, ToolCallEndpoint, *ToolContext) (ToolCallEndpoint, error)
}

// BaseMiddleware provides no-op implementations for selective embedding.
type BaseMiddleware struct{}

func (*BaseMiddleware) BeforeAgent(ctx context.Context, run *RunContext) (context.Context, *RunContext, error) {
	return ctx, run, nil
}

func (*BaseMiddleware) AfterAgent(ctx context.Context, _ *RunState) (context.Context, error) {
	return ctx, nil
}

func (*BaseMiddleware) BeforeModelRewriteState(ctx context.Context, state *RunState, _ *ModelContext) (context.Context, *RunState, error) {
	return ctx, state, nil
}

func (*BaseMiddleware) AfterModelRewriteState(ctx context.Context, state *RunState, _ *ModelContext) (context.Context, *RunState, error) {
	return ctx, state, nil
}

func (*BaseMiddleware) WrapModel(_ context.Context, model BaseChatModel, _ *ModelContext) (BaseChatModel, error) {
	return model, nil
}

func (*BaseMiddleware) BeforeModelCall(ctx context.Context, call *ModelCall, _ *ModelContext) (context.Context, *ModelCall, error) {
	return ctx, call, nil
}

func (*BaseMiddleware) WrapToolCall(_ context.Context, endpoint ToolCallEndpoint, _ *ToolContext) (ToolCallEndpoint, error) {
	return endpoint, nil
}
