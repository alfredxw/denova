package agents

import (
	"encoding/json"
	"testing"

	agent "github.com/alfredxw/denova/agent"

	"denova/config"
	"denova/internal/agents/session"
)

func TestIncompleteToolExchangeGetsStableUnknownEffectResult(t *testing.T) {
	t.Parallel()

	messages := []*agent.Message{
		agent.UserMessage("update the chapter"),
		agent.AssistantMessage("", []agent.ToolCall{{
			ID: "call-write", Function: agent.FunctionCall{Name: "write", Arguments: `{"path":"chapter.md"}`},
		}}),
		agent.UserMessage("continue"),
	}
	policy := ToolResultContextPolicy{Enabled: false}
	first := applyToolResultContextPolicy(messages, policy)
	second := applyToolResultContextPolicy(first, policy)
	if len(first) != 4 || len(second) != len(first) {
		t.Fatalf("recovered model context lengths = %d then %d, want stable four-message exchange", len(first), len(second))
	}
	if first[1].Role != agent.Assistant || len(first[1].ToolCalls) != 1 || first[1].ToolCalls[0].ID != "call-write" {
		t.Fatalf("recovered tool call = %#v", first[1])
	}
	result := first[2]
	if result.Role != agent.ToolRole || result.ToolCallID != "call-write" || result.ToolName != "write" {
		t.Fatalf("synthetic tool result identity = %#v", result)
	}
	if result.ToolResult == nil || result.ToolResult.Status != agent.ToolResultError ||
		result.ToolResult.SyntheticReason != agent.ToolSyntheticEffectUnknown {
		t.Fatalf("synthetic tool result summary = %#v", result.ToolResult)
	}
	var payload struct {
		Schema         string `json:"schema"`
		Status         string `json:"status"`
		AutomaticRetry bool   `json:"automatic_retry"`
	}
	if err := json.Unmarshal([]byte(result.Content), &payload); err != nil {
		t.Fatalf("synthetic tool result is not provider-neutral JSON: %v", err)
	}
	if payload.Schema != "agent.tool_result.recovery.v1" || payload.Status != "effect_unknown" || payload.AutomaticRetry {
		t.Fatalf("synthetic tool recovery payload = %#v", payload)
	}
	if second[2].Content != result.Content {
		t.Fatalf("synthetic recovery result changed across assembly: %q != %q", second[2].Content, result.Content)
	}
}

func TestIncompleteParallelToolCallsCompleteOnlyMissingResults(t *testing.T) {
	t.Parallel()

	messages := []*agent.Message{
		agent.AssistantMessage("", []agent.ToolCall{
			{ID: "call-read", Function: agent.FunctionCall{Name: "read", Arguments: `{"path":"a.md"}`}},
			{ID: "call-write", Function: agent.FunctionCall{Name: "write", Arguments: `{"path":"b.md"}`}},
		}),
		agent.ToolMessage(agent.TextToolResult("read result"), "call-read", agent.WithToolName("read")),
	}
	got := applyToolResultContextPolicy(messages, ToolResultContextPolicy{Enabled: true})
	if len(got) != 3 {
		t.Fatalf("completed parallel exchange = %#v", got)
	}
	counts := map[string]int{}
	for _, message := range got {
		if message.Role == agent.ToolRole {
			counts[message.ToolCallID]++
		}
	}
	if counts["call-read"] != 1 || counts["call-write"] != 1 {
		t.Fatalf("tool result counts = %#v, want one per call", counts)
	}
}

func TestIncompleteReusedCallIDCompletesOnlyItsAssistantBatch(t *testing.T) {
	t.Parallel()

	messages := []*agent.Message{
		agent.AssistantMessage("", []agent.ToolCall{{
			ID: "provider-local", Type: "function",
			Function: agent.FunctionCall{Name: "write", Arguments: `{"path":"one.md"}`},
		}}),
		agent.UserMessage("next turn"),
		agent.AssistantMessage("", []agent.ToolCall{{
			ID: "provider-local", Type: "function",
			Function: agent.FunctionCall{Name: "read", Arguments: `{"path":"two.md"}`},
		}}),
		agent.ToolMessage(agent.TextToolResult("second result"), "provider-local"),
	}

	got := applyToolResultContextPolicy(messages, ToolResultContextPolicy{Enabled: true})
	if len(got) != 5 || got[0].Role != agent.Assistant || got[1].ToolCallID != "provider-local" ||
		!isUnknownToolEffectResult(got[1].Content) || got[2].Role != agent.User ||
		got[3].Role != agent.Assistant || got[4].Content != "second result" {
		t.Fatalf("provider-local ID reuse suppressed the missing-result recovery: %#v", got)
	}
	if got[1].ToolName != "write" || got[4].ToolName != "read" {
		t.Fatalf("reused ID results were paired to the wrong assistant batch: %#v", got)
	}
}

func TestCanonicalSessionHistoryProjectsUnknownToolEffectIntoNextModelContext(t *testing.T) {
	t.Parallel()

	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.Create("tool recovery context")
	if err != nil {
		t.Fatal(err)
	}
	if err := sess.Append(agent.UserMessage("update the chapter")); err != nil {
		t.Fatal(err)
	}
	if err := sess.AppendContextMessage(agent.AssistantMessage("", []agent.ToolCall{{
		ID: "canonical-call", Function: agent.FunctionCall{Name: "write", Arguments: `{"path":"chapter.md"}`},
	}})); err != nil {
		t.Fatal(err)
	}
	conversation := NewSessionConversationForAgent(sess, &config.Config{}, AgentKindIDE)
	snapshot, err := sess.SnapshotContext(AgentKindIDE)
	if err != nil {
		t.Fatal(err)
	}
	messages := conversation.modelHistory(snapshot)
	if len(messages) != 3 || messages[1].Role != agent.Assistant || len(messages[1].ToolCalls) != 1 ||
		messages[2].Role != agent.ToolRole || messages[2].ToolCallID != "canonical-call" ||
		!isUnknownToolEffectResult(messages[2].Content) {
		t.Fatalf("canonical next model context did not contain a complete unknown-effect exchange: %#v", messages)
	}
}
