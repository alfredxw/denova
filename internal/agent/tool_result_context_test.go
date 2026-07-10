package agent

import (
	"strings"
	"testing"

	"github.com/cloudwego/eino/schema"
)

func TestApplyToolResultContextPolicyKeepsRecentAndPlaceholdersOldResults(t *testing.T) {
	messages := []*schema.Message{
		schema.UserMessage("查资料"),
		schema.AssistantMessage("", []schema.ToolCall{{ID: "call-1", Type: "function", Function: schema.FunctionCall{Name: "read_file", Arguments: `{"path":"a"}`}}}),
		schema.ToolMessage(strings.Repeat("A", 60), "call-1", schema.WithToolName("read_file")),
		schema.AssistantMessage("", []schema.ToolCall{{ID: "call-2", Type: "function", Function: schema.FunctionCall{Name: "read_file", Arguments: `{"path":"b"}`}}}),
		schema.ToolMessage("recent result", "call-2", schema.WithToolName("read_file")),
		schema.AssistantMessage("完成", nil),
	}

	filtered := applyToolResultContextPolicy(messages, ToolResultContextPolicy{
		Enabled:      true,
		KeepRecent:   1,
		BudgetBytes:  20,
		PreviewChars: 100,
	})

	if len(filtered) != len(messages) {
		t.Fatalf("tool context messages should remain paired, got %d want %d", len(filtered), len(messages))
	}
	if filtered[2].Role != schema.Tool || !strings.Contains(filtered[2].Content, "tool_result.placeholder.v1") {
		t.Fatalf("old over-budget result should become placeholder: %#v", filtered[2])
	}
	if filtered[4].Content != "recent result" {
		t.Fatalf("recent tool result should remain full, got %q", filtered[4].Content)
	}
	if filtered[1].Role != schema.Assistant || len(filtered[1].ToolCalls) != 1 {
		t.Fatalf("assistant tool call should remain paired: %#v", filtered[1])
	}
}

func TestApplyToolResultContextPolicyCountsRecentResultsAgainstBudget(t *testing.T) {
	messages := []*schema.Message{
		schema.ToolMessage("older-recent", "call-1", schema.WithToolName("read_file")),
		schema.ToolMessage("newer-recent", "call-2", schema.WithToolName("read_file")),
	}
	filtered := applyToolResultContextPolicy(messages, ToolResultContextPolicy{
		Enabled:      true,
		KeepRecent:   2,
		BudgetBytes:  len("newer-recent"),
		PreviewChars: 100,
	})
	if filtered[1].Content != "newer-recent" {
		t.Fatalf("newest result should receive budget first: %#v", filtered[1])
	}
	if !strings.Contains(filtered[0].Content, "tool_result.placeholder.v1") {
		t.Fatalf("recent results must still respect the aggregate budget: %#v", filtered[0])
	}
}

func TestApplyToolResultContextPolicyDisabledRemovesToolContext(t *testing.T) {
	messages := []*schema.Message{
		schema.UserMessage("查资料"),
		schema.AssistantMessage("", []schema.ToolCall{{ID: "call-1", Type: "function", Function: schema.FunctionCall{Name: "read_file", Arguments: `{}`}}}),
		schema.ToolMessage("result", "call-1", schema.WithToolName("read_file")),
		schema.AssistantMessage("完成", nil),
	}

	filtered := applyToolResultContextPolicy(messages, ToolResultContextPolicy{Enabled: false})

	if len(filtered) != 2 || filtered[0].Role != schema.User || filtered[1].Content != "完成" {
		t.Fatalf("disabled retention should remove context-only tool messages: %#v", filtered)
	}
}

func TestToolResultContextRecorderBoundsLargeResults(t *testing.T) {
	content := toolResultContextContent("read_file", "call-1", strings.Repeat("内容", 20), ToolResultContextPolicy{PreviewChars: 5})
	if !strings.Contains(content, "tool result preview truncated") {
		t.Fatalf("large result should include truncation marker: %q", content)
	}
	call := assistantToolContextMessage(schema.AssistantMessage("", []schema.ToolCall{{
		ID:   "call-1",
		Type: "function",
		Function: schema.FunctionCall{
			Name:      "write_file",
			Arguments: strings.Repeat("x", 20),
		},
	}}), ToolResultContextPolicy{PreviewChars: 6})
	if call == nil || len(call.ToolCalls) != 1 || !strings.Contains(call.ToolCalls[0].Function.Arguments, "tool call args truncated") {
		t.Fatalf("large tool args should be bounded: %#v", call)
	}
}

func TestToolResultContextRemovesDenovaMetadata(t *testing.T) {
	raw := "章节内容\n\n[Denova tool result metadata]\nschema: tool_result.v1\nmutates_workspace: false"
	content := toolResultContextContent("read_file", "call-1", raw, ToolResultContextPolicy{PreviewChars: 100})
	if content != "章节内容" {
		t.Fatalf("retained content should remove metadata, got %q", content)
	}

	filtered := applyToolResultContextPolicy([]*schema.Message{
		schema.ToolMessage(raw, "call-1", schema.WithToolName("read_file")),
	}, ToolResultContextPolicy{Enabled: true, KeepRecent: 1, BudgetBytes: 1024, PreviewChars: 100})
	if len(filtered) != 1 || filtered[0].Content != "章节内容" {
		t.Fatalf("policy should sanitize legacy retained tool result: %#v", filtered)
	}
}
