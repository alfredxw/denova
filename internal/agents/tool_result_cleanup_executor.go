package agents

import (
	"context"
	"fmt"

	agent "github.com/alfredxw/denova/agent"
)

// ToolResultCleanupExecutionMode identifies how a provider-visible cleanup is
// installed. Both modes use the same CleanupPlan and append-only journal
// record; native execution is only a transport optimization.
type ToolResultCleanupExecutionMode string

const (
	ToolResultCleanupLocalProjection ToolResultCleanupExecutionMode = "local_projection"
	ToolResultCleanupNativeCacheEdit ToolResultCleanupExecutionMode = "native_cache_edit"
)

// ToolResultCleanupExecutor is the narrow provider capability boundary for a
// native cache edit. Providers that do not install an executor always use the
// deterministic local projection. Execute must not change CleanupPlan
// semantics or publish canonical conversation state.
type ToolResultCleanupExecutor interface {
	ExecutionMode() ToolResultCleanupExecutionMode
	Execute(context.Context, *agent.ModelRequestSnapshot, ToolResultCleanupPlan) error
}

type toolResultCleanupExecutorContextKey struct{}

func contextWithToolResultCleanupExecutor(ctx context.Context, executor ToolResultCleanupExecutor) context.Context {
	if executor == nil {
		return ctx
	}
	return context.WithValue(ctx, toolResultCleanupExecutorContextKey{}, executor)
}

func resolveToolResultCleanupExecutor(ctx context.Context) (ToolResultCleanupExecutor, ToolResultCleanupExecutionMode, error) {
	executor, _ := ctx.Value(toolResultCleanupExecutorContextKey{}).(ToolResultCleanupExecutor)
	if executor == nil {
		return nil, ToolResultCleanupLocalProjection, nil
	}
	mode := executor.ExecutionMode()
	if mode != ToolResultCleanupNativeCacheEdit {
		return nil, ToolResultCleanupLocalProjection, fmt.Errorf("unsupported tool-result cleanup executor mode %q", mode)
	}
	return executor, mode, nil
}
