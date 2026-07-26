package agents

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	agent "github.com/alfredxw/denova/agent"
	"github.com/alfredxw/denova/agent/model/openai"

	"denova/config"
)

func TestApplyToolResultContextPolicyPreservesToolExchangeExactly(t *testing.T) {
	arguments := "  {\"path\":\"chapter.md\",\"selection\":\"" + strings.Repeat("段落", 3000) + "\"}  "
	content := "{\"items\":[" + strings.Repeat("{\"name\":\"条目\"},", 3000) + "\nnot-valid-inner-json"
	messages := []*agent.Message{
		agent.UserMessage("读取资料"),
		agent.AssistantMessage("", []agent.ToolCall{{
			ID: "call-large", Type: "function",
			Function: agent.FunctionCall{Name: "read", Arguments: arguments},
		}}),
		agent.ToolMessage(agent.TextToolResult(content), "call-large", agent.WithToolName("read")),
		agent.UserMessage("继续"),
	}

	filtered := applyToolResultContextPolicy(messages, ToolResultContextPolicy{Enabled: true})
	if len(filtered) != len(messages) {
		t.Fatalf("complete tool exchange should remain in context: got=%d want=%d", len(filtered), len(messages))
	}
	if got := filtered[1].ToolCalls[0].Function.Arguments; got != arguments {
		t.Fatalf("tool arguments were rewritten: got_bytes=%d want_bytes=%d", len(got), len(arguments))
	}
	if got := filtered[2].Content; got != content {
		t.Fatalf("tool result was rewritten: got_bytes=%d want_bytes=%d", len(got), len(content))
	}
	if strings.Contains(filtered[2].Content, "tool_result.placeholder") || strings.Contains(filtered[1].ToolCalls[0].Function.Arguments, "args_omitted") {
		t.Fatalf("retained exchanges must not contain synthetic placeholders: %#v", filtered)
	}
}

