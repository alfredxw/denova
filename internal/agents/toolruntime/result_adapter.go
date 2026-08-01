package toolruntime

import (
	"context"
	agenttool "denova/internal/agents/tool"

	agent "github.com/alfredxw/denova/agent"

	"denova/internal/agents/toolresult"
)

// processToolResult projects the orchestration decision into the intentionally
// smaller result-lifecycle contract. Approval state and scheduling details do
// not leak into toolresult.
func processToolResult(
	ctx context.Context,
	decision agenttool.Decision,
	arguments string,
	result agent.ToolResult,
	policy toolresult.ProcessingPolicy,
) (agent.ToolResult, error) {
	return toolresult.Process(ctx, toolresult.Call{
		ToolName:       decision.ToolName,
		ProviderCallID: decision.ProviderCallID,
		ExecutionID:    decision.ExecutionID,
		Descriptor:     decision.Descriptor,
	}, arguments, result, policy)
}
