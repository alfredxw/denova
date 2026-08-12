package agentui

import (
	"testing"
	"time"

	agentcontext "denova/internal/agents/context"
	"denova/internal/agents/session"
)

func TestMessagesFromHistoryConvertsLegacyEntries(t *testing.T) {
	createdAt := time.Date(2026, 7, 8, 12, 0, 0, 0, time.UTC)
	entries := []session.HistoryEntry{
		{ID: "user-1", Role: "user", Content: "你好", CreatedAt: createdAt, UserReferences: []agentcontext.UserReference{{Kind: "file", Label: "chapters/ch01.md"}}},
		{ID: "assistant-1", Role: "assistant", Content: "回复", RunID: "run-1"},
		{ID: "thinking-1", Role: "thinking", Content: "思考"},
		{ID: "tool-1", Role: "tool_call", Name: "read", Args: `{"path":"a.md"}`, Status: "success", Result: "ok"},
		{ID: "tool-result-1", Role: "tool_result", Name: "read", Content: "ok"},
		{ID: "ctx-1", Role: "context_compaction", Content: "压缩"},
		{ID: "usage-1", Role: "token_usage", Content: "用量", TotalTokens: 12},
		{ID: "plan-1", Role: "proposed_plan", Content: "计划"},
		{ID: "roll-1", Role: "rule_roll", Content: "检定"},
		{ID: "image-1", Role: "interactive_image", Content: "图像"},
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
	assertMessagePartType(t, messages[7], "assistant", DataTypeProposedPlan)
	assertMessagePartType(t, messages[8], "assistant", DataTypeRuleRoll)
	assertMessagePartType(t, messages[9], "assistant", DataTypeInteractiveImage)
	assertMessagePartType(t, messages[10], "assistant", DataTypeSystem)
	assertMessagePartType(t, messages[11], "assistant", DataTypeError)
	assertMessagePartType(t, messages[12], "assistant", DataTypeClear)

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
