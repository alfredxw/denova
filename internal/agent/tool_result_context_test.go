package agent

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/cloudwego/eino/schema"

	"denova/config"
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
		schema.AssistantMessage("", []schema.ToolCall{
			{ID: "call-1", Type: "function", Function: schema.FunctionCall{Name: "read_file", Arguments: `{}`}},
			{ID: "call-2", Type: "function", Function: schema.FunctionCall{Name: "read_file", Arguments: `{}`}},
		}),
		schema.ToolMessage("older-recent", "call-1", schema.WithToolName("read_file")),
		schema.ToolMessage("newer-recent", "call-2", schema.WithToolName("read_file")),
	}
	filtered := applyToolResultContextPolicy(messages, ToolResultContextPolicy{
		Enabled:      true,
		KeepRecent:   2,
		BudgetBytes:  len("newer-recent"),
		PreviewChars: 100,
	})
	if filtered[2].Content != "newer-recent" {
		t.Fatalf("newest result should receive budget first: %#v", filtered[2])
	}
	if !strings.Contains(filtered[1].Content, "tool_result.placeholder.v1") {
		t.Fatalf("recent results must still respect the aggregate budget: %#v", filtered[1])
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
			Arguments: `{"content":"` + strings.Repeat("x", 20) + `"}`,
		},
	}}), ToolResultContextPolicy{PreviewChars: 6})
	if call == nil || len(call.ToolCalls) != 1 || !strings.Contains(call.ToolCalls[0].Function.Arguments, "tool_call.args_omitted.v1") {
		t.Fatalf("large tool args should be replaced with a valid receipt: %#v", call)
	}
	if err := validateToolArgumentsJSON(call.ToolCalls[0].Function.Arguments); err != nil {
		t.Fatalf("retained tool arguments must stay valid JSON: %v", err)
	}
}

func TestToolResultContextRecorderSkipsMalformedCallAndResult(t *testing.T) {
	conversation := &recordedToolContextConversation{
		policy: ToolResultContextPolicy{Enabled: true, PreviewChars: 100},
	}
	recorder := newToolResultContextRecorder(conversation)
	recorder.RecordAssistantToolCalls(schema.AssistantMessage("", []schema.ToolCall{{
		ID:   "call-invalid",
		Type: "function",
		Function: schema.FunctionCall{
			Name:      "write_file",
			Arguments: `{"content":`,
		},
	}}), agentEventMetadata{})
	recorder.RecordToolResult("write_file", "call-invalid", "invalid arguments", agentEventMetadata{})

	if len(conversation.messages) != 0 {
		t.Fatalf("malformed tool call and result must not persist into future context: %#v", conversation.messages)
	}
}

func TestApplyToolResultContextPolicyDropsMalformedLegacyToolPair(t *testing.T) {
	messages := []*schema.Message{
		schema.AssistantMessage("", []schema.ToolCall{{
			ID:   "call-invalid",
			Type: "function",
			Function: schema.FunctionCall{
				Name:      "read_file",
				Arguments: `{"path":`,
			},
		}}),
		schema.ToolMessage("invalid arguments", "call-invalid", schema.WithToolName("read_file")),
		schema.UserMessage("继续"),
	}

	filtered := applyToolResultContextPolicy(messages, ToolResultContextPolicy{Enabled: true, BudgetBytes: 1024, PreviewChars: 100})
	if len(filtered) != 1 || filtered[0].Role != schema.User || filtered[0].Content != "继续" {
		t.Fatalf("legacy malformed tool pair must be excluded from the next request: %#v", filtered)
	}
}

func TestApplyToolResultContextPolicyUsesValidArgumentsReceipt(t *testing.T) {
	messages := []*schema.Message{
		schema.AssistantMessage("", []schema.ToolCall{{
			ID:   "call-large",
			Type: "function",
			Function: schema.FunctionCall{
				Name:      "read_file",
				Arguments: `{"path":"` + strings.Repeat("a", 100) + `"}`,
			},
		}}),
		schema.ToolMessage("content", "call-large", schema.WithToolName("read_file")),
	}

	filtered := applyToolResultContextPolicy(messages, ToolResultContextPolicy{Enabled: true, BudgetBytes: 1024, PreviewChars: 20})
	if len(filtered) != 2 || len(filtered[0].ToolCalls) != 1 {
		t.Fatalf("large retained tool pair should remain available: %#v", filtered)
	}
	arguments := filtered[0].ToolCalls[0].Function.Arguments
	if err := validateToolArgumentsJSON(arguments); err != nil {
		t.Fatalf("retained arguments must be valid JSON: %v", err)
	}
	var receipt struct {
		Schema string `json:"schema"`
	}
	if err := json.Unmarshal([]byte(arguments), &receipt); err != nil || receipt.Schema != "tool_call.args_omitted.v1" {
		t.Fatalf("expected a retained arguments receipt, got %q err=%v", arguments, err)
	}
}

