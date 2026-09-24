package toolruntime

import (
	"context"
	"errors"
	"log/slog"

	agent "github.com/alfredxw/denova/agent"
)

// HostToolInvocation records entry into Tool.Run, not whether its effects
// committed. Only InvokeHostTool can establish that a call was not invoked.
type HostToolInvocation uint8

const (
	HostToolInvoked HostToolInvocation = iota
	HostToolNotInvoked
)

// HostToolResult keeps execution evidence separate from tool-owned output.
// NotInvoked permits failure settlement without a domain receipt. Invoked
// still requires the caller's normal receipt/unknown-effect recovery policy.
type HostToolResult struct {
	Result     agent.ToolResult
	Invocation HostToolInvocation
}

// InvokeHostTool applies the existing product validation, workspace gate and
// receipt projection to one durably admitted external call. The caller must
// persist the returned result even when err is non-nil: a mutation may have
// committed immediately before cancellation or a transport failure.
func InvokeHostTool(ctx context.Context, policy OrchestratorConfig, identity HostToolIdentity, definition agent.ToolDefinition, args string) (HostToolResult, error) {
	outcome := HostToolResult{Invocation: HostToolNotInvoked}
	ctx, err := ContextWithHostToolIdentity(ctx, identity)
	if err != nil {
		return outcome, err
	}
	if err := definition.Validate(ctx); err != nil {
		return outcome, err
	}
	info, err := definition.Tool.Info(ctx)
	if err != nil {
		return outcome, err
	}
	if info == nil {
		return outcome, errors.New("host tool has no schema")
	}
	toolContext := &agent.ToolContext{
		Name: info.Name, ProviderCallID: identity.ProviderCallID, ExecutionID: identity.ExecutionID,
		Definition: agent.ToolDefinitionSnapshot{Info: info, Descriptor: definition.Descriptor},
	}
	var returned agent.ToolResult
	middleware := NewOrchestratorMiddleware(policy)
	endpoint, err := middleware.WrapToolCall(ctx, func(ctx context.Context, args string, opts ...agent.ToolOption) (agent.ToolResult, error) {
		outcome.Invocation = HostToolInvoked
		var callErr error
		returned, callErr = definition.Tool.Run(ctx, args, opts...)
		return returned, callErr
	}, toolContext)
	if err != nil {
		return outcome, err
	}
	if policy.AgentKind == "interactive_story" {
		endpoint, err = NewInteractiveStoryMiddleware().WrapToolCall(ctx, endpoint, toolContext)
		if err != nil {
			return outcome, err
		}
	}
	outcome.Result, err = endpoint(ctx, args)
	if err != nil && outcome.Invocation == HostToolNotInvoked {
		slog.InfoContext(ctx, "Host tool failed before execution", "operation_id", identity.OperationID, "execution_id", identity.ExecutionID, "tool", info.Name, "error", err)
	}
	if err != nil && (len(returned.Details) > 0 || len(returned.Effects) > 0) {
		// The Native middleware may short-circuit on a cancelled context. Host
		// settlement still needs a committed domain receipt, never a retry.
		decision := middleware.buildToolDecision(ctx, toolContext, args)
		result, record := projectToolError(decision, args, returned, err, policy.ToolResultMaxBytes)
		result, effectErr := appendAgentMutationEffect(result, record)
		outcome.Result = result
		return outcome, errors.Join(err, effectErr)
	}
	return outcome, err
}
