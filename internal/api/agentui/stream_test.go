package agentui

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"denova/internal/agent"
)

func TestStreamEncoderMapsAgentEventsToUIStream(t *testing.T) {
	var out bytes.Buffer
	encoder := NewStreamEncoder(&out)

	events := []agent.Event{
		{Type: "thinking", Data: map[string]any{
			"content":            "分析",
			"run_id":             "run-1",
			"created_at":         "2026-07-08T12:00:00Z",
			"display_role":       "thinking",
			"turn_id":            "turn-1",
			"navigation_turn_id": "turn-1",
			"turn_versions":      []map[string]any{{"turn_id": "turn-1", "ts": "2026-07-08T12:00:00Z", "current": true}},
			"turn_version_index": 0,
		}},
		{Type: "chunk", Data: map[string]any{"content": "正文", "run_id": "run-1"}},
		{Type: "tool_call", Data: map[string]any{"id": "tool-1", "name": "read_file", "args": `{"path"`}},
		{Type: "tool_args_delta", Data: map[string]any{"id": "tool-1", "delta": `:"a.md"}`}},
		{Type: "tool_result", Data: map[string]any{"id": "tool-1", "name": "read_file", "content": "ok"}},
		{Type: "workspace_change", Data: map[string]any{
			"id":              "tool-change-1",
			"change_group_id": "run-1",
			"change_set_id":   "change-1",
			"path":            "chapters/ch01.md",
			"affected_paths":  []string{"chapters/ch01.md"},
		}},
		{Type: "context_compaction", Data: map[string]any{"id": "ctx-1", "content": "压缩完成"}},
		{Type: "token_usage", Data: map[string]any{"id": "usage-1", "total_tokens": 42}},
		{Type: "plan_question", Data: map[string]any{"id": "question-1", "content": "选择方向"}},
		{Type: "proposed_plan", Data: map[string]any{"id": "plan-1", "content": "执行计划"}},
		{Type: "rule_roll", Data: map[string]any{"id": "roll-1", "rule_roll": map[string]any{"label": "检定"}}},
		{Type: "tool_result", Data: map[string]any{
			"id":      "tool-2",
			"name":    "generate_interactive_image",
			"content": `{"schema":"interactive_image.v1"}`,
			"interactive_image": map[string]any{
				"schema":     "interactive_image.v1",
				"image_path": "assets/interactive/images/scene.png",
			},
		}},
		{Type: "error", Data: map[string]any{"message": "失败"}},
		{Type: "aborted", Data: map[string]any{"message": "取消"}},
		{Type: "done", Data: map[string]any{}},
	}

	for _, event := range events {
		if err := encoder.WriteEvent(event); err != nil {
			t.Fatalf("WriteEvent(%s) failed: %v", event.Type, err)
		}
	}

	chunks, done := parseUIStreamChunks(t, out.String())
	if !done {
		t.Fatalf("expected [DONE] marker, got stream:\n%s", out.String())
	}

	expectedTypes := []string{
		"start",
		"reasoning-start",
		"reasoning-delta",
		"reasoning-end",
		"text-start",
		"text-delta",
		"text-end",
		"tool-input-start",
		"tool-input-delta",
		"tool-input-delta",
		"tool-input-available",
		"tool-output-available",
		DataTypeWorkspaceChange,
		DataTypeContextCompaction,
		DataTypeTokenUsage,
		DataTypePlanQuestion,
		DataTypeProposedPlan,
		DataTypeRuleRoll,
		"tool-input-start",
		"tool-input-available",
		"tool-output-available",
		DataTypeInteractiveImage,
		"error",
		"abort",
		"finish",
	}
	if got := chunkTypes(chunks); strings.Join(got, ",") != strings.Join(expectedTypes, ",") {
		t.Fatalf("chunk types mismatch\nwant: %v\n got: %v", expectedTypes, got)
	}

	assertChunk(t, chunks, DataTypeInteractiveImage, "id", "tool-2")
	assertChunk(t, chunks, DataTypeWorkspaceChange, "id", "tool-change-1")
	assertChunk(t, chunks, DataTypeRuleRoll, "id", "roll-1")
	assertChunk(t, chunks, "tool-input-available", "toolCallId", "tool-1")
	assertStartMetadata(t, chunks[0])
}

func parseUIStreamChunks(t *testing.T, raw string) ([]map[string]any, bool) {
	t.Helper()
	chunks := []map[string]any{}
	done := false
	for _, frame := range strings.Split(raw, "\n\n") {
		frame = strings.TrimSpace(frame)
		if frame == "" {
			continue
		}
		if !strings.HasPrefix(frame, "data: ") {
			t.Fatalf("unexpected frame %q", frame)
		}
		data := strings.TrimPrefix(frame, "data: ")
		if data == "[DONE]" {
			done = true
			continue
		}
		var chunk map[string]any
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			t.Fatalf("invalid json chunk %q: %v", data, err)
		}
		chunks = append(chunks, chunk)
	}
	return chunks, done
}

func chunkTypes(chunks []map[string]any) []string {
	types := make([]string, 0, len(chunks))
	for _, chunk := range chunks {
		types = append(types, chunk["type"].(string))
	}
	return types
}

func assertChunk(t *testing.T, chunks []map[string]any, chunkType, key, value string) {
	t.Helper()
	for _, chunk := range chunks {
		if chunk["type"] == chunkType && chunk[key] == value {
			return
		}
	}
	t.Fatalf("missing chunk type=%s %s=%s in %#v", chunkType, key, value, chunks)
}

func assertStartMetadata(t *testing.T, chunk map[string]any) {
	t.Helper()
	metadata, ok := chunk["messageMetadata"].(map[string]any)
	if !ok {
		t.Fatalf("expected start metadata, got %#v", chunk)
	}
	for key, want := range map[string]any{
		"run_id":             "run-1",
		"created_at":         "2026-07-08T12:00:00Z",
		"display_role":       "thinking",
		"turn_id":            "turn-1",
		"navigation_turn_id": "turn-1",
	} {
		if metadata[key] != want {
			t.Fatalf("metadata %s mismatch: want %v got %#v", key, want, metadata[key])
		}
	}
	if metadata["turn_version_index"] != float64(0) {
		t.Fatalf("expected turn_version_index metadata, got %#v", metadata)
	}
	if _, ok := metadata["turn_versions"].([]any); !ok {
		t.Fatalf("expected turn_versions metadata, got %#v", metadata["turn_versions"])
	}
}
