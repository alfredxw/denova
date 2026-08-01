package chat

import (
	"context"
	"denova/internal/agents/toolresult"
	"fmt"

	agent "github.com/alfredxw/denova/agent"

	agentcontext "denova/internal/agents/context"
)

// ToolResultCleanupExecutor is the narrow provider capability boundary for a
// native cache edit. Providers that do not install an executor always use the
// deterministic local projection. Execute must not change CleanupPlan
// semantics or publish canonical conversation state.
type ToolResultCleanupExecutor interface {
	ExecutionMode() agentcontext.ToolResultCleanupExecutionMode
	Execute(context.Context, *agent.ModelRequestSnapshot, toolresult.CleanupPlan) error
}

type toolResultCleanupExecutorContextKey struct{}

func contextWithToolResultCleanupExecutor(ctx context.Context, executor ToolResultCleanupExecutor) context.Context {
	if executor == nil {
		return ctx
	}
	return context.WithValue(ctx, toolResultCleanupExecutorContextKey{}, executor)
}

func resolveToolResultCleanupExecutor(ctx context.Context) (ToolResultCleanupExecutor, agentcontext.ToolResultCleanupExecutionMode, error) {
	executor, _ := ctx.Value(toolResultCleanupExecutorContextKey{}).(ToolResultCleanupExecutor)
	if executor == nil {
		return nil, agentcontext.ToolResultCleanupLocalProjection, nil
	}
	mode := executor.ExecutionMode()
	if mode != agentcontext.ToolResultCleanupNativeCacheEdit {
		return nil, agentcontext.ToolResultCleanupLocalProjection, fmt.Errorf("unsupported tool-result cleanup executor mode %q", mode)
	}
	return executor, mode, nil
}
