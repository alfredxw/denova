package agent

import (
	"context"
	"encoding/json"
	"errors"
)

// NestedToolCall is the only public request accepted by the in-execution tool
// seam. Callers cannot provide a definition, descriptor, policy, or endpoint.
type NestedToolCall struct {
	Name      string
	Arguments json.RawMessage
}

// NestedToolOutcome is the bounded projection available to an orchestrating
// tool. Canonical effects, receipts, details, and display content stay owned by
// the executor that produced them.
type NestedToolOutcome struct {
	Name      string
	Status    ToolResultStatus
	Reason    ToolSyntheticReason
	Output    json.RawMessage
	Truncated bool
	Artifacts []ToolArtifactRef
}

type nestedToolInvoker func(context.Context, []NestedToolCall) ([]NestedToolOutcome, error)
type nestedToolInvokerContextKey struct{}

func contextWithNestedToolInvoker(ctx context.Context, invoke nestedToolInvoker) context.Context {
	return context.WithValue(ctx, nestedToolInvokerContextKey{}, invoke)
}

// CallNestedTools re-enters the current immutable Agent registry and complete
// execution pipeline. It fails closed outside a concrete native tool call.
func CallNestedTools(ctx context.Context, calls []NestedToolCall) ([]NestedToolOutcome, error) {
	if ctx == nil {
		return nil, errors.New("nested tool calls require an execution context")
	}
	invoke, ok := ctx.Value(nestedToolInvokerContextKey{}).(nestedToolInvoker)
	if !ok || invoke == nil {
		return nil, errors.New("nested tool calls are unavailable outside an Agent tool execution")
	}
	if len(calls) == 0 {
		return []NestedToolOutcome{}, nil
	}
	cloned := make([]NestedToolCall, len(calls))
	for index, call := range calls {
		cloned[index] = NestedToolCall{Name: call.Name, Arguments: append(json.RawMessage(nil), call.Arguments...)}
	}
	return invoke(ctx, cloned)
}
