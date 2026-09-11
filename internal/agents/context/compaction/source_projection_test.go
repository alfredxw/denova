package compaction

import (
	"reflect"
	"strings"
	"testing"

	"denova/internal/agents/toolresult"

	agent "github.com/alfredxw/denova/agent"
)

func TestCompactionSourceUsesTheAuthorizedRuntimeArtifactProjection(t *testing.T) {
	const stored = "artifacts/result.txt"
	const resolved = "/runtime/project/artifacts/result.txt"
	raw := []*agent.Message{
		agent.AssistantMessage("", []agent.ToolCall{{ID: "read-artifact", Type: "function", Function: agent.FunctionCall{Name: "read", Arguments: `{}`}}}),
		agent.ToolMessage(agent.TextToolResult("Read "+stored), "read-artifact", agent.WithToolName("read")),
	}
	raw[1].ToolResult.Artifacts = []agent.ToolArtifactRef{{ID: "result", ReadablePath: stored}}
	projected := cloneMessages(raw)
	projected[1].Content = "Read " + resolved
	projected[1].ToolResult.Artifacts[0].ReadablePath = resolved
	source := cloneMessages(raw)
	source[1].Content = "[source turn_id=archive branch_id=main]\n" + source[1].Content
	request := agent.CompactionCompactRequest{
		Messages: raw, ContextMessages: projected, SourceMessages: source,
		ModelSnapshot: (&agent.ModelCall{Messages: projected}).Snapshot(),
	}
	result, err := projectCompactionSource(request, toolresult.ContextPolicy{Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	visible, locator := compactionSourceMatchMessage(result[1])
	if locator == "" || !sameProviderVisibleMessage(visible, projected[1]) {
		t.Fatal("summary source did not use the exact provider artifact projection")
	}
	if raw[1].ToolResult.Artifacts[0].ReadablePath != stored || strings.Contains(raw[1].Content, resolved) {
		t.Fatal("runtime paths entered canonical history")
	}
}

func TestCleanupSourceProjectionPreservesRepeatedToolOccurrences(t *testing.T) {
	turn := []*agent.Message{
		agent.UserMessage("Read the archive"),
		agent.AssistantMessage("", []agent.ToolCall{{ID: "reused", Type: "function", Function: agent.FunctionCall{Name: "read", Arguments: `{}`}}}),
		{Role: agent.ToolRole, ToolCallID: "reused", ToolName: "read", Content: "original evidence",
			ToolResult: &agent.ToolResultSummary{Status: agent.ToolResultSuccess, ResultRetention: agent.ToolResultDeferred,
				ContextHints: &agent.ToolResultContextHints{ContextValue: agent.ToolResultContextDiscardable}}},
		agent.AssistantMessage("Archive reviewed", nil),
	}
	raw := append(cloneMessages(turn), cloneMessages(turn)...)
	contextMessages := cloneMessages(raw)
	contextMessages[2].Content = "[Older tool result removed.]"
	contextMessages[2].ToolResult.ContextHints = nil
	contextMessages[2].ToolResult.ResultRetention = agent.ToolResultProtected
	policy := toolresult.ContextPolicy{Enabled: true}
	primary := append([]*agent.Message{agent.SystemMessage("Stable instructions")}, toolresult.ApplyContextPolicy(contextMessages, policy)...)
	primary = append(primary, agent.UserMessage("Continue"))
	request := agent.CompactionCompactRequest{
		Messages: raw, ContextMessages: contextMessages,
		ModelSnapshot: (&agent.ModelCall{Messages: primary}).Snapshot(),
	}
	for _, scenario := range []struct {
		name string
		from int
	}{
		{"both_turns", 0},
		{"newest_turn", len(turn)},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			request.SourceMessages = cloneMessages(raw[scenario.from:])
			locatorCount := 0
			for _, message := range request.SourceMessages {
				if message.Content != "" {
					message.Content = "[source turn_id=selected branch_id=main]\n" + message.Content
					locatorCount++
				}
			}
			result, err := projectCompactionSource(request, policy)
			if err != nil {
				t.Fatal(err)
			}
			positions, locators, matched := locateCompactionSourceInPrimary(primary, result)
			if !matched || len(locators) != locatorCount || positions[0] != scenario.from+1 {
				t.Fatalf("projected range lost occurrence identity: positions=%v locators=%v matched=%v", positions, locators, matched)
			}
			for index, message := range result {
				visible, _ := compactionSourceMatchMessage(message)
				if !sameProviderVisibleMessage(visible, primary[positions[index]]) {
					t.Fatalf("source message %d differs from the provider", index)
				}
			}
			result[0].Content = "mutated result"
			if !reflect.DeepEqual(request.ModelSnapshot.Messages(), primary) || raw[2].Content != "original evidence" {
				t.Fatal("source projection mutated primary or canonical history")
			}
		})
	}
	request.SourceMessages = cloneMessages(raw)
	for _, drift := range []struct {
		name   string
		mutate func([]*agent.Message)
	}{
		{"unrelated_user", func(messages []*agent.Message) { messages[1].Content = "different user request" }},
		{"unauthorized_placeholder", func(messages []*agent.Message) { messages[3].Content = "different placeholder" }},
		{"tool_status", func(messages []*agent.Message) { messages[3].ToolResult.Status = agent.ToolResultError }},
	} {
		t.Run(drift.name, func(t *testing.T) {
			changed := cloneMessages(primary)
			drift.mutate(changed)
			request.ModelSnapshot = (&agent.ModelCall{Messages: changed}).Snapshot()
			if _, err := projectCompactionSource(request, policy); err == nil {
				t.Fatal("Cleanup projection accepted unrelated provider drift")
			}
		})
	}
}
