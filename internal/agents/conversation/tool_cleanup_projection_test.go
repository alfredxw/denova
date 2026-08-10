package conversation

import (
	"denova/internal/agents/toolresult"
	"fmt"
	"testing"

	agent "github.com/alfredxw/denova/agent"

	"denova/internal/agents/session"
)

func TestToolResultCleanupProjectionUsesAbsoluteIndexAndCallIdentity(t *testing.T) {
	rich := cleanupProjectionResult("call-1", "bounded rich body", agent.ToolResultDeferred, agent.ToolResultContextNormal)
	messages := []*agent.Message{
		agent.UserMessage("request"),
		agent.AssistantMessage("", []agent.ToolCall{cleanupProjectionCall("call-1", 1)}),
		rich,
		agent.AssistantMessage("done", nil),
	}
	record := session.ToolResultCleanupRecord{Replacements: []session.ToolResultReplacement{{
		MessageIndex: 12, ToolCallID: "call-1", Placeholder: "frozen placeholder",
	}}}
	projected := applyToolResultCleanupProjection(messages, 10, record)
	if projected[2].Content != "frozen placeholder" || projected[2].ToolResult.ContextHints != nil || projected[2].ToolResult.ResultRetention != agent.ToolResultProtected {
		t.Fatalf("cleanup projection = %#v", projected[2])
	}
	if messages[2].Content != "bounded rich body" || messages[2].ToolResult.ContextHints == nil {
		t.Fatalf("canonical rich message mutated: %#v", messages[2])
	}

	record.Replacements[0].ToolCallID = "different-call"
	unchanged := applyToolResultCleanupProjection(messages, 10, record)
	if unchanged[2].Content != "bounded rich body" {
		t.Fatalf("mismatched identity changed result: %#v", unchanged[2])
	}
}

func cleanupProjectionCall(callID string, index int) agent.ToolCall {
	return agent.ToolCall{
		ID: callID, Type: "function",
		Function: agent.FunctionCall{Name: "read", Arguments: fmt.Sprintf(`{"path":"chapter-%d.md"}`, index)},
	}
}

func cleanupProjectionResult(callID, content string, retention agent.ToolResultRetentionMode, value agent.ToolResultContextValue) *agent.Message {
	result := agent.TextToolResult(content)
	result.ResultRetention = retention
	result.ContextHints = &agent.ToolResultContextHints{
		Recovery:     agent.ToolResultRecoveryHint{Kind: agent.ToolResultRecoveryRead},
		ContextValue: value,
	}
	return agent.ToolMessage(result, callID, agent.WithToolName("read"))
}

func TestResolveToolResultCleanupTargetsSupportsProviderLocalIDReuse(t *testing.T) {
	first := cleanupProjectionResult("reused-call", "first", agent.ToolResultDeferred, agent.ToolResultContextNormal)
	second := cleanupProjectionResult("reused-call", "second", agent.ToolResultDeferred, agent.ToolResultContextNormal)
	third := cleanupProjectionResult("reused-call", "third", agent.ToolResultDeferred, agent.ToolResultContextNormal)
	canonical := []*agent.Message{
		agent.AssistantMessage("", []agent.ToolCall{cleanupProjectionCall("reused-call", 0)}), first,
		agent.UserMessage("later"),
		agent.AssistantMessage("", []agent.ToolCall{cleanupProjectionCall("reused-call", 1)}), second,
		agent.UserMessage("latest"),
		agent.AssistantMessage("", []agent.ToolCall{cleanupProjectionCall("reused-call", 2)}), third,
	}
	visible := canonical[3:]
	resolved, err := toolresult.ResolveCleanupTargets(visible, canonical, toolresult.CleanupPlan{Replacements: []toolresult.CleanupReplacement{
		{MessageIndex: 1, ToolCallID: "reused-call", Placeholder: "second receipt"},
		{MessageIndex: 4, ToolCallID: "reused-call", Placeholder: "third receipt"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(resolved) != 2 || resolved[0].MessageIndex != 4 || resolved[1].MessageIndex != 7 {
		t.Fatalf("resolved provider-local IDs = %#v", resolved)
	}
}
