package engine

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/alfredxw/denova/agent/context/history"
	"github.com/alfredxw/denova/agent/schema"
)

func TestCanonicalCheckpointDoesNotRepeatCommittedBodies(t *testing.T) {
	state := engineTranscript{Version: 2, Archive: &history.HistoryArchive{From: 0, To: 2}}
	for range 40 {
		state.Messages = append(state.Messages, schema.UserMessage("read"), schema.AssistantMessage(strings.Repeat("chapter", 4096), nil))
	}
	state.Messages = append(state.Messages, schema.AssistantMessage("", []schema.ToolCall{
		{ID: "pending-a", Function: schema.FunctionCall{Name: "read", Arguments: `{}`}},
		{ID: "pending-b", Function: schema.FunctionCall{Name: "read", Arguments: `{}`}},
	}), schema.ToolMessage(schema.TextToolResult("accepted result"), "pending-a"))
	encoded, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	checkpoint, err := CanonicalMessageCheckpoint(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if len(checkpoint.Metadata) > 1024 || strings.Contains(string(checkpoint.Metadata), "chapter") {
		t.Fatalf("checkpoint repeats committed bodies: %d bytes", len(checkpoint.Metadata))
	}
	if checkpoint.MessageCount != 82 || len(checkpoint.Pending) != 2 || checkpoint.Pending[1].Content != "accepted result" {
		t.Fatalf("checkpoint lost the incomplete batch: %+v", checkpoint)
	}
}
