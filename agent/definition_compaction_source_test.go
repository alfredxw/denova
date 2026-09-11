package agent

import (
	"reflect"
	"testing"
)

func TestTransientCompactionCleanupKeepsHistoricalCoordinates(t *testing.T) {
	history := []*Message{
		UserMessage("Old request"),
		AssistantMessage("", []ToolCall{{ID: "reused", Type: "function", Function: FunctionCall{Name: "read", Arguments: `{}`}}}),
		ToolMessage(TextToolResult("old body"), "reused", WithToolName("read")),
		AssistantMessage("Old answer", nil),
	}
	loop := append([]*Message{SystemMessage("Stable instructions")}, cloneMessages(history)...)
	loop = append(loop, UserMessage("Current request"))
	initialLoopMessages := len(loop)
	loop = append(loop, history[1].Clone(), ToolMessage(TextToolResult("active body"), "reused", WithToolName("read")))
	raw := cloneMessages(loop[1:])
	visible := append([]*Message{SystemMessage("Middleware prefix")}, cloneMessages(loop)...)
	before := cloneMessages(visible)
	projected, err := projectCompactionCleanup(history, visible, loop, raw, initialLoopMessages, []CleanupReplacement{
		{MessageIndex: 4, ToolCallID: "reused", Placeholder: "[Old result removed.]"},
		{MessageIndex: len(visible) - 1, ToolCallID: "reused", Placeholder: "[Active result removed.]"},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := cloneMessages(history)
	want[2] = cleanupProjectedMessage(want[2], "[Old result removed.]")
	if !reflect.DeepEqual(projected, want) {
		t.Fatalf("source cleanup did not retain raw history coordinates: %#v", projected)
	}
	if !reflect.DeepEqual(visible, before) || history[2].Content != "old body" {
		t.Fatal("source cleanup mutated the primary request or canonical history")
	}
}