func TestToolResultContextRemovesDenovaMetadata(t *testing.T) {
	raw := "章节内容\n\n[Denova tool result metadata]\nschema: tool_result.v1\nmutates_workspace: false"
	content := toolResultContextContent("read_file", "call-1", raw, ToolResultContextPolicy{PreviewChars: 100})
	if content != "章节内容" {
		t.Fatalf("retained content should remove metadata, got %q", content)
	}

	filtered := applyToolResultContextPolicy([]*schema.Message{
		schema.AssistantMessage("", []schema.ToolCall{{ID: "call-1", Type: "function", Function: schema.FunctionCall{Name: "read_file", Arguments: `{}`}}}),
		schema.ToolMessage(raw, "call-1", schema.WithToolName("read_file")),
	}, ToolResultContextPolicy{Enabled: true, KeepRecent: 1, BudgetBytes: 1024, PreviewChars: 100})
	if len(filtered) != 2 || filtered[1].Content != "章节内容" {
		t.Fatalf("policy should sanitize legacy retained tool result: %#v", filtered)
	}
}

func TestApplyToolResultContextPolicyDropsTransientIndexesWithTheirCalls(t *testing.T) {
	messages := []*schema.Message{
		schema.AssistantMessage("", []schema.ToolCall{{ID: "call-list", Type: "function", Function: schema.FunctionCall{Name: "list_lore_items", Arguments: `{"keywords":["门"]}`}}}),
		schema.ToolMessage("很长的资料索引", "call-list", schema.WithToolName("list_lore_items")),
		schema.AssistantMessage("继续故事", nil),
	}

	filtered := applyToolResultContextPolicy(messages, ToolResultContextPolicy{Enabled: true, KeepRecent: 3, BudgetBytes: 1024, PreviewChars: 100})
	if len(filtered) != 1 || filtered[0].Content != "继续故事" {
		t.Fatalf("transient index call and result should not cross turns: %#v", filtered)
	}
}

func TestToolResultContextReplacesLoreBodiesWithSourceReceipt(t *testing.T) {
	raw := "# 资料库条目\n\n## 黄泉酒馆（location / major / resident）\nID：lore-tavern\n\n```markdown\n掌柜隐藏着不可公开的秘密正文。\n```"
	content := toolResultContextContent("read_lore_items", "call-lore", raw, ToolResultContextPolicy{PreviewChars: 2000})
	for _, want := range []string{retainedToolReceiptSchema, "read_lore_items", "lore-tavern", "黄泉酒馆"} {
		if !strings.Contains(content, want) {
			t.Fatalf("retained lore receipt missing %q: %s", want, content)
		}
	}
	if strings.Contains(content, "不可公开的秘密正文") {
		t.Fatalf("lore body must not be duplicated into cross-turn context: %s", content)
	}
}

func TestInteractiveStoryToolContextDropsTransientReadFilePreview(t *testing.T) {
	messages := []*schema.Message{
		schema.AssistantMessage("", []schema.ToolCall{{ID: "call-style", Type: "function", Function: schema.FunctionCall{Name: "read_file", Arguments: `{"file_path":"/style/reference.md"}`}}}),
		schema.ToolMessage("文风正文", "call-style", schema.WithToolName("read_file")),
	}
	filtered := applyToolResultContextPolicy(messages, ToolResultContextPolicy{AgentKind: config.AgentKindInteractiveStory, Enabled: true, BudgetBytes: 1024, PreviewChars: 100})
	if len(filtered) != 0 {
		t.Fatalf("interactive style preview should be transient: %#v", filtered)
	}
}

func TestInteractiveStoryToolContextKeepsOnlySemanticReadReceipts(t *testing.T) {
	messages := []*schema.Message{
		schema.AssistantMessage("", []schema.ToolCall{
			{ID: "prepare", Type: "function", Function: schema.FunctionCall{Name: "prepare_interactive_turn", Arguments: `{}`}},
			{ID: "patches", Type: "function", Function: schema.FunctionCall{Name: "submit_actor_state_patches", Arguments: `{}`}},
			{ID: "choices", Type: "function", Function: schema.FunctionCall{Name: "submit_choices", Arguments: `{}`}},
			{ID: "lore", Type: "function", Function: schema.FunctionCall{Name: "read_lore_items", Arguments: `{}`}},
		}),
		schema.ToolMessage(`{"outcome":"success"}`, "prepare", schema.WithToolName("prepare_interactive_turn")),
		schema.ToolMessage(`{"ready":false}`, "patches", schema.WithToolName("submit_actor_state_patches")),
		schema.ToolMessage(`{"ready":true}`, "choices", schema.WithToolName("submit_choices")),
		schema.ToolMessage("# 资料库条目\n\n## 酒馆\nID：lore-tavern\n\n秘密正文", "lore", schema.WithToolName("read_lore_items")),
	}
	filtered := applyToolResultContextPolicy(messages, ToolResultContextPolicy{AgentKind: config.AgentKindInteractiveStory, Enabled: true, BudgetBytes: 4096, PreviewChars: 1000})
	if len(filtered) != 2 || len(filtered[0].ToolCalls) != 1 || filtered[0].ToolCalls[0].Function.Name != "read_lore_items" || !strings.Contains(filtered[1].Content, retainedToolReceiptSchema) {
		t.Fatalf("game cross-turn context should contain only the semantic lore receipt pair: %#v", filtered)
	}
}

