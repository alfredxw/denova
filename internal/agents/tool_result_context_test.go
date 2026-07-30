package agents

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	agent "github.com/alfredxw/denova/agent"
	"github.com/alfredxw/denova/agent/model/openai"

	"denova/config"
	"denova/internal/agents/session"
	producttools "denova/internal/agents/tools"
)

func TestApplyToolResultContextPolicyKeepsBoundedRichExchangeAcrossTurns(t *testing.T) {
	arguments := "  {\"path\":\"chapter.md\",\"selection\":\"" + strings.Repeat("段落", 3000) + "\"}  "
	content := "{\"items\":[" + strings.Repeat("{\"name\":\"条目\"},", 3000) + "\nnot-valid-inner-json"
	descriptor := producttools.BoundedReadDescriptor(agent.ToolSourceRead, config.AgentToolWorkspaceRead)
	projected := filterStructuredToolResultWithDescriptor("read", descriptor, arguments, agent.TextToolResult(content), 0)
	messages := []*agent.Message{
		agent.UserMessage("读取资料"),
		agent.AssistantMessage("", []agent.ToolCall{{
			ID: "call-large", Type: "function",
			Function: agent.FunctionCall{Name: "read", Arguments: arguments},
		}}),
		agent.ToolMessage(projected.Result, "call-large", agent.WithToolName("read")),
		agent.UserMessage("继续"),
	}

	filtered := applyToolResultContextPolicy(messages, ToolResultContextPolicy{Enabled: true})
	if len(filtered) != len(messages) {
		t.Fatalf("complete tool exchange should remain in context: got=%d want=%d", len(filtered), len(messages))
	}
	if got := filtered[1].ToolCalls[0].Function.Arguments; got != arguments {
		t.Fatalf("tool arguments changed before pressure cleanup: got=%q", got)
	}
	if got := filtered[2].Content; got != projected.Result.ModelContent || strings.Contains(got, retainedToolReceiptSchema) {
		t.Fatalf("cross-turn rich result changed before pressure cleanup: bytes=%d", len(got))
	}
	if projected.Result.ModelContent != content {
		t.Fatalf("same-run rich result changed: got_bytes=%d want_bytes=%d", len(projected.Result.ModelContent), len(content))
	}
}

