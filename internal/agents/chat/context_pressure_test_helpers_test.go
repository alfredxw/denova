package chat

import (
	"fmt"
	"strings"

	agent "github.com/alfredxw/denova/agent"

	agentcontext "denova/internal/agents/context"
)

func pressureTestPolicy(window int) agentcontext.ContextPressurePolicy {
	return agentcontext.ContextPressurePolicy{
		Enabled: true, CompactionEnabled: true, CleanupEnabled: true,
		Scope: agentcontext.ContextPressureTotal, ContextWindowTokens: window,
		CleanupThreshold: 0.70, CleanupTarget: 0.60, CleanupMinTokens: 2000,
		KeepRecentGroups: 3, KeepRecentTokens: 1000, WarmSuffixTokens: 8000,
		EagerMinTokens: 20_000, EagerMinContextRatio: 0.10,
		CompactionThreshold: 0.85, CompactionRecoveryBand: 0.80,
	}
}

func pressureHistory(groups, resultWords int, retention agent.ToolResultRetentionMode, value agent.ToolResultContextValue) []*agent.Message {
	messages := []*agent.Message{agent.SystemMessage("stable system")}
	for index := 0; index < groups; index++ {
		callID := fmt.Sprintf("call-%d", index)
		messages = append(messages,
			agent.UserMessage(fmt.Sprintf("request %d", index)),
			agent.AssistantMessage("", []agent.ToolCall{pressureCall(callID, index)}),
			pressureToolResult(callID, fmt.Sprintf("rich-result-%d ", index)+strings.Repeat("payload ", resultWords), retention, value),
			agent.AssistantMessage(fmt.Sprintf("used result %d", index), nil),
		)
	}
	return append(messages, agent.UserMessage("current turn"))
}

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
			Kind:            agent.ToolResultRecoveryRead,
			Reference:       map[string]any{"path": ".denova/artifacts/session/" + callID + ".log"},
			ArtifactPath:    ".denova/artifacts/session/" + callID + ".log",
			EstimatedBytes:  int64(len(content)),
			EstimatedTokens: agentcontext.EstimateStringTokens(content),
		},
		ContextValue:    value,
		SupersessionKey: "read:chapter.md",
	}
	result.Artifacts = []agent.ToolArtifactRef{{
		ID:           "artifact-" + callID,
		Purpose:      agent.ToolArtifactPurposeCompleteModelOutput,
		ReadablePath: result.ContextHints.Recovery.ArtifactPath,
		ContentType:  "text/plain", EstimatedBytes: int64(len(content)),
		EstimatedTokens: agentcontext.EstimateStringTokens(content), Complete: true,
	}}
	return agent.ToolMessage(result, callID, agent.WithToolName("read"))
}