func TestApplyToolResultContextPolicyPairsByCallIDWhenResultToolNameMissing(t *testing.T) {
	messages := []*schema.Message{
		schema.AssistantMessage("", []schema.ToolCall{
			{ID: "call-list", Type: "function", Function: schema.FunctionCall{Name: "list_lore_items", Arguments: `{}`}},
			{ID: "call-read", Type: "function", Function: schema.FunctionCall{Name: "read_lore_items", Arguments: `{}`}},
		}),
		schema.ToolMessage("索引结果", "call-list"),
		schema.ToolMessage("# 资料库条目\n\n## 酒馆\nID：lore-tavern\n\n秘密正文", "call-read"),
	}

	filtered := applyToolResultContextPolicy(messages, ToolResultContextPolicy{Enabled: true, BudgetBytes: 4096, PreviewChars: 1000})
	if len(filtered) != 2 {
		t.Fatalf("only the retained call/result pair should remain: %#v", filtered)
	}
	if len(filtered[0].ToolCalls) != 1 || filtered[0].ToolCalls[0].ID != "call-read" {
		t.Fatalf("assistant calls should be filtered by paired call id: %#v", filtered[0])
	}
	if filtered[1].ToolName != "read_lore_items" || !strings.Contains(filtered[1].Content, retainedToolReceiptSchema) {
		t.Fatalf("result should inherit its paired tool name and become a receipt: %#v", filtered[1])
	}
}

func TestApplyToolResultContextPolicyDropsOrphanCallAndResult(t *testing.T) {
	messages := []*schema.Message{
		schema.AssistantMessage("useful narration", []schema.ToolCall{{ID: "missing-result", Type: "function", Function: schema.FunctionCall{Name: "read_file", Arguments: `{}`}}}),
		schema.ToolMessage("orphan result", "missing-call", schema.WithToolName("read_file")),
	}

	filtered := applyToolResultContextPolicy(messages, ToolResultContextPolicy{Enabled: true, BudgetBytes: 1024, PreviewChars: 100})
	if len(filtered) != 1 || filtered[0].Content != "useful narration" || len(filtered[0].ToolCalls) != 0 {
		t.Fatalf("unpaired protocol messages must not enter the next model turn: %#v", filtered)
	}
}

func TestApplyToolResultContextPolicyDropsAmbiguousDuplicatePair(t *testing.T) {
	messages := []*schema.Message{
		schema.AssistantMessage("", []schema.ToolCall{
			{ID: "duplicate", Type: "function", Function: schema.FunctionCall{Name: "read_file", Arguments: `{}`}},
			{ID: "duplicate", Type: "function", Function: schema.FunctionCall{Name: "read_lore_items", Arguments: `{}`}},
		}),
		schema.ToolMessage("ambiguous", "duplicate"),
	}

	filtered := applyToolResultContextPolicy(messages, ToolResultContextPolicy{Enabled: true, BudgetBytes: 1024, PreviewChars: 100})
	if len(filtered) != 0 {
		t.Fatalf("duplicate call ids must be dropped instead of mispaired: %#v", filtered)
	}
}

func TestToolResultContextKeepsLoreErrorsInsteadOfPositiveReceipt(t *testing.T) {
	raw := "读取资料失败：条目不存在"
	content := toolResultContextContent("read_lore_items", "call-lore", raw, ToolResultContextPolicy{PreviewChars: 100})
	if content != raw {
		t.Fatalf("failed reads should remain an error instead of becoming a receipt: %q", content)
	}
}

func TestToolResultContextContentDoesNotMidCutJSON(t *testing.T) {
	raw, err := json.Marshal(map[string]any{"items": strings.Repeat("修炼境界寿元设定", 500)})
	if err != nil {
		t.Fatal(err)
	}
	content := toolResultContextContent("write_file", "call-json", string(raw), ToolResultContextPolicy{PreviewChars: 2000})
	if strings.Contains(content, "preview truncated for context") {
		t.Fatalf("must not mid-cut JSON tool bodies: %s", content)
	}
	if !strings.Contains(content, "tool_result_json_preview_exceeded") {
		t.Fatalf("oversized JSON should become placeholder: %s", content)
	}
}


type recordedToolContextConversation struct {
	Conversation
	messages []*schema.Message
	policy   ToolResultContextPolicy
}

func (c *recordedToolContextConversation) AppendContextMessage(msg *schema.Message) error {
	c.messages = append(c.messages, msg)
	return nil
}

func (c *recordedToolContextConversation) ToolResultContextPolicy() ToolResultContextPolicy {
	return c.policy
}
