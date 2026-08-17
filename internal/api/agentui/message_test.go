package agentui

import (
	"testing"
	"time"

	agent "github.com/alfredxw/denova/agent"

	agentcontext "denova/internal/agents/context"
	"denova/internal/agents/session"
)

func TestMessagesFromHistoryConvertsLegacyEntries(t *testing.T) {
	createdAt := time.Date(2026, 7, 8, 12, 0, 0, 0, time.UTC)
	presentation := agent.ToolPresentation{
		Call:   agent.ToolPresentationImage,
		Result: agent.ToolPresentationInteractiveMedia,
	}
	entries := []session.HistoryEntry{
		{ID: "user-1", Role: "user", Content: "你好", CreatedAt: createdAt, UserReferences: []agentcontext.UserReference{{Kind: "file", Label: "chapters/ch01.md"}}},
		{ID: "assistant-1", Role: "assistant", Content: "回复", RunID: "run-1"},
		{ID: "thinking-1", Role: "thinking", Content: "思考"},
		{ID: "tool-1", Role: "tool_call", Name: "read", Args: `{"path":"a.md"}`, Status: "success", Result: "ok", ToolPresentation: &presentation},
		{ID: "tool-result-1", Role: "tool_result", Name: "read", Content: "ok"},
		{ID: "ctx-1", Role: "context_compaction", Content: "压缩"},
		{ID: "usage-1", Role: "token_usage", Content: "用量", TotalTokens: 12},
		{ID: "run-1", Role: "execution_summary", RunID: "run-1", RunStartedAt: "2026-07-08T12:00:00Z", RunFinishedAt: "2026-07-08T12:01:01Z", DurationMS: 61_000, RunStatus: "completed"},
		{ID: "plan-1", Role: "proposed_plan", Content: "计划"},
		{ID: "roll-1", Role: "rule_roll", Content: "检定"},
		{ID: "image-1", Role: "interactive_image", Content: "图像"},
		{ID: "ask-1", Role: "ask", Ask: &session.AskInteraction{
			Schema: "ask.pending.v1", ID: "ask-1", ToolCallID: "ask-1", AgentKind: "ide", Status: session.AskAnswered,
			Questions: []session.AskQuestion{{ID: "direction", Question: "选择方向？"}},
			Answers: []session.AskAnswerResult{{
				QuestionID: "direction", Question: "选择方向？",
				SelectedOptions: []session.AskSelectedOption{{ID: "continue", Label: "继续"}},
			}},
		}},
		{ID: "system-1", Role: "system", Content: "系统"},
		{ID: "error-1", Role: "error", Content: "错误"},
		{ID: "clear-1", Type: "clear", CreatedAt: createdAt},
	}

	messages := MessagesFromHistory(entries)
	if len(messages) != len(entries) {
		t.Fatalf("expected %d messages, got %d", len(entries), len(messages))
	}

	assertMessagePartType(t, messages[0], "user", "text")
	assertMessagePartType(t, messages[1], "assistant", "text")
	assertMessagePartType(t, messages[2], "assistant", "reasoning")
	assertMessagePartType(t, messages[3], "assistant", "dynamic-tool")
	assertMessagePartType(t, messages[4], "assistant", DataTypeToolResult)
	assertMessagePartType(t, messages[5], "assistant", DataTypeContextCompaction)
	assertMessagePartType(t, messages[6], "assistant", DataTypeTokenUsage)
	assertMessagePartType(t, messages[7], "assistant", DataTypeExecutionSummary)
	assertMessagePartType(t, messages[8], "assistant", DataTypeProposedPlan)
	assertMessagePartType(t, messages[9], "assistant", DataTypeRuleRoll)
	assertMessagePartType(t, messages[10], "assistant", DataTypeInteractiveImage)
	assertMessagePartType(t, messages[11], "assistant", DataTypeAsk)
	assertMessagePartType(t, messages[12], "assistant", DataTypeSystem)
	assertMessagePartType(t, messages[13], "assistant", DataTypeError)
	assertMessagePartType(t, messages[14], "assistant", DataTypeClear)

	if messages[1].Metadata["run_id"] != "run-1" {
		t.Fatalf("expected run metadata to be preserved, got %#v", messages[1].Metadata)
	}
	userReferences, ok := messages[0].Metadata["user_references"].([]agentcontext.UserReference)
	if !ok || len(userReferences) != 1 || userReferences[0].Label != "chapters/ch01.md" {
		t.Fatalf("expected user reference metadata to be preserved, got %#v", messages[0].Metadata)
	}
	if messages[6].Parts[0]["data"].(map[string]any)["total_tokens"] != 12 {
		t.Fatalf("expected token usage payload, got %#v", messages[6].Parts[0]["data"])
	}
	ask, ok := messages[11].Parts[0]["data"].(*session.AskInteraction)
	if !ok || ask.Status != session.AskAnswered || len(ask.Answers) != 1 || ask.Answers[0].SelectedOptions[0].Label != "继续" {
		t.Fatalf("Ask history payload = %#v", messages[11].Parts[0]["data"])
	}
	executionSummary := messages[7].Parts[0]["data"].(map[string]any)
	if executionSummary["run_started_at"] != "2026-07-08T12:00:00Z" || executionSummary["duration_ms"] != int64(61_000) || executionSummary["run_status"] != "completed" {
		t.Fatalf("execution summary payload = %#v", executionSummary)
	}
	agentMetadata := messages[3].Metadata["tool_presentation"].(agent.ToolPresentation)
	if agentMetadata.Call != agent.ToolPresentationImage || agentMetadata.Result != agent.ToolPresentationInteractiveMedia {
		t.Fatalf("tool presentation metadata = %#v", agentMetadata)
	}
}

