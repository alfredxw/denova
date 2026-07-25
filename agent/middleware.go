package agent

import (
	"context"
	"time"
)

// ToolCallEndpoint is the single middleware seam for structured tool calls.
type ToolCallEndpoint func(context.Context, string, ...ToolOption) (ToolResult, error)

// ToolContext identifies one concrete tool call.
type ToolContext struct {
	Index      int
	Name       string
	CallID     string
	Definition ToolDefinitionSnapshot
}

// ModelContext contains read-only metadata for a model invocation.
type ModelContext struct {
	Tools []*ToolInfo
	Retry *RetryConfig
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

func (*BaseMiddleware) WrapToolCall(_ context.Context, endpoint ToolCallEndpoint, _ *ToolContext) (ToolCallEndpoint, error) {
	return endpoint, nil
}