func TestOpenAIRequestAssemblyKeepsToolContentAsString(t *testing.T) {
	arguments := `{"path":"chapter.md","offset":1,"limit":200}`
	content := "{\"items\":[" + strings.Repeat("{\"name\":\"条目\"},", 2000) + "\nnot-valid-inner-json"
	messages := applyToolResultContextPolicy([]*agent.Message{
		agent.AssistantMessage("", []agent.ToolCall{{
			ID: "call-json", Type: "function",
			Function: agent.FunctionCall{Name: "read", Arguments: arguments},
		}}),
		agent.ToolMessage(agent.TextToolResult(content), "call-json", agent.WithToolName("read")),
		agent.UserMessage("基于结果继续"),
	}, ToolResultContextPolicy{Enabled: true})

	var rawRequest []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, readErr := io.ReadAll(r.Body)
		if readErr != nil {
			t.Errorf("read OpenAI request: %v", readErr)
			http.Error(w, "read request", http.StatusInternalServerError)
			return
		}
		rawRequest = append([]byte(nil), body...)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"response-1","model":"test-model","created":1,"choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
	}))
	t.Cleanup(server.Close)

	chatModel, err := openai.New(context.Background(), &openai.Config{
		APIKey:  "test-key",
		Model:   "test-model",
		BaseURL: server.URL,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = chatModel.Generate(context.Background(), messages); err != nil {
		t.Fatalf("generate through capture server: %v", err)
	}
	if len(rawRequest) == 0 {
		t.Fatalf("OpenAI adapter did not assemble a request: %v", err)
	}

	var request struct {
		Messages []struct {
			Role       string `json:"role"`
			Content    string `json:"content"`
			ToolCallID string `json:"tool_call_id"`
			ToolCalls  []struct {
				Function struct {
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(rawRequest, &request); err != nil {
		t.Fatalf("decode assembled request: %v\n%s", err, rawRequest)
	}
	if len(request.Messages) != 3 {
		t.Fatalf("assembled messages = %#v", request.Messages)
	}
	if got := request.Messages[0].ToolCalls[0].Function.Arguments; got != arguments {
		t.Fatalf("assembled arguments changed: got=%q want=%q", got, arguments)
	}
	toolMessage := request.Messages[1]
	if toolMessage.Role != "tool" || toolMessage.ToolCallID != "call-json" || toolMessage.Content != content {
		t.Fatalf("tool content must be one opaque JSON string, got role=%q id=%q bytes=%d", toolMessage.Role, toolMessage.ToolCallID, len(toolMessage.Content))
	}
}

func TestApplyToolResultContextPolicyDisabledRemovesToolContext(t *testing.T) {
	messages := []*agent.Message{
		agent.UserMessage("查资料"),
		agent.AssistantMessage("", []agent.ToolCall{{ID: "call-1", Type: "function", Function: agent.FunctionCall{Name: "read", Arguments: `{}`}}}),
		agent.ToolMessage(agent.TextToolResult("result"), "call-1", agent.WithToolName("read")),
		agent.AssistantMessage("完成", nil),
	}

	filtered := applyToolResultContextPolicy(messages, ToolResultContextPolicy{Enabled: false})
	if len(filtered) != 2 || filtered[0].Role != agent.User || filtered[1].Content != "完成" {
		t.Fatalf("disabled retention should remove context-only tool messages: %#v", filtered)
	}
}

func TestToolResultContextRecorderPersistsAlreadyBoundedResultExactly(t *testing.T) {
	conversation := &recordedToolContextConversation{policy: ToolResultContextPolicy{Enabled: true, MaxResultBytes: 256}}
	recorder := newToolResultContextRecorder(conversation)
	arguments := `{"path":"chapter.md"}`
	recorder.RecordAssistantToolCalls(agent.AssistantMessage("", []agent.ToolCall{{
		ID: "call-1", Type: "function", Function: agent.FunctionCall{Name: "read", Arguments: arguments},
	}}), agentEventMetadata{})
	bounded := FilterToolResultForModelWithLimit("read", arguments, strings.Repeat("正文", 500), 256)
	recorder.RecordToolResult(agent.ToolMessage(bounded.Result, "call-1", agent.WithToolName("read")), agentEventMetadata{})

	if len(conversation.messages) != 2 {
		t.Fatalf("recorded messages = %#v", conversation.messages)
	}
	if got := conversation.messages[0].ToolCalls[0].Function.Arguments; got != arguments {
		t.Fatalf("recorded arguments changed: got=%q want=%q", got, arguments)
	}
	if got := conversation.messages[1].Content; got != bounded.Content {
		t.Fatalf("bounded result was filtered a second time: got_bytes=%d want_bytes=%d", len(got), len(bounded.Content))
	}
	if !strings.Contains(conversation.messages[1].Content, "[tool result truncated]") ||
		conversation.messages[1].ToolResult == nil || !conversation.messages[1].ToolResult.ModelTruncated ||
		strings.Contains(conversation.messages[1].Content, toolResultMetadataHeader) {
		t.Fatalf("structured truncation summary should remain intact: %#v", conversation.messages[1])
	}
}

func TestToolResultContextRecorderSkipsMalformedCallAndResult(t *testing.T) {
	conversation := &recordedToolContextConversation{policy: ToolResultContextPolicy{Enabled: true}}
	recorder := newToolResultContextRecorder(conversation)
	recorder.RecordAssistantToolCalls(agent.AssistantMessage("", []agent.ToolCall{{
		ID: "call-invalid", Type: "function",
		Function: agent.FunctionCall{Name: "write", Arguments: `{"content":`},
	}}), agentEventMetadata{})
	recorder.RecordToolResult(agent.ToolMessage(agent.TextToolResult("invalid arguments"), "call-invalid", agent.WithToolName("write")), agentEventMetadata{})
	if len(conversation.messages) != 0 {
		t.Fatalf("malformed tool call and result must not persist: %#v", conversation.messages)
	}
}

func TestApplyToolResultContextPolicyDropsMalformedAndOrphanedPairsAndCompletesMissingResult(t *testing.T) {
	messages := []*agent.Message{
		agent.AssistantMessage("useful narration", []agent.ToolCall{
			{ID: "invalid", Type: "function", Function: agent.FunctionCall{Name: "read", Arguments: `{"path":`}},
			{ID: "missing", Type: "function", Function: agent.FunctionCall{Name: "read", Arguments: `{}`}},
		}),
		agent.ToolMessage(agent.TextToolResult("invalid arguments"), "invalid", agent.WithToolName("read")),
		agent.ToolMessage(agent.TextToolResult("orphan result"), "unknown", agent.WithToolName("read")),
		agent.UserMessage("继续"),
	}
	filtered := applyToolResultContextPolicy(messages, ToolResultContextPolicy{Enabled: true})
	if len(filtered) != 3 || filtered[0].Content != "useful narration" || len(filtered[0].ToolCalls) != 1 ||
		filtered[0].ToolCalls[0].ID != "missing" || filtered[1].Role != agent.ToolRole ||
		filtered[1].ToolCallID != "missing" || !isUnknownToolEffectResult(filtered[1].Content) || filtered[2].Role != agent.User {
		t.Fatalf("malformed/orphaned protocol must be dropped while a unique missing result is recovered: %#v", filtered)
	}
}

func TestApplyToolResultContextPolicyDropsTransientIndexesWithTheirCalls(t *testing.T) {
	messages := []*agent.Message{
		agent.AssistantMessage("", []agent.ToolCall{{ID: "call-list", Type: "function", Function: agent.FunctionCall{Name: "list_lore_items", Arguments: `{"keywords":["门"]}`}}}),
		agent.ToolMessage(agent.TextToolResult("很长的资料索引"), "call-list", agent.WithToolName("list_lore_items")),
		agent.AssistantMessage("继续故事", nil),
	}
	filtered := applyToolResultContextPolicy(messages, ToolResultContextPolicy{Enabled: true})
	if len(filtered) != 1 || filtered[0].Content != "继续故事" {
		t.Fatalf("transient index call and result should not cross turns: %#v", filtered)
	}
}

func TestToolResultContextReplacesLoreBodiesWithSourceReceipt(t *testing.T) {
	raw := "# 资料库条目\n\n## 黄泉酒馆（location / major / resident）\nID：lore-tavern\n\n```markdown\n掌柜隐藏着不可公开的秘密正文。\n```"
	content := toolResultContextContent("read_lore_items", raw, ToolResultContextPolicy{})
	for _, want := range []string{retainedToolReceiptSchema, "read_lore_items", "lore-tavern", "黄泉酒馆"} {
		if !strings.Contains(content, want) {
			t.Fatalf("retained lore receipt missing %q: %s", want, content)
		}
	}
	if strings.Contains(content, "不可公开的秘密正文") {
		t.Fatalf("lore body must not be duplicated into cross-turn context: %s", content)
	}
}

func TestInteractiveStoryToolContextKeepsOnlySemanticReadReceipts(t *testing.T) {
	messages := []*agent.Message{
		agent.AssistantMessage("", []agent.ToolCall{
			{ID: "prepare", Type: "function", Function: agent.FunctionCall{Name: "prepare_interactive_turn", Arguments: `{}`}},
			{ID: "file", Type: "function", Function: agent.FunctionCall{Name: "read", Arguments: `{}`}},
			{ID: "lore", Type: "function", Function: agent.FunctionCall{Name: "read_lore_items", Arguments: `{}`}},
		}),
		agent.ToolMessage(agent.TextToolResult(`{"outcome":"success"}`), "prepare", agent.WithToolName("prepare_interactive_turn")),
		agent.ToolMessage(agent.TextToolResult("文风正文"), "file", agent.WithToolName("read")),
		agent.ToolMessage(agent.TextToolResult("# 资料库条目\n\n## 酒馆\nID：lore-tavern\n\n秘密正文"), "lore", agent.WithToolName("read_lore_items")),
	}
	filtered := applyToolResultContextPolicy(messages, ToolResultContextPolicy{AgentKind: config.AgentKindInteractiveStory, Enabled: true})
	if len(filtered) != 2 || len(filtered[0].ToolCalls) != 1 || filtered[0].ToolCalls[0].Function.Name != "read_lore_items" || !strings.Contains(filtered[1].Content, retainedToolReceiptSchema) {
		t.Fatalf("game context should contain only the semantic lore receipt pair: %#v", filtered)
	}
}

func TestApplyToolResultContextPolicyPairsByCallIDWhenResultToolNameMissing(t *testing.T) {
	messages := []*agent.Message{
		agent.AssistantMessage("", []agent.ToolCall{
			{ID: "call-list", Type: "function", Function: agent.FunctionCall{Name: "list_lore_items", Arguments: `{}`}},
			{ID: "call-read", Type: "function", Function: agent.FunctionCall{Name: "read_lore_items", Arguments: `{}`}},
		}),
		agent.ToolMessage(agent.TextToolResult("索引结果"), "call-list"),
		agent.ToolMessage(agent.TextToolResult("# 资料库条目\n\n## 酒馆\nID：lore-tavern\n\n秘密正文"), "call-read"),
	}
	filtered := applyToolResultContextPolicy(messages, ToolResultContextPolicy{Enabled: true})
	if len(filtered) != 2 || len(filtered[0].ToolCalls) != 1 || filtered[0].ToolCalls[0].ID != "call-read" {
		t.Fatalf("only the retained call/result pair should remain: %#v", filtered)
	}
	if filtered[1].ToolName != "read_lore_items" || !strings.Contains(filtered[1].Content, retainedToolReceiptSchema) {
		t.Fatalf("result should inherit its paired tool name and become a receipt: %#v", filtered[1])
	}
}

func TestApplyToolResultContextPolicyDropsAmbiguousDuplicatePair(t *testing.T) {
	messages := []*agent.Message{
		agent.AssistantMessage("", []agent.ToolCall{
			{ID: "duplicate", Type: "function", Function: agent.FunctionCall{Name: "read", Arguments: `{}`}},
			{ID: "duplicate", Type: "function", Function: agent.FunctionCall{Name: "read_lore_items", Arguments: `{}`}},
		}),
		agent.ToolMessage(agent.TextToolResult("ambiguous"), "duplicate"),
	}
	if filtered := applyToolResultContextPolicy(messages, ToolResultContextPolicy{Enabled: true}); len(filtered) != 0 {
		t.Fatalf("duplicate call ids must be dropped instead of mispaired: %#v", filtered)
	}
}

func TestToolResultContextKeepsLoreErrorsInsteadOfPositiveReceipt(t *testing.T) {
	raw := "读取资料失败：条目不存在"
	if content := toolResultContextContent("read_lore_items", raw, ToolResultContextPolicy{}); content != raw {
		t.Fatalf("failed reads should remain errors instead of positive receipts: %q", content)
	}
}

type recordedToolContextConversation struct {
	Conversation
	messages []*agent.Message
	policy   ToolResultContextPolicy
}

func (c *recordedToolContextConversation) AppendContextMessage(msg *agent.Message) error {
	c.messages = append(c.messages, msg)
	return nil
}

func (c *recordedToolContextConversation) ToolResultContextPolicy() ToolResultContextPolicy {
	return c.policy
}
