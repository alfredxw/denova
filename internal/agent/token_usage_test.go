package agent

import (
	"testing"

	"github.com/cloudwego/eino/schema"
)

func TestRunTokenUsageCollectorRecordsToolContext(t *testing.T) {
	collector := newRunTokenUsageCollector("run-1", "interactive")
	collector.NoteToolResult("read_workspace_file")

	collector.AddMessage(&schema.Message{
		ToolCalls: []schema.ToolCall{{
			ID:       "call-1",
			Function: schema.FunctionCall{Name: "write_workspace_file"},
		}},
		ResponseMeta: &schema.ResponseMeta{
			FinishReason: "tool_calls",
			Usage: &schema.TokenUsage{
				PromptTokens:       100,
				PromptTokenDetails: schema.PromptTokenDetails{CachedTokens: 40},
				CompletionTokens:   20,
				TotalTokens:        120,
			},
		},
	})

	if len(collector.stats.Calls) != 1 {
		t.Fatalf("usage calls = %d, want 1", len(collector.stats.Calls))
	}
	call := collector.stats.Calls[0]
	if len(call.AfterTools) != 1 || call.AfterTools[0] != "read_workspace_file" {
		t.Fatalf("after tools not recorded: %#v", call.AfterTools)
	}
	if len(call.RequestedTools) != 1 || call.RequestedTools[0] != "write_workspace_file" {
		t.Fatalf("requested tools not recorded: %#v", call.RequestedTools)
	}
	if call.FinishReason != "tool_calls" || call.CreatedAt == "" {
		t.Fatalf("usage call metadata not recorded: %#v", call)
	}
	if call.UncachedPromptTokens != 60 || collector.stats.UncachedPromptTokens != 60 {
		t.Fatalf("uncached prompt tokens not recorded: call=%#v stats=%#v", call, collector.stats)
	}

	collector.AddMessage(&schema.Message{
		ResponseMeta: &schema.ResponseMeta{
			FinishReason: "stop",
			Usage: &schema.TokenUsage{
				PromptTokens:       80,
				PromptTokenDetails: schema.PromptTokenDetails{CachedTokens: 100},
				CompletionTokens:   10,
				TotalTokens:        90,
			},
		},
	})

	if len(collector.stats.Calls) != 2 {
		t.Fatalf("usage calls = %d, want 2", len(collector.stats.Calls))
	}
	if len(collector.stats.Calls[1].AfterTools) != 0 {
		t.Fatalf("after tools should be consumed by the next model call: %#v", collector.stats.Calls[1].AfterTools)
	}
	if collector.stats.Calls[1].UncachedPromptTokens != 0 || collector.stats.UncachedPromptTokens != 60 {
		t.Fatalf("uncached prompt tokens should be clamped: calls=%#v stats=%#v", collector.stats.Calls, collector.stats)
	}
}
