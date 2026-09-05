package agentui

import (
	"bytes"
	"testing"

	agentrun "denova/internal/agents/run"
	"denova/internal/agents/session"
)

func TestToolInspectionPreservesRecordedInputInHistoryAndStream(t *testing.T) {
	input := " {\"id\":9223372036854775807,\"id\":1,\"text\":\"\\u4e2d\"} "
	for _, status := range []string{"success", "error", "cancelled"} {
		t.Run(status, func(t *testing.T) {
			messages := MessagesFromHistory([]session.HistoryEntry{{
				ID: "call-1", Role: "tool_call", Name: "custom", Args: input,
				Status: status, Result: "raw output\n",
			}})
			metadata := messages[0].Parts[0]["toolMetadata"].(map[string]any)
			if metadata["input_text"] != input {
				t.Fatalf("history changed recorded input: %#v", metadata)
			}
		})
	}

	var output bytes.Buffer
	encoder := NewStreamEncoder(&output, "inspection-stream")
	for _, event := range []agentrun.Event{
		{Type: "tool_call", Data: map[string]any{"id": "call-1", "name": "custom", "args": input[:12]}},
		{Type: "tool_args_delta", Data: map[string]any{"id": "call-1", "delta": input[12:]}},
		{Type: "tool_started", Data: map[string]any{"id": "call-1", "name": "custom"}},
		{Type: "tool_result", Data: map[string]any{"id": "call-1", "name": "custom", "content": "raw output\n", "display_truncated": true}},
	} {
		if err := encoder.WriteEvent(event); err != nil {
			t.Fatal(err)
		}
	}
	chunks, _ := parseUIStreamChunks(t, output.String())
	seen := 0
	for _, chunk := range chunks {
		if chunk["type"] != "tool-input-available" && chunk["type"] != "tool-output-available" {
			continue
		}
		seen++
		metadata := chunk["toolMetadata"].(map[string]any)
		if metadata["input_text"] != input {
			t.Fatalf("stream changed recorded input: %#v", metadata)
		}
		if chunk["type"] == "tool-output-available" && (metadata["display_truncated"] != true || chunk["output"] != "raw output\n") {
			t.Fatalf("stream changed output or truncation: %#v", chunk)
		}
	}
	if seen != 2 {
		t.Fatalf("expected input and output chunks, got %d", seen)
	}
}
