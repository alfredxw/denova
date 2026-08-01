package conversation

import (
	"fmt"

	agent "github.com/alfredxw/denova/agent"

	agentcontext "denova/internal/agents/context"
)

func pressureCall(callID string, index int) agent.ToolCall {
	return agent.ToolCall{
		ID: callID, Type: "function",
		Function: agent.FunctionCall{Name: "read", Arguments: fmt.Sprintf(`{"path":"chapter-%d.md"}`, index)},
	}
}

func pressureToolResult(callID, content string, retention agent.ToolResultRetentionMode, value agent.ToolResultContextValue) *agent.Message {
	result := agent.TextToolResult(content)
	result.ResultRetention = retention
	result.ContextHints = &agent.ToolResultContextHints{
		Recovery: agent.ToolResultRecoveryHint{
			Kind: agent.ToolResultRecoveryRead,
			Reference: map[string]any{
				"path": ".denova/artifacts/session/" + callID + ".log",
			},
			ArtifactPath:   ".denova/artifacts/session/" + callID + ".log",
			EstimatedBytes: int64(len(content)), EstimatedTokens: agentcontext.EstimateStringTokens(content),
		},
		ContextValue: value, SupersessionKey: "read:chapter.md",
	}
	return agent.ToolMessage(result, callID, agent.WithToolName("read"))
}
