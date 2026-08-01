package sse

import (
	"bytes"
	"context"
	agentrun "denova/internal/agents/run"
	"strings"
	"testing"

	apptask "denova/internal/app/task"
	"denova/internal/observability"
)

func TestSSEWriteHandlerKeepsChapterBodyByDefault(t *testing.T) {
	var buf bytes.Buffer
	writeSSE := newSSEWriteHandler(context.Background(), &buf)
	writeChapterBodySSEEvents(t, writeSSE)

	got := buf.String()
	if !strings.Contains(got, "第一行") || !strings.Contains(got, "第二行") {
		t.Fatalf("default SSE output should preserve chapter body, got %q", got)
	}
	if !strings.Contains(got, "id: 1\n") || !strings.Contains(got, "id: 4\n") {
		t.Fatalf("SSE output should include monotonic display cursors, got %q", got)
	}
	if strings.Contains(got, `"sse_display_notice":"chapter_body_hidden"`) {
		t.Fatalf("default SSE output should not include hidden body notice, got %q", got)
	}
}

func TestSSEWriteHandlerAppliesMiddlewareChainBeforeWriteWhenEnabled(t *testing.T) {
	var buf bytes.Buffer
	writeSSE := newSSEWriteHandler(context.Background(), &buf, WithHideChapterBodyLiveOutput(true))
	writeChapterBodySSEEvents(t, writeSSE)

	got := buf.String()
	if !strings.Contains(got, `"delta":"{\"path\":\"chapters/ch02.md\"}"`) {
		t.Fatalf("filtered SSE output should include path-only delta, got %q", got)
	}
	if strings.Contains(got, "第一行") || strings.Contains(got, "第二行") || strings.Contains(got, `\"content\"`) || strings.Contains(got, "...") {
		t.Fatalf("filtered SSE output should not include chapter body or placeholder, got %q", got)
	}
	if !strings.Contains(got, `"sse_display_notice":"chapter_body_hidden"`) || !strings.Contains(got, `"sse_hidden_fields":["content"]`) {
		t.Fatalf("filtered SSE output should include hidden body notice, got %q", got)
	}
	if !strings.Contains(got, `"sse_generated_chars":3`) || !strings.Contains(got, `"sse_generated_chars":7`) {
		t.Fatalf("filtered SSE output should include generated character progress, got %q", got)
	}
	if count := strings.Count(got, "event: tool_args_delta"); count != 2 {
		t.Fatalf("tool_args_delta events = %d, want 2; output=%q", count, got)
	}
}

func TestUIWriteHandlerUsesFullReplayProtocolWithoutMisleadingEventCursor(t *testing.T) {
	var buf bytes.Buffer
	handler := newUIWriteHandler(context.Background(), &buf)
	if err := handler.Handle(apptask.Event{Cursor: 9, Event: agentrun.Event{
		Type: "chunk", Data: map[string]any{"content": "继续"},
	}}); err != nil {
		t.Fatal(err)
	}
	if got := buf.String(); strings.Contains(got, "id: 9") || !strings.Contains(got, `"type":"text-delta"`) {
		t.Fatalf("UI stream must rely on full replay rather than event-level suffix IDs, got %q", got)
	}
}

func TestTaskCheckpointCommitsCursorOnlyAfterCompleteLegacyReplay(t *testing.T) {
	checkpoint := apptask.DisplayCheckpoint{
		Version: 1, Cursor: 19, Complete: true,
		Events: []agentrun.Event{
			{Type: "thinking", Data: map[string]any{"content": "完整思考"}},
			{Type: "chunk", Data: map[string]any{"content": "完整正文"}},
		},
	}
	var buf bytes.Buffer
	committed, err := writeTaskCheckpoint(&buf, checkpoint, newSSEWriteHandler(context.Background(), &buf))
	if err != nil {
		t.Fatal(err)
	}
	if !committed {
		t.Fatal("complete checkpoint was not committed")
	}
	got := buf.String()
	start := strings.Index(got, "event: task_checkpoint")
	thinking := strings.Index(got, "event: thinking")
	content := strings.Index(got, "event: chunk")
	commit := strings.Index(got, "id: 19\nevent: task_checkpoint_committed")
	if start < 0 || thinking <= start || content <= thinking || commit <= content {
		t.Fatalf("checkpoint frame order = %q", got)
	}
	if strings.Contains(got[:commit], "id: 0") {
		t.Fatalf("checkpoint envelope must not overwrite Last-Event-ID before commit: %q", got)
	}
}

func TestIncompleteTaskCheckpointRequiresRehydrateWithoutCursorCommitOrReplay(t *testing.T) {
	checkpoint := apptask.DisplayCheckpoint{
		Version: 1, Cursor: 41, Complete: false, PersistenceRequired: true,
		Events: []agentrun.Event{
			{Type: "agent_cycle_started", Data: map[string]any{"operation_id": "operation-1"}},
			{Type: "thinking", Data: map[string]any{"content": "must-not-look-complete"}},
		},
	}
	var buf bytes.Buffer
	committed, err := writeTaskCheckpoint(&buf, checkpoint, newSSEWriteHandler(context.Background(), &buf))
	if err != nil {
		t.Fatal(err)
	}
	if committed {
		t.Fatal("incomplete checkpoint advanced the reconnect cursor")
	}
	got := buf.String()
	if !strings.Contains(got, "event: task_rehydrate_required") || !strings.Contains(got, "agent_stream.rehydrate_required") {
		t.Fatalf("incomplete checkpoint did not request canonical rehydrate: %q", got)
	}
	if !strings.Contains(got, `"persistence_required":true`) {
		t.Fatalf("incomplete Agent-cycle checkpoint lost its persistence barrier: %q", got)
	}
	if strings.Contains(got, "task_checkpoint_committed") || strings.Contains(got, "id: 0") || strings.Contains(got, "id: 41") || strings.Contains(got, "must-not-look-complete") {
		t.Fatalf("incomplete checkpoint replayed or committed omitted output: %q", got)
	}
}