func TestMessagesFromHistoryPreservesDisplaySegmentIDsInTextMetadata(t *testing.T) {
	messages := MessagesFromHistory([]session.HistoryEntry{
		{ID: "run-1-display-001-thinking", DisplaySegmentID: "run-1-display-001-thinking", Role: "thinking", Content: "分析", RunID: "run-1"},
		{ID: "run-1-display-002-assistant", DisplaySegmentID: "run-1-display-002-assistant", DisplayPhase: session.DisplayPhaseFinal, Role: "assistant", Content: "正文", RunID: "run-1"},
	})

	want := []string{"run-1-display-001-thinking", "run-1-display-002-assistant"}
	if len(messages) != len(want) {
		t.Fatalf("messages = %#v", messages)
	}
	for index, id := range want {
		if messages[index].Metadata["display_segment_id"] != id {
			t.Fatalf("message %d metadata = %#v, want segment id %q", index, messages[index].Metadata, id)
		}
	}
	if messages[1].Metadata["display_phase"] != session.DisplayPhaseFinal {
		t.Fatalf("final display phase was not preserved: %#v", messages[1].Metadata)
	}
}

func TestMessagesFromHistoryDoesNotTreatCanonicalMessageIDAsDisplaySegmentID(t *testing.T) {
	messages := MessagesFromHistory([]session.HistoryEntry{
		{Type: "message", ID: "canonical-message-1", Role: "assistant", Content: "完整回复", RunID: "run-1"},
	})
	if len(messages) != 1 {
		t.Fatalf("messages = %#v", messages)
	}
	if segmentID := messages[0].Metadata["display_segment_id"]; segmentID != nil {
		t.Fatalf("canonical message ID leaked into display segment identity: %#v", messages[0].Metadata)
	}
}

func assertMessagePartType(t *testing.T, message Message, role, partType string) {
	t.Helper()
	if message.Role != role {
		t.Fatalf("message %s role mismatch: want %s got %s", message.ID, role, message.Role)
	}
	if len(message.Parts) != 1 {
		t.Fatalf("message %s expected one part, got %#v", message.ID, message.Parts)
	}
	if message.Parts[0]["type"] != partType {
		t.Fatalf("message %s part type mismatch: want %s got %#v", message.ID, partType, message.Parts[0])
	}
}
