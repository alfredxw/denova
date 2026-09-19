package toolruntime

import (
	"context"
	"errors"

	agent "github.com/alfredxw/denova/agent"
)

// InvokeHostTool applies the existing product validation, workspace gate and
// receipt projection to one durably admitted external call. The caller must
// persist the returned result even when err is non-nil: a mutation may have
// committed immediately before cancellation or a transport failure.
func InvokeHostTool(ctx context.Context, policy OrchestratorConfig, identity HostToolIdentity, definition agent.ToolDefinition, args string) (agent.ToolResult, error) {
	ctx, err := ContextWithHostToolIdentity(ctx, identity)
	if err != nil {
		return agent.ToolResult{}, err
	}
	if err := definition.Validate(ctx); err != nil {
		return agent.ToolResult{}, err
	}
	info, err := definition.Tool.Info(ctx)
	if err != nil {
		return agent.ToolResult{}, err
	}
	if info == nil {
		return agent.ToolResult{}, errors.New("host tool has no schema")
	}
	toolContext := &agent.ToolContext{
		Name: info.Name, ProviderCallID: identity.ProviderCallID, ExecutionID: identity.ExecutionID,
		Definition: agent.ToolDefinitionSnapshot{Info: info, Descriptor: definition.Descriptor},
	}
	var returned agent.ToolResult
	middleware := NewOrchestratorMiddleware(policy)
	endpoint, err := middleware.WrapToolCall(ctx, func(ctx context.Context, args string, opts ...agent.ToolOption) (agent.ToolResult, error) {
		var callErr error
		returned, callErr = definition.Tool.Run(ctx, args, opts...)
		return returned, callErr
	}, toolContext)
	if err != nil {
		return agent.ToolResult{}, err
	}
	result, err := endpoint(ctx, args)
	if err != nil && (len(returned.Details) > 0 || len(returned.Effects) > 0) {
		// The Native middleware may short-circuit on a cancelled context. Host
		// settlement still needs a committed domain receipt, never a retry.
		decision := middleware.buildToolDecision(ctx, toolContext, args)
		result, record := projectToolError(decision, args, returned, err, policy.ToolResultMaxBytes)
		result, effectErr := appendAgentMutationEffect(result, record)
		return result, errors.Join(err, effectErr)
	}
	return result, err
}