func TestOpenAIRequestAssemblyKeepsToolContentAsString(t *testing.T) {
	arguments := `{"path":"chapter.md","offset":1,"limit":200}`
	content := "{\"items\":[" + strings.Repeat("{\"name\":\"条目\"},", 2000) + "\nnot-valid-inner-json"
	descriptor := producttools.BoundedReadDescriptor(agent.ToolSourceRead, config.AgentToolWorkspaceRead)
	projected := filterStructuredToolResultWithDescriptor("read", descriptor, arguments, agent.TextToolResult(content), 0)
	messages := applyToolResultContextPolicy([]*agent.Message{
		agent.AssistantMessage("", []agent.ToolCall{{
			ID: "call-json", Type: "function",
			Function: agent.FunctionCall{Name: "read", Arguments: arguments},
		}}),
		agent.ToolMessage(projected.Result, "call-json", agent.WithToolName("read")),
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
		t.Fatalf("assembled arguments changed before pressure cleanup: %q", got)
	}
	toolMessage := request.Messages[1]
	if toolMessage.Role != "tool" || toolMessage.ToolCallID != "call-json" ||
		toolMessage.Content != projected.Result.ModelContent || strings.Contains(toolMessage.Content, retainedToolReceiptSchema) {
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

func TestDescriptorControlsRetentionForFutureToolsAndRedactsSensitiveArguments(t *testing.T) {
	arguments := `{"query":"stable","access_token":"do-not-retain"}`
	descriptor := producttools.BoundedReadDescriptor(agent.ToolSourceWeb, "future.read")
	result := filterStructuredToolResultWithDescriptor(
		"future_lookup", descriptor, arguments, agent.TextToolResult("large future result"), 0,
	).Result
	messages := []*agent.Message{
		agent.AssistantMessage("", []agent.ToolCall{{
			ID: "future-call", Type: "function",
			Function: agent.FunctionCall{Name: "future_lookup", Arguments: arguments},
		}}),
		agent.ToolMessage(result, "future-call", agent.WithToolName("future_lookup")),
	}
	retained := applyToolResultContextPolicy(messages, ToolResultContextPolicy{
		AgentKind: config.AgentKindInteractiveStory, Enabled: true,
	})
	if len(retained) != 2 || retained[1].Content != "large future result" {
		t.Fatalf("descriptor-declared future tool was not retained: %#v", retained)
	}
	retainedArguments := retained[0].ToolCalls[0].Function.Arguments
	if retainedArguments != arguments {
		t.Fatalf("canonical rich arguments changed before pressure cleanup = %s", retainedArguments)
	}

	descriptor.ResultRetention = agent.ToolResultEagerCandidate
	descriptor.ResultRecoveryKind = agent.ToolResultRecoveryRerun
	transient := filterStructuredToolResultWithDescriptor(
		"future_lookup", descriptor, arguments, agent.TextToolResult("transient result"), 0,
	).Result
	messages[1] = agent.ToolMessage(transient, "future-call", agent.WithToolName("future_lookup"))
	if filtered := applyToolResultContextPolicy(messages, ToolResultContextPolicy{Enabled: true}); len(filtered) != 2 || filtered[1].Content != "transient result" {
		t.Fatalf("turn age must not immediately clear a bounded result: %#v", filtered)
	}
}

func TestToolResultContextRecorderPersistsAtomicBoundedRichBatchInSourceOrder(t *testing.T) {
	conversation := &recordedToolContextConversation{policy: ToolResultContextPolicy{Enabled: true, MaxResultBytes: 4096}}
	recorder := newToolResultContextRecorder(conversation)
	readArguments := `{"path":"chapter.md"}`
	searchArguments := `{"query":"theme"}`
	assistant := agent.AssistantMessage("I will inspect both sources.", []agent.ToolCall{
		{ID: "call-read", Type: "function", Function: agent.FunctionCall{Name: "read", Arguments: readArguments}},
		{ID: "call-search", Type: "function", Function: agent.FunctionCall{Name: "web_search", Arguments: searchArguments}},
	})
	assistant.ResponseMeta = &agent.ResponseMeta{FinishReason: "tool_calls"}
	assistant.Extra = map[string]any{"provider_response_id": "response-1"}
	assistant.ReasoningContent = "private chain of thought must not cross turns"
	assistant.MultiContent = []json.RawMessage{json.RawMessage(`{"type":"reasoning","text":"private"}`)}
	assistant.AssistantGenMultiContent = []json.RawMessage{json.RawMessage(`{"type":"reasoning","text":"private"}`)}
	recorder.RecordAssistantToolCalls(assistant, agentEventMetadata{})

	bounded := FilterToolResultForModelWithLimit("read", readArguments, strings.Repeat("正文", 5000), 4096)
	bounded.Result.ResultRetention = agent.ToolResultDeferred
	bounded.Result.ContextHints = &agent.ToolResultContextHints{
		Recovery: agent.ToolResultRecoveryHint{
			Kind: agent.ToolResultRecoveryRead, Reference: map[string]any{"path": "chapter.md"},
			EstimatedBytes: int64(bounded.OriginalBytes), EstimatedTokens: estimatedToolResultTokens(int64(bounded.OriginalBytes)),
		},
		ContextValue: agent.ToolResultContextNormal,
	}
	readResult := agent.ToolMessage(bounded.Result, "call-read", agent.WithToolName("wrong-name"))
	searchResult := agent.ToolMessage(agent.TextToolResult("search evidence"), "call-search")

	// Parallel tools may finish out of order. Nothing is published until every
	// terminal result has arrived, then results are restored to call order.
	recorder.RecordToolResult(searchResult, agentEventMetadata{})
	if conversation.batches != 0 || len(conversation.messages) != 0 {
		t.Fatalf("partial tool batch was published: %#v", conversation.messages)
	}
	assistant.Content = "mutated"
	assistant.ToolCalls[0].Function.Arguments = `{}`
	searchResult.Content = "mutated"
	recorder.RecordToolResult(readResult, agentEventMetadata{})

	if len(conversation.messages) != 3 {
		t.Fatalf("recorded messages = %#v", conversation.messages)
	}
	recordedAssistant := conversation.messages[0]
	if recordedAssistant.Content != "I will inspect both sources." || len(recordedAssistant.ToolCalls) != 2 ||
		recordedAssistant.ToolCalls[0].ID != "call-read" || recordedAssistant.ToolCalls[1].ID != "call-search" ||
		recordedAssistant.ToolCalls[0].Function.Arguments != readArguments {
		t.Fatalf("original assistant batch was not preserved: %#v", recordedAssistant)
	}
	if recordedAssistant.ReasoningContent != "" || recordedAssistant.ResponseMeta != nil || recordedAssistant.Extra != nil ||
		len(recordedAssistant.MultiContent) != 0 || len(recordedAssistant.AssistantGenMultiContent) != 0 {
		t.Fatalf("private reasoning or transport metadata crossed the durable tool boundary: %#v", recordedAssistant)
	}
	if conversation.batches != 1 {
		t.Fatalf("tool exchange batches = %d, want 1", conversation.batches)
	}
	if got := conversation.messages[1].Content; got != bounded.Content || strings.Contains(got, retainedToolReceiptSchema) {
		t.Fatalf("recorded result is not the bounded rich result: %s", got)
	}
	if conversation.messages[1].ToolCallID != "call-read" || conversation.messages[1].ToolName != "read" ||
		conversation.messages[2].ToolCallID != "call-search" || conversation.messages[2].ToolName != "web_search" ||
		conversation.messages[2].Content != "search evidence" {
		t.Fatalf("parallel results were not restored to source order: %#v", conversation.messages)
	}
	if conversation.messages[1].ToolResult == nil || !conversation.messages[1].ToolResult.ModelTruncated ||
		conversation.messages[1].ToolResult.ContextHints == nil || conversation.messages[1].ToolResult.RetainedContent != "" {
		t.Fatalf("stored rich summary should retain deterministic cleanup metadata: %#v", conversation.messages[1])
	}
}

func TestToolResultContextRecorderFailsClosedOnDuplicateResult(t *testing.T) {
	conversation := &recordedToolContextConversation{policy: ToolResultContextPolicy{Enabled: true}}
	recorder := newToolResultContextRecorder(conversation)
	recorder.RecordAssistantToolCalls(agent.AssistantMessage("", []agent.ToolCall{
		{ID: "call-a", Type: "function", Function: agent.FunctionCall{Name: "read", Arguments: `{}`}},
		{ID: "call-b", Type: "function", Function: agent.FunctionCall{Name: "read", Arguments: `{}`}},
	}), agentEventMetadata{})
	recorder.RecordToolResult(agent.ToolMessage(agent.TextToolResult("first"), "call-a"), agentEventMetadata{})
	recorder.RecordToolResult(agent.ToolMessage(agent.TextToolResult("duplicate"), "call-a"), agentEventMetadata{})
	recorder.RecordToolResult(agent.ToolMessage(agent.TextToolResult("second"), "call-b"), agentEventMetadata{})
	if conversation.batches != 0 || len(conversation.messages) != 0 {
		t.Fatalf("ambiguous result batch must fail closed: %#v", conversation.messages)
	}
}

func TestToolResultContextRecorderSurfacesAtomicPersistenceFailure(t *testing.T) {
	persistErr := errors.New("journal unavailable")
	conversation := &recordedToolContextConversation{
		policy: ToolResultContextPolicy{Enabled: true}, appendErr: persistErr,
	}
	recorder := newToolResultContextRecorder(conversation)
	recorder.RecordAssistantToolCalls(agent.AssistantMessage("inspect", []agent.ToolCall{{
		ID: "call-read", Type: "function", Function: agent.FunctionCall{Name: "read", Arguments: `{"path":"chapter.md"}`},
	}}), agentEventMetadata{})
	err := recorder.RecordToolResult(agent.ToolMessage(agent.TextToolResult("rich result"), "call-read"), agentEventMetadata{})
	if !errors.Is(err, persistErr) || conversation.batches != 0 || len(conversation.messages) != 0 {
		t.Fatalf("persistence failure was not atomic: err=%v batches=%d messages=%#v", err, conversation.batches, conversation.messages)
	}
}

func TestWritingToolResultBatchesRoundTripWithProviderLocalIDReuse(t *testing.T) {
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.Create("multi-tool-context")
	if err != nil {
		t.Fatal(err)
	}
	conversation := NewSessionConversationForAgent(sess, &config.Config{}, AgentKindIDE)
	conversation.BindAgentCycleIdentity(HarnessCycleIdentity{CommandID: "command", OperationID: "operation", Cycle: 1})
	recorder := newToolResultContextRecorder(conversation)

	first := agent.AssistantMessage("first batch", []agent.ToolCall{
		{ID: "provider-local", Type: "function", Function: agent.FunctionCall{Name: "read", Arguments: `{"path":"one.md"}`}},
		{ID: "parallel", Type: "function", Function: agent.FunctionCall{Name: "web_search", Arguments: `{"query":"one"}`}},
	})
	first.ReasoningContent = "private cross-turn reasoning"
	first.ResponseMeta = &agent.ResponseMeta{FinishReason: "tool_calls"}
	first.Extra = map[string]any{"provider_transport": "opaque"}
	recorder.RecordAssistantToolCalls(first, agentEventMetadata{})
	recorder.RecordToolResult(agent.ToolMessage(agent.TextToolResult("search one"), "parallel"), agentEventMetadata{})
	recorder.RecordToolResult(agent.ToolMessage(agent.TextToolResult("file one"), "provider-local"), agentEventMetadata{})

	second := agent.AssistantMessage("second batch", []agent.ToolCall{{
		ID: "provider-local", Type: "function", Function: agent.FunctionCall{Name: "read", Arguments: `{"path":"two.md"}`},
	}})
	recorder.RecordAssistantToolCalls(second, agentEventMetadata{})
	recorder.RecordToolResult(agent.ToolMessage(agent.TextToolResult("file two"), "provider-local"), agentEventMetadata{})

	snapshot, err := sess.SnapshotContext(AgentKindIDE)
	if err != nil {
		t.Fatal(err)
	}
	history := conversation.modelHistory(snapshot)
	if len(history) != 5 || history[0].Content != "first batch" || len(history[0].ToolCalls) != 2 ||
		history[1].ToolCallID != "provider-local" || history[1].Content != "file one" ||
		history[2].ToolCallID != "parallel" || history[2].Content != "search one" ||
		history[3].Content != "second batch" || len(history[3].ToolCalls) != 1 ||
		history[4].ToolCallID != "provider-local" || history[4].Content != "file two" {
		t.Fatalf("writing rich tool batches did not round trip atomically: %#v", history)
	}
	if history[0].ReasoningContent != "" || history[0].ResponseMeta != nil || history[0].Extra != nil {
		t.Fatalf("private reasoning/transport metadata survived cross-turn journal projection: %#v", history[0])
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

func TestApplyToolResultContextPolicyDoesNotUseToolNameCleanupWhitelist(t *testing.T) {
	messages := []*agent.Message{
		agent.AssistantMessage("", []agent.ToolCall{{ID: "call-list", Type: "function", Function: agent.FunctionCall{Name: "list_lore_items", Arguments: `{"keywords":["门"]}`}}}),
		agent.ToolMessage(agent.TextToolResult("很长的资料索引"), "call-list", agent.WithToolName("list_lore_items")),
		agent.AssistantMessage("继续故事", nil),
	}
	filtered := applyToolResultContextPolicy(messages, ToolResultContextPolicy{Enabled: true})
	if len(filtered) != 3 || filtered[1].Content != "很长的资料索引" || filtered[2].Content != "继续故事" {
		t.Fatalf("tool-name policy must not clear rich results at turn boundaries: %#v", filtered)
	}
}

func TestInteractiveStoryToolContextKeepsAllBoundedRichPairsUntilPressureCleanup(t *testing.T) {
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
	if len(filtered) != 4 || len(filtered[0].ToolCalls) != 3 || filtered[1].Content != `{"outcome":"success"}` || filtered[2].Content != "文风正文" {
		t.Fatalf("game context must keep bounded rich pairs until shared pressure cleanup: %#v", filtered)
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
	if len(filtered) != 3 || len(filtered[0].ToolCalls) != 2 {
		t.Fatalf("all valid rich call/result pairs should remain: %#v", filtered)
	}
	if filtered[1].ToolName != "list_lore_items" || filtered[2].ToolName != "read_lore_items" || filtered[2].Content == "" {
		t.Fatalf("results should inherit paired tool names without immediate receipt conversion: %#v", filtered)
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

func TestApplyToolResultContextPolicyScopesReusedCallIDsToAssistantBatch(t *testing.T) {
	messages := []*agent.Message{
		agent.AssistantMessage("", []agent.ToolCall{{
			ID: "provider-local", Type: "function",
			Function: agent.FunctionCall{Name: "read", Arguments: `{"path":"one.md"}`},
		}}),
		agent.ToolMessage(agent.TextToolResult("first rich result"), "provider-local"),
		agent.UserMessage("next turn"),
		agent.AssistantMessage("", []agent.ToolCall{{
			ID: "provider-local", Type: "function",
			Function: agent.FunctionCall{Name: "web_fetch", Arguments: `{"url":"https://example.com"}`},
		}}),
		agent.ToolMessage(agent.TextToolResult("second rich result"), "provider-local"),
	}

	filtered := applyToolResultContextPolicy(messages, ToolResultContextPolicy{Enabled: true})
	if len(filtered) != len(messages) {
		t.Fatalf("provider-local call ID reuse removed valid exchanges: %#v", filtered)
	}
	if filtered[1].ToolName != "read" || filtered[1].Content != "first rich result" ||
		filtered[4].ToolName != "web_fetch" || filtered[4].Content != "second rich result" {
		t.Fatalf("reused call ID paired results across assistant batches: %#v", filtered)
	}
}

func TestApplyToolResultContextPolicyKeepsAmbiguityLocalToOneBatch(t *testing.T) {
	messages := []*agent.Message{
		agent.AssistantMessage("", []agent.ToolCall{
			{ID: "provider-local", Type: "function", Function: agent.FunctionCall{Name: "read", Arguments: `{}`}},
			{ID: "provider-local", Type: "function", Function: agent.FunctionCall{Name: "write", Arguments: `{}`}},
		}),
		agent.ToolMessage(agent.TextToolResult("ambiguous"), "provider-local"),
		agent.UserMessage("next turn"),
		agent.AssistantMessage("", []agent.ToolCall{{
			ID: "provider-local", Type: "function", Function: agent.FunctionCall{Name: "read", Arguments: `{}`},
		}}),
		agent.ToolMessage(agent.TextToolResult("unambiguous"), "provider-local"),
	}

	filtered := applyToolResultContextPolicy(messages, ToolResultContextPolicy{Enabled: true})
	if len(filtered) != 3 || filtered[0].Role != agent.User || filtered[1].Role != agent.Assistant ||
		len(filtered[1].ToolCalls) != 1 || filtered[2].ToolCallID != "provider-local" ||
		filtered[2].Content != "unambiguous" {
		t.Fatalf("ambiguous batch poisoned a later valid ID occurrence: %#v", filtered)
	}
}

func TestToolResultContextKeepsLoreErrorsInsteadOfPositiveReceipt(t *testing.T) {
	raw := "读取资料失败：条目不存在"
	descriptor := producttools.BoundedReadDescriptor(agent.ToolSourceLore, config.AgentToolLoreRead)
	filtered := filterStructuredToolResultWithDescriptor(
		"read_lore_items", descriptor, `{}`, agent.ToolErrorResult(raw, raw), 0,
	)
	if filtered.Result.Status != agent.ToolResultError || filtered.Result.ModelContent != raw || filtered.Result.RetainedContent != "" {
		t.Fatalf("failed lore read must remain a protected rich error: %#v", filtered.Result)
	}
}

type recordedToolContextConversation struct {
	Conversation
	messages  []*agent.Message
	policy    ToolResultContextPolicy
	batches   int
	appendErr error
}

func (c *recordedToolContextConversation) AppendContextMessages(messages ...*agent.Message) error {
	if c.appendErr != nil {
		return c.appendErr
	}
	c.batches++
	c.messages = append(c.messages, messages...)
	return nil
}

func (c *recordedToolContextConversation) ToolResultContextPolicy() ToolResultContextPolicy {
	return c.policy
}