func TestWritingUICheckpointUsesCompleteProjectionOrExplicitRehydrateError(t *testing.T) {
	t.Run("complete", func(t *testing.T) {
		var buf bytes.Buffer
		committed, err := writeUITaskCheckpoint(newUIWriteHandler(context.Background(), &buf), apptask.DisplayCheckpoint{
			Version: 1, Cursor: 7, Complete: true,
			Events: []agentrun.Event{
				{Type: "thinking", Data: map[string]any{"content": "完整思考"}},
				{Type: "chunk", Data: map[string]any{"content": "完整正文"}},
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		got := buf.String()
		if !committed || !strings.Contains(got, "完整思考") || !strings.Contains(got, "完整正文") {
			t.Fatalf("Writing checkpoint replay = committed:%t output:%q", committed, got)
		}
	})

	t.Run("incomplete", func(t *testing.T) {
		var buf bytes.Buffer
		committed, err := writeUITaskCheckpoint(newUIWriteHandler(context.Background(), &buf), apptask.DisplayCheckpoint{
			Version: 1, TaskID: "writing-task-8", Cursor: 8, Complete: false,
			Events: []agentrun.Event{{Type: "thinking", Data: map[string]any{"content": "partial-thinking"}}},
		})
		if err != nil {
			t.Fatal(err)
		}
		got := buf.String()
		if committed || !strings.Contains(got, "Display history exceeded the recovery budget") ||
			!strings.Contains(got, `"event":"task_rehydrate_required"`) ||
			!strings.Contains(got, `"code":"agent_stream.rehydrate_required"`) ||
			!strings.Contains(got, `"task_id":"writing-task-8"`) {
			t.Fatalf("Writing incomplete checkpoint = committed:%t output:%q", committed, got)
		}
		if strings.Contains(got, "partial-thinking") {
			t.Fatalf("Writing UI treated incomplete output as a replay: %q", got)
		}
	})
}

func TestSSEErrorIncludesRequestID(t *testing.T) {
	ctx := observability.WithRequestID(context.Background(), "0198-sse-request")
	var buf bytes.Buffer
	writeSSE := newSSEWriteHandler(ctx, &buf)
	if err := writeSSE(apptask.Event{Cursor: 12, Event: agentrun.Event{
		Type: "error",
		Data: map[string]any{"message": "生成失败"},
	}}); err != nil {
		t.Fatal(err)
	}

	got := buf.String()
	if !strings.Contains(got, `"request_id":"0198-sse-request"`) {
		t.Fatalf("SSE error omitted request_id: %q", got)
	}
	if !strings.Contains(got, "生成失败 · 日志 ID / Log ID: 0198-sse-request") {
		t.Fatalf("SSE error omitted user-visible Log ID: %q", got)
	}
}

func TestCheckpointRehydratePayloadUsesIndependentPersistenceBarrier(t *testing.T) {
	data := taskRehydrateRequiredData(apptask.DisplayCheckpoint{
		Settled:                 true,
		Status:                  apptask.Failed,
		TerminalReason:          "provider failed after acceptance",
		TerminalReasonTruncated: true,
		PersistenceRequired:     true,
	})
	if data["persistence_required"] != true {
		t.Fatalf("rehydrate payload persistence barrier = %#v", data["persistence_required"])
	}
	if data["status"] != apptask.Failed || data["terminal_reason"] != "provider failed after acceptance" || data["terminal_reason_truncated"] != true {
		t.Fatalf("rehydrate payload terminal outcome = %#v", data)
	}
}

func writeChapterBodySSEEvents(t *testing.T, writeSSE func(apptask.Event) error) {
	t.Helper()
	write := func(cursor uint64, event agentrun.Event) error {
		return writeSSE(apptask.Event{Cursor: cursor, Event: event})
	}
	if err := write(1, agentrun.Event{Type: "tool_call", Data: map[string]interface{}{
		"agent_kind": agentrun.AgentKindIDE,
		"id":         "call-1",
		"name":       "write",
		"args":       "",
	}}); err != nil {
		t.Fatalf("write tool_call failed: %v", err)
	}
	if err := write(2, agentrun.Event{Type: "tool_args_delta", Data: map[string]interface{}{
		"agent_kind": agentrun.AgentKindIDE,
		"id":         "call-1",
		"name":       "write",
		"delta":      `{"path":"chapters/ch02.md","content":"第一行`,
	}}); err != nil {
		t.Fatalf("write first delta failed: %v", err)
	}
	if err := write(3, agentrun.Event{Type: "tool_args_delta", Data: map[string]interface{}{
		"agent_kind": agentrun.AgentKindIDE,
		"id":         "call-1",
		"name":       "write",
		"delta":      `\n第二行"}`,
	}}); err != nil {
		t.Fatalf("write suppressed delta failed: %v", err)
	}
	if err := write(4, agentrun.Event{Type: "tool_result", Data: map[string]interface{}{
		"id":      "call-1",
		"name":    "write",
		"content": "Updated file chapters/ch02.md",
	}}); err != nil {
		t.Fatalf("write tool_result failed: %v", err)
	}
}
