package agents

import (
	"testing"

	agent "github.com/alfredxw/denova/agent"

	"denova/internal/agents/session"
)

func TestToolResultCleanupProjectionUsesAbsoluteIndexAndCallIdentity(t *testing.T) {
	rich := pressureToolResult("call-1", "bounded rich body", agent.ToolResultDeferred, agent.ToolResultContextNormal)
	messages := []*agent.Message{
		agent.UserMessage("request"),
		agent.AssistantMessage("", []agent.ToolCall{pressureCall("call-1", 1)}),
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

func TestResolveToolResultCleanupTargetsSupportsProviderLocalIDReuse(t *testing.T) {
	first := pressureToolResult("reused-call", "first", agent.ToolResultDeferred, agent.ToolResultContextNormal)
	second := pressureToolResult("reused-call", "second", agent.ToolResultDeferred, agent.ToolResultContextNormal)
	third := pressureToolResult("reused-call", "third", agent.ToolResultDeferred, agent.ToolResultContextNormal)
	canonical := []*agent.Message{
		agent.AssistantMessage("", []agent.ToolCall{pressureCall("reused-call", 0)}), first,
		agent.UserMessage("later"),
		agent.AssistantMessage("", []agent.ToolCall{pressureCall("reused-call", 1)}), second,
		agent.UserMessage("latest"),
		agent.AssistantMessage("", []agent.ToolCall{pressureCall("reused-call", 2)}), third,
	}
	visible := canonical[3:]
	resolved, err := ResolveToolResultCleanupTargets(visible, canonical, ToolResultCleanupPlan{Replacements: []ToolResultCleanupReplacement{
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
