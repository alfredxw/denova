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
	// Attempt is the zero-based provider attempt for this model step. Every
	// retry re-enters BeforeModelCall and fixed context maintenance.
	Attempt int

	maintenanceMessages []*Message
	// stablePrefixSeed is lifecycle-owned provenance. It is deliberately kept
	// outside Message.Extra so model output and caller middleware cannot label
	// arbitrary body messages as cache-stable context.
	stablePrefixSeed []*Message

	contextNormalization *ContextNormalizationMetrics
}

// ContextNormalizationMetrics is a bounded, provider-neutral report produced
// by a presentation middleware when it repairs the final model request. It
// deliberately contains counts only: repaired message bodies never enter
// lifecycle telemetry.
type ContextNormalizationMetrics struct {
	RepairCount    int
	MessagesBefore int
	MessagesAfter  int
}

// ReportContextNormalization reports that this middleware repaired the model
// request. Multiple middleware reports for one attempt are accumulated and
// bounded by the number of messages they observed.
func (context *ModelContext) ReportContextNormalization(metrics ContextNormalizationMetrics) {
	if context == nil || metrics.RepairCount <= 0 {
		return
	}
	metrics.MessagesBefore = max(0, metrics.MessagesBefore)
	metrics.MessagesAfter = max(0, metrics.MessagesAfter)
	metrics.RepairCount = min(metrics.RepairCount, max(1, metrics.MessagesBefore+metrics.MessagesAfter))
	if context.contextNormalization == nil {
		context.contextNormalization = &metrics
		return
	}
	context.contextNormalization.RepairCount += metrics.RepairCount
	context.contextNormalization.RepairCount = min(
		context.contextNormalization.RepairCount,
		max(1, context.contextNormalization.MessagesBefore+metrics.MessagesAfter),
	)
	context.contextNormalization.MessagesAfter = metrics.MessagesAfter
}

// ContextNormalization returns the bounded repair report for this model
// attempt. Hosts may use it for observability; only Agent lifecycle consumes it
// to publish the canonical event.
func (context *ModelContext) ContextNormalization() (ContextNormalizationMetrics, bool) {
	if context == nil || context.contextNormalization == nil {
		return ContextNormalizationMetrics{}, false
	}
	return *context.contextNormalization, true
}

func (context *ModelContext) takeContextNormalization() (ContextNormalizationMetrics, bool) {
	metrics, present := context.ContextNormalization()
	if context != nil {
		context.contextNormalization = nil
	}
	return metrics, present
}

func (context *ModelContext) contextMaintenanceMessages() []*Message {
	if context == nil {
		return nil
	}
	return cloneMessages(context.maintenanceMessages)
}

type contextMaintenanceCommittedKey struct{}

func contextWithMaintenanceCommitted(ctx context.Context) context.Context {
	return context.WithValue(ctx, contextMaintenanceCommittedKey{}, true)
}

// ContextMaintenanceCommitted reports that Agent already published a durable
// checkpoint during this native loop. Reversible maintenance middleware must
// leave subsequent model calls unchanged so one run cannot publish competing
// context projections.
func ContextMaintenanceCommitted(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	committed, _ := ctx.Value(contextMaintenanceCommittedKey{}).(bool)
	return committed
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

	stablePrefixMessages int
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
	model                BaseChatModel
	messages             []*Message
	options              []ModelOption
	streaming            bool
	stablePrefixMessages int
}

// Snapshot freezes a detached copy of this final model call.
func (call *ModelCall) Snapshot() *ModelRequestSnapshot {
	if call == nil {
		return nil
	}
	return &ModelRequestSnapshot{
		model: call.Model, messages: cloneMessages(call.Messages),
		options: append([]ModelOption(nil), call.Options...), streaming: call.Streaming,
		stablePrefixMessages: min(max(0, call.stablePrefixMessages), len(call.Messages)),
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

// StablePrefixMessages reports the lifecycle-authenticated contiguous message
// prefix that may be reused by provider caches. The boundary is captured after
// caller middleware and cannot be supplied through Message content or Extra.
// Tool schemas remain a separate stable prefix component in ResolvedOptions.
func (snapshot *ModelRequestSnapshot) StablePrefixMessages() int {
	if snapshot == nil {
		return 0
	}
	return min(max(0, snapshot.stablePrefixMessages), len(snapshot.messages))
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
		stablePrefixMessages: snapshot.StablePrefixMessages(),
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
