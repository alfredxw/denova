package agentui

import (
	"bytes"
	agentrun "denova/internal/agents/run"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestStreamEncoderMapsAgentEventsToUIStream(t *testing.T) {
	var out bytes.Buffer
	encoder := NewStreamEncoder(&out, "0198-stream-request")

	events := []agentrun.Event{
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
		{Type: "tool_call", Data: map[string]any{"id": "tool-1", "name": "read", "args": `{"path"`, "tool_presentation": map[string]any{"call": "search", "result": "search"}}},
		{Type: "tool_args_delta", Data: map[string]any{"id": "tool-1", "delta": `:"a.md"}`}},
		{Type: "tool_started", Data: map[string]any{"id": "tool-1", "name": "read"}},
		{Type: "tool_result", Data: map[string]any{"id": "tool-1", "name": "read", "content": "ok"}},
		{Type: "workspace_change", Data: map[string]any{
			"id":              "tool-change-1",
			"change_group_id": "run-1",
			"change_set_id":   "change-1",
			"path":            "chapters/ch01.md",
			"affected_paths":  []string{"chapters/ch01.md"},
		}},
		{Type: "context_compaction", Data: map[string]any{"id": "ctx-1", "content": "压缩完成"}},
		{Type: "token_usage", Data: map[string]any{"id": "usage-1", "total_tokens": 42}},
		{Type: "execution_summary", Data: map[string]any{"run_id": "run-1", "run_started_at": "2026-07-08T12:00:00Z", "run_finished_at": "2026-07-08T12:01:01Z", "duration_ms": 61_000, "status": "completed"}},
		{Type: "ask_pending", Data: map[string]any{"schema": "ask.pending.v1", "id": "ask-1", "tool_call_id": "ask-1", "status": "pending", "questions": []map[string]any{{"id": "q1", "question": "选择方向"}}}},
		{Type: "proposed_plan", Data: map[string]any{"id": "plan-1", "content": "执行计划"}},
		{Type: "rule_roll", Data: map[string]any{"id": "roll-1", "rule_roll": map[string]any{"label": "检定"}}},
		{Type: "tool_result", Data: map[string]any{
			"id":                "tool-2",
			"name":              "generate_interactive_image",
			"content":           `{"schema":"interactive_image.v1"}`,
			"tool_presentation": map[string]any{"call": "image", "result": "interactive_media"},
			"interactive_image": map[string]any{
				"schema":     "interactive_image.v1",
				"image_path": "assets/interactive/images/scene.png",
			},
		}},
		{Type: "error", Data: map[string]any{"message": "失败"}},
		{Type: "aborted", Data: map[string]any{"reason": "取消"}},
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
		DataTypeExecutionSummary,
		DataTypeAsk,
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
	assertChunkAgentPresentation(t, chunks, "tool-output-available", "image", "interactive_media")
	assertDataChunkValue(t, chunks, DataTypeInteractiveImage, "tool_presentation", map[string]any{"call": "image", "result": "interactive_media"})
	assertChunk(t, chunks, DataTypeWorkspaceChange, "id", "tool-change-1")
	assertChunk(t, chunks, DataTypeRuleRoll, "id", "roll-1")
	assertChunk(t, chunks, DataTypeAsk, "id", "ask-1")
	assertDataChunkValue(t, chunks, DataTypeExecutionSummary, "duration_ms", float64(61_000))
	assertChunk(t, chunks, "tool-input-start", "toolName", "read")
	assertChunkAgentPresentation(t, chunks, "tool-input-start", "search", "search")
	assertChunk(t, chunks, "tool-input-available", "toolCallId", "tool-1")
	assertStreamingToolInput(t, chunks, "tool-1", `{"path":"a.md"}`)
	assertChunk(t, chunks, "error", "errorText", "失败 · 日志 ID / Log ID: 0198-stream-request")
	assertChunk(t, chunks, "abort", "reason", "取消")
	assertStartMetadata(t, chunks[0])
	assertDataChunksHaveStrictShape(t, chunks)
}

func TestStreamEncoderFinishesCleanlyWhenUserPauses(t *testing.T) {
	var out bytes.Buffer
	encoder := NewStreamEncoder(&out, "")

	if err := encoder.WriteEvent(agentrun.Event{
		Type: "aborted",
		Data: map[string]any{"reason": agentrun.AbortReasonUserRequested},
	}); err != nil {
		t.Fatal(err)
	}

	chunks, done := parseUIStreamChunks(t, out.String())
	if !done {
		t.Fatalf("user-requested pause did not finish the stream: %s", out.String())
	}
	if got := chunkTypes(chunks); !reflect.DeepEqual(got, []string{"start", "finish"}) {
		t.Fatalf("chunk types = %v, want a clean finish without an abort chunk", got)
	}
}

func TestStreamEncoderKeepsSubAgentActivityIDsDistinct(t *testing.T) {
	var out bytes.Buffer
	encoder := NewStreamEncoder(&out, "")
	for _, sessionID := range []string{"child-a", "child-b"} {
		if err := encoder.WriteEvent(agentrun.Event{Type: "subagent_settled", Data: map[string]any{
			"run_id": "parent-run", "subagent": true, "subagent_session_id": sessionID, "status": "completed",
		}}); err != nil {
			t.Fatal(err)
		}
	}
	if err := encoder.WriteEvent(agentrun.Event{Type: "done", Data: map[string]any{}}); err != nil {
		t.Fatal(err)
	}

	chunks, done := parseUIStreamChunks(t, out.String())
	if !done {
		t.Fatal("stream did not finish")
	}
	assertChunk(t, chunks, DataTypeActivity, "id", "subagent_settled-parent-run-child-a")
	assertChunk(t, chunks, DataTypeActivity, "id", "subagent_settled-parent-run-child-b")
}

func assertChunkAgentPresentation(t *testing.T, chunks []map[string]any, chunkType, call, result string) {
	t.Helper()
	for _, chunk := range chunks {
		if chunk["type"] != chunkType {
			continue
		}
		provider, _ := chunk["providerMetadata"].(map[string]any)
		agentMeta, _ := provider["agent"].(map[string]any)
		presentation, _ := agentMeta["tool_presentation"].(map[string]any)
		if presentation["call"] == call && presentation["result"] == result {
			return
		}
	}
	t.Fatalf("missing chunk type=%s tool presentation call=%s result=%s in %#v", chunkType, call, result, chunks)
}

func assertDataChunkValue(t *testing.T, chunks []map[string]any, chunkType, key string, want any) {
	t.Helper()
	for _, chunk := range chunks {
		if chunk["type"] != chunkType {
			continue
		}
		data, _ := chunk["data"].(map[string]any)
		if reflect.DeepEqual(data[key], want) {
			return
		}
	}
	t.Fatalf("missing chunk type=%s data.%s=%v in %#v", chunkType, key, want, chunks)
}

func assertDataChunksHaveStrictShape(t *testing.T, chunks []map[string]any) {
	t.Helper()
	for _, chunk := range chunks {
		chunkType, _ := chunk["type"].(string)
		if !strings.HasPrefix(chunkType, "data-") {
			continue
		}
		if _, present := chunk["providerMetadata"]; present {
			t.Fatalf("AI SDK data chunk contains unsupported providerMetadata: %#v", chunk)
		}
	}
}

func TestStreamEncoderUsesPersistedDisplaySegmentIDs(t *testing.T) {
	var out bytes.Buffer
	encoder := NewStreamEncoder(&out, "")
	for _, event := range []agentrun.Event{
		{Type: "chunk", Data: map[string]any{"content": "第一", "run_id": "run-order", "display_segment_id": "run-order-display-001-assistant", "display_phase": "candidate"}},
		{Type: "chunk", Data: map[string]any{"content": "段", "run_id": "run-order", "display_segment_id": "run-order-display-001-assistant"}},
		{Type: "thinking", Data: map[string]any{"content": "检查", "run_id": "run-order", "display_segment_id": "run-order-display-002-thinking"}},
		{Type: "done", Data: map[string]any{}},
	} {
		if err := encoder.WriteEvent(event); err != nil {
			t.Fatal(err)
		}
	}

	chunks, _ := parseUIStreamChunks(t, out.String())
	assertChunk(t, chunks, "text-start", "id", "run-order-display-001-assistant")
	assertChunk(t, chunks, "reasoning-start", "id", "run-order-display-002-thinking")
	assertChunkAgentMetadata(t, chunks, "text-start", "display_segment_id", "run-order-display-001-assistant")
	assertChunkAgentMetadata(t, chunks, "text-start", "display_phase", "candidate")
	assertChunkAgentMetadata(t, chunks, "reasoning-start", "display_segment_id", "run-order-display-002-thinking")
}

func TestStreamEncoderPreservesFailedAndIncompleteToolStates(t *testing.T) {
	var out bytes.Buffer
	encoder := NewStreamEncoder(&out, "")

	failedIDs := []string{"failed-tool-1", "failed-tool-2", "failed-tool-3"}
	for index, status := range []string{"error", "blocked", "skipped"} {
		id := failedIDs[index]
		if err := encoder.WriteEvent(agentrun.Event{Type: "tool_call", Data: map[string]any{
			"id": id, "name": "todo", "args": `{"plan":[]}`,
		}}); err != nil {
			t.Fatal(err)
		}
		if err := encoder.WriteEvent(agentrun.Event{Type: "tool_result", Data: map[string]any{
			"id": id, "name": "todo", "status": status, "content": "did not apply",
		}}); err != nil {
			t.Fatal(err)
		}
	}
	if err := encoder.WriteEvent(agentrun.Event{Type: "tool_call", Data: map[string]any{
		"id": "unfinished-tool", "name": "read", "args": `{"path":"draft.md"}`,
	}}); err != nil {
		t.Fatal(err)
	}
	if err := encoder.Finish("stop"); err != nil {
		t.Fatal(err)
	}

	chunks, done := parseUIStreamChunks(t, out.String())
	if !done {
		t.Fatal("stream did not finish")
	}
	errorChunks := 0
	for _, chunk := range chunks {
		if chunk["type"] != "tool-output-error" {
			continue
		}
		errorChunks++
		errorText, _ := chunk["errorText"].(string)
		if strings.TrimSpace(errorText) == "" {
			t.Fatalf("tool error chunk omitted errorText: %#v", chunk)
		}
	}
	if errorChunks != 4 {
		t.Fatalf("tool-output-error chunks = %d, want 4: %#v", errorChunks, chunks)
	}
	for _, id := range []string{"failed-tool-1", "failed-tool-2", "failed-tool-3", "unfinished-tool"} {
		assertChunk(t, chunks, "tool-output-error", "toolCallId", id)
	}
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

func assertChunkAgentMetadata(t *testing.T, chunks []map[string]any, chunkType, key, value string) {
	t.Helper()
	for _, chunk := range chunks {
		if chunk["type"] != chunkType {
			continue
		}
		provider, _ := chunk["providerMetadata"].(map[string]any)
		agentMeta, _ := provider["agent"].(map[string]any)
		if agentMeta[key] == value {
			return
		}
	}
	t.Fatalf("missing chunk type=%s providerMetadata.agent.%s=%s in %#v", chunkType, key, value, chunks)
}

func assertStreamingToolInput(t *testing.T, chunks []map[string]any, toolCallID, want string) {
	t.Helper()
	var got strings.Builder
	for _, chunk := range chunks {
		if chunk["type"] != "tool-input-delta" || chunk["toolCallId"] != toolCallID {
			continue
		}
		delta, _ := chunk["inputTextDelta"].(string)
		got.WriteString(delta)
	}
	if got.String() != want {
		t.Fatalf("streaming tool input %q = %q, want %q in %#v", toolCallID, got.String(), want, chunks)
	}
	for _, chunk := range chunks {
		if chunk["type"] != "tool-input-available" || chunk["toolCallId"] != toolCallID {
			continue
		}
		input, _ := chunk["input"].(map[string]any)
		if input["path"] == "a.md" {
			return
		}
		t.Fatalf("available tool input %q = %#v, want parsed path", toolCallID, chunk["input"])
	}
	t.Fatalf("missing available tool input for %q in %#v", toolCallID, chunks)
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
