package sse

import (
	"bytes"
	"strings"
	"testing"

	"nova/internal/agent"
)

func TestSSEWriteHandlerKeepsChapterBodyByDefault(t *testing.T) {
	var buf bytes.Buffer
	writeSSE := newSSEWriteHandler(&buf)
	writeChapterBodySSEEvents(t, writeSSE)

	got := buf.String()
	if !strings.Contains(got, "第一行") || !strings.Contains(got, "第二行") {
		t.Fatalf("default SSE output should preserve chapter body, got %q", got)
	}
	if strings.Contains(got, `"sse_display_notice":"chapter_body_hidden"`) {
		t.Fatalf("default SSE output should not include hidden body notice, got %q", got)
	}
}

func TestSSEWriteHandlerAppliesMiddlewareChainBeforeWriteWhenEnabled(t *testing.T) {
	var buf bytes.Buffer
	writeSSE := newSSEWriteHandler(&buf, WithHideChapterBodyLiveOutput(true))
	writeChapterBodySSEEvents(t, writeSSE)

	got := buf.String()
	if !strings.Contains(got, `"delta":"{\"file_path\":\"chapters/ch02.md\"}"`) {
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

func writeChapterBodySSEEvents(t *testing.T, writeSSE func(agent.Event) error) {
	t.Helper()
	if err := writeSSE(agent.Event{Type: "tool_call", Data: map[string]interface{}{
		"agent_kind": agent.AgentKindIDE,
		"id":         "call-1",
		"name":       "write_file",
		"args":       "",
	}}); err != nil {
		t.Fatalf("write tool_call failed: %v", err)
	}
	if err := writeSSE(agent.Event{Type: "tool_args_delta", Data: map[string]interface{}{
		"agent_kind": agent.AgentKindIDE,
		"id":         "call-1",
		"name":       "write_file",
		"delta":      `{"file_path":"chapters/ch02.md","content":"第一行`,
	}}); err != nil {
		t.Fatalf("write first delta failed: %v", err)
	}
	if err := writeSSE(agent.Event{Type: "tool_args_delta", Data: map[string]interface{}{
		"agent_kind": agent.AgentKindIDE,
		"id":         "call-1",
		"name":       "write_file",
		"delta":      `\n第二行"}`,
	}}); err != nil {
		t.Fatalf("write suppressed delta failed: %v", err)
	}
	if err := writeSSE(agent.Event{Type: "tool_result", Data: map[string]interface{}{
		"id":      "call-1",
		"name":    "write_file",
		"content": "Updated file chapters/ch02.md",
	}}); err != nil {
		t.Fatalf("write tool_result failed: %v", err)
	}
}
