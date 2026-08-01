package chat

import (
	"context"
	agentconversation "denova/internal/agents/conversation"
	agentrun "denova/internal/agents/run"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	agent "github.com/alfredxw/denova/agent"
	"github.com/alfredxw/denova/agent/providers"
	"github.com/alfredxw/denova/agent/providers/builtin"

	"denova/config"
	"denova/internal/agents/session"
	"denova/internal/agents/toolresult"
	producttools "denova/internal/agents/tools"
)

func TestApplyToolResultContextPolicyKeepsBoundedRichExchangeAcrossTurns(t *testing.T) {
	arguments := "  {\"path\":\"chapter.md\",\"selection\":\"" + strings.Repeat("段落", 3000) + "\"}  "
	content := "{\"items\":[" + strings.Repeat("{\"name\":\"条目\"},", 3000) + "\nnot-valid-inner-json"
	descriptor := producttools.BoundedReadDescriptor(agent.ToolSourceRead, config.AgentToolWorkspaceRead)
	projected := toolresult.FilterStructured("read", descriptor, arguments, agent.TextToolResult(content), 0)
	messages := []*agent.Message{
		agent.UserMessage("读取资料"),
		agent.AssistantMessage("", []agent.ToolCall{{
			ID: "call-large", Type: "function",
			Function: agent.FunctionCall{Name: "read", Arguments: arguments},
		}}),
		agent.ToolMessage(projected.Result, "call-large", agent.WithToolName("read")),
		agent.UserMessage("继续"),
	}

	filtered := toolresult.ApplyContextPolicy(messages, toolresult.ContextPolicy{Enabled: true})
	if len(filtered) != len(messages) {
		t.Fatalf("complete tool exchange should remain in context: got=%d want=%d", len(filtered), len(messages))
	}
	if got := filtered[1].ToolCalls[0].Function.Arguments; got != arguments {
		t.Fatalf("tool arguments changed before pressure cleanup: got=%q", got)
	}
	if got := filtered[2].Content; got != projected.Result.ModelContent || strings.Contains(got, toolresult.ReceiptSchema) {
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
	projected := toolresult.FilterStructured("read", descriptor, arguments, agent.TextToolResult(content), 0)
	messages := toolresult.ApplyContextPolicy([]*agent.Message{
		agent.AssistantMessage("", []agent.ToolCall{{
			ID: "call-json", Type: "function",
			Function: agent.FunctionCall{Name: "read", Arguments: arguments},
		}}),
		agent.ToolMessage(projected.Result, "call-json", agent.WithToolName("read")),
		agent.UserMessage("基于结果继续"),
	}, toolresult.ContextPolicy{Enabled: true})

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

	registry, err := builtin.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	chatModel, err := registry.NewChatModel(context.Background(), providers.ModelConfig{
		Provider: providers.ProviderOpenAICompatible,
		Protocol: providers.ProtocolOpenAIChatCompletions,
		APIKey:   "test-key",
		Model:    "test-model",
		BaseURL:  server.URL,
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
		toolMessage.Content != projected.Result.ModelContent || strings.Contains(toolMessage.Content, toolresult.ReceiptSchema) {
		t.Fatalf("tool content must be one opaque JSON string, got role=%q id=%q bytes=%d", toolMessage.Role, toolMessage.ToolCallID, len(toolMessage.Content))
	}
}

func TestResponsesContinuationSurvivesProductSessionReload(t *testing.T) {
	requests := make(chan []byte, 2)
	call := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Errorf("read Responses request: %v", err)
		}
		requests <- body
		call++
		writer.Header().Set("Content-Type", "application/json")
		if call == 1 {
			_, _ = writer.Write([]byte(`{
				"id":"resp_1","object":"response","created_at":1,"status":"completed","model":"test-model",
				"output":[
					{"id":"rs_1","type":"reasoning","summary":[],"content":[],"encrypted_content":"encrypted-state","status":"completed"},
					{"id":"fc_1","type":"function_call","call_id":"call_1","name":"read","arguments":"{\"path\":\"chapter.md\"}","status":"completed"}
				],
				"usage":{"input_tokens":1,"input_tokens_details":{"cached_tokens":0},"output_tokens":1,"output_tokens_details":{"reasoning_tokens":1},"total_tokens":2}
			}`))
			return
		}
		_, _ = writer.Write([]byte(`{
			"id":"resp_2","object":"response","created_at":2,"status":"completed","model":"test-model",
			"output":[{"id":"msg_2","type":"message","status":"completed","role":"assistant","content":[{"type":"output_text","text":"done","annotations":[],"logprobs":[]}]}],
			"usage":{"input_tokens":2,"input_tokens_details":{"cached_tokens":1},"output_tokens":1,"output_tokens_details":{"reasoning_tokens":0},"total_tokens":3}
		}`))
	}))
	t.Cleanup(server.Close)

	registry, err := builtin.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	modelConfig := providers.ModelConfig{
		Provider: providers.ProviderOpenAICompatible, Protocol: providers.ProtocolOpenAIResponses,
		APIKey: "test-key", Model: "test-model", BaseURL: server.URL + "/v1", HTTPClient: server.Client(),
	}
	model, err := registry.NewChatModel(context.Background(), modelConfig)
	if err != nil {
		t.Fatal(err)
	}
	first, err := model.Generate(context.Background(), []*agent.Message{agent.UserMessage("inspect")})
	if err != nil {
		t.Fatal(err)
	}
	<-requests

	workspace := t.TempDir()
	store, err := session.NewStore(workspace)
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.GetOrCreate("responses-reload")
	if err != nil {
		t.Fatal(err)
	}
	conversation := agentconversation.NewSessionConversationForAgent(sess, &config.Config{}, agentrun.AgentKindIDE)
	conversation.BindAgentCycleIdentity(agentrun.CycleIdentity{CommandID: "command", OperationID: "operation", Cycle: 1})
	recorder := newToolResultContextRecorder(conversation)
	recorder.RecordAssistantToolCalls(first, agentEventMetadata{})
	if err := recorder.RecordToolResult(agent.ToolMessage(agent.TextToolResult("chapter contents"), "call_1", agent.WithToolName("read")), agentEventMetadata{}); err != nil {
		t.Fatal(err)
	}

	reloadedStore, err := session.NewStore(workspace)
	if err != nil {
		t.Fatal(err)
	}
	reloaded, err := reloadedStore.Get("responses-reload")
	if err != nil {
		t.Fatal(err)
	}
	history := append(reloaded.GetEffectiveMessages(), agent.UserMessage("continue"))
	if _, err := model.Generate(context.Background(), history); err != nil {
		t.Fatal(err)
	}

	var secondRequest struct {
		Input []map[string]any `json:"input"`
	}
	if err := json.Unmarshal(<-requests, &secondRequest); err != nil {
		t.Fatal(err)
	}
	if len(secondRequest.Input) != 4 {
		t.Fatalf("reloaded Responses input = %#v", secondRequest.Input)
	}
	wantTypes := []string{"reasoning", "function_call", "function_call_output"}
	for index, want := range wantTypes {
		if got := secondRequest.Input[index]["type"]; got != want {
			t.Fatalf("reloaded input[%d] type = %#v, want %q", index, got, want)
		}
	}
	if secondRequest.Input[0]["encrypted_content"] != "encrypted-state" || secondRequest.Input[3]["role"] != "user" {
		t.Fatalf("reloaded continuation lost state or user turn: %#v", secondRequest.Input)
	}
}

func TestApplyToolResultContextPolicyDisabledRemovesToolContext(t *testing.T) {
	messages := []*agent.Message{
		agent.UserMessage("查资料"),
		agent.AssistantMessage("", []agent.ToolCall{{ID: "call-1", Type: "function", Function: agent.FunctionCall{Name: "read", Arguments: `{}`}}}),
		agent.ToolMessage(agent.TextToolResult("result"), "call-1", agent.WithToolName("read")),
		agent.AssistantMessage("完成", nil),
	}

	filtered := toolresult.ApplyContextPolicy(messages, toolresult.ContextPolicy{Enabled: false})
	if len(filtered) != 2 || filtered[0].Role != agent.User || filtered[1].Content != "完成" {
		t.Fatalf("disabled retention should remove context-only tool messages: %#v", filtered)
	}
}

func TestDescriptorControlsRetentionForFutureToolsAndRedactsSensitiveArguments(t *testing.T) {
	arguments := `{"query":"stable","access_token":"do-not-retain"}`
	descriptor := producttools.BoundedReadDescriptor(agent.ToolSourceWeb, "future.read")
	result := toolresult.FilterStructured(
		"future_lookup", descriptor, arguments, agent.TextToolResult("large future result"), 0,
	).Result
	messages := []*agent.Message{
		agent.AssistantMessage("", []agent.ToolCall{{
			ID: "future-call", Type: "function",
			Function: agent.FunctionCall{Name: "future_lookup", Arguments: arguments},
		}}),
		agent.ToolMessage(result, "future-call", agent.WithToolName("future_lookup")),
	}
	retained := toolresult.ApplyContextPolicy(messages, toolresult.ContextPolicy{
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
	transient := toolresult.FilterStructured(
		"future_lookup", descriptor, arguments, agent.TextToolResult("transient result"), 0,
	).Result
	messages[1] = agent.ToolMessage(transient, "future-call", agent.WithToolName("future_lookup"))
	if filtered := toolresult.ApplyContextPolicy(messages, toolresult.ContextPolicy{Enabled: true}); len(filtered) != 2 || filtered[1].Content != "transient result" {
		t.Fatalf("turn age must not immediately clear a bounded result: %#v", filtered)
	}
}

func TestToolResultContextRecorderPersistsAtomicBoundedRichBatchInSourceOrder(t *testing.T) {
	conversation := &recordedToolContextConversation{policy: toolresult.ContextPolicy{Enabled: true, MaxResultBytes: 4096}}
	recorder := newToolResultContextRecorder(conversation)
	readArguments := `{"path":"chapter.md"}`
	searchArguments := `{"query":"theme"}`
	assistant := agent.AssistantMessage("I will inspect both sources.", []agent.ToolCall{
		{ID: "call-read", Type: "function", Function: agent.FunctionCall{Name: "read", Arguments: readArguments}},
		{ID: "call-search", Type: "function", Function: agent.FunctionCall{Name: "web_search", Arguments: searchArguments}},
	})
	assistant.ResponseMeta = &agent.ResponseMeta{FinishReason: "tool_calls"}
	continuationConfig := providers.ModelConfig{
		Provider: providers.ProviderOpenAI, Protocol: providers.ProtocolOpenAIResponses,
		Model: "gpt-5", BaseURL: "https://api.openai.com/v1",
	}
	continuation, err := providers.NewContinuation(continuationConfig, []any{
		map[string]any{"type": "reasoning", "encrypted_content": "encrypted-state"},
	})
	if err != nil {
		t.Fatal(err)
	}
	assistant.Extra = map[string]any{
		"provider_response_id":         "response-1",
		providers.ExtraKeyContinuation: continuation,
	}
	assistant.ReasoningContent = "private chain of thought must not cross turns"
	assistant.MultiContent = []json.RawMessage{json.RawMessage(`{"type":"reasoning","text":"private"}`)}
	assistant.AssistantGenMultiContent = []json.RawMessage{json.RawMessage(`{"type":"reasoning","text":"private"}`)}
	recorder.RecordAssistantToolCalls(assistant, agentEventMetadata{})

	bounded := toolresult.FilterWithLimit("read", readArguments, strings.Repeat("正文", 5000), 4096)
	bounded.Result.ResultRetention = agent.ToolResultDeferred
	bounded.Result.ContextHints = &agent.ToolResultContextHints{
		Recovery: agent.ToolResultRecoveryHint{
			Kind: agent.ToolResultRecoveryRead, Reference: map[string]any{"path": "chapter.md"},
			EstimatedBytes: int64(bounded.OriginalBytes), EstimatedTokens: toolresult.EstimatedTokens(int64(bounded.OriginalBytes)),
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
	if recordedAssistant.ReasoningContent != "" || recordedAssistant.ResponseMeta != nil ||
		len(recordedAssistant.MultiContent) != 0 || len(recordedAssistant.AssistantGenMultiContent) != 0 {
		t.Fatalf("private reasoning or transport metadata crossed the durable tool boundary: %#v", recordedAssistant)
	}
	var persistedOutput []any
	matched, err := providers.DecodeContinuation(recordedAssistant.Extra, continuationConfig, &persistedOutput)
	if err != nil || !matched || len(persistedOutput) != 1 || len(recordedAssistant.Extra) != 1 {
		t.Fatalf("protocol continuation was not isolated: extra=%#v matched=%t err=%v", recordedAssistant.Extra, matched, err)
	}
	if conversation.batches != 1 {
		t.Fatalf("tool exchange batches = %d, want 1", conversation.batches)
	}
	if got := conversation.messages[1].Content; got != bounded.Content || strings.Contains(got, toolresult.ReceiptSchema) {
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
	conversation := &recordedToolContextConversation{policy: toolresult.ContextPolicy{Enabled: true}}
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
		policy: toolresult.ContextPolicy{Enabled: true}, appendErr: persistErr,
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
	conversation := agentconversation.NewSessionConversationForAgent(sess, &config.Config{}, agentrun.AgentKindIDE)
	conversation.BindAgentCycleIdentity(agentrun.CycleIdentity{CommandID: "command", OperationID: "operation", Cycle: 1})
	recorder := newToolResultContextRecorder(conversation)

	first := agent.AssistantMessage("first batch", []agent.ToolCall{
		{ID: "provider-local", Type: "function", Function: agent.FunctionCall{Name: "read", Arguments: `{"path":"one.md"}`}},
		{ID: "parallel", Type: "function", Function: agent.FunctionCall{Name: "web_search", Arguments: `{"query":"one"}`}},
	})
	first.ReasoningContent = "private cross-turn reasoning"
	first.ResponseMeta = &agent.ResponseMeta{FinishReason: "tool_calls"}
	continuationConfig := providers.ModelConfig{
		Provider: providers.ProviderOpenAI, Protocol: providers.ProtocolOpenAIResponses,
		Model: "gpt-5", BaseURL: "https://api.openai.com/v1",
	}
	continuation, err := providers.NewContinuation(continuationConfig, []any{
		map[string]any{"type": "reasoning", "encrypted_content": "encrypted-state"},
	})
	if err != nil {
		t.Fatal(err)
	}
	first.Extra = map[string]any{
		"provider_transport":           "opaque",
		providers.ExtraKeyContinuation: continuation,
	}
	recorder.RecordAssistantToolCalls(first, agentEventMetadata{})
	recorder.RecordToolResult(agent.ToolMessage(agent.TextToolResult("search one"), "parallel"), agentEventMetadata{})
	recorder.RecordToolResult(agent.ToolMessage(agent.TextToolResult("file one"), "provider-local"), agentEventMetadata{})

	second := agent.AssistantMessage("second batch", []agent.ToolCall{{
		ID: "provider-local", Type: "function", Function: agent.FunctionCall{Name: "read", Arguments: `{"path":"two.md"}`},
	}})
	recorder.RecordAssistantToolCalls(second, agentEventMetadata{})
	recorder.RecordToolResult(agent.ToolMessage(agent.TextToolResult("file two"), "provider-local"), agentEventMetadata{})

	projection, err := conversation.SnapshotContextCompaction(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	history := projection.Messages
	if len(history) != 5 || history[0].Content != "first batch" || len(history[0].ToolCalls) != 2 ||
		history[1].ToolCallID != "provider-local" || history[1].Content != "file one" ||
		history[2].ToolCallID != "parallel" || history[2].Content != "search one" ||
		history[3].Content != "second batch" || len(history[3].ToolCalls) != 1 ||
		history[4].ToolCallID != "provider-local" || history[4].Content != "file two" {
		t.Fatalf("writing rich tool batches did not round trip atomically: %#v", history)
	}
	if history[0].ReasoningContent != "" || history[0].ResponseMeta != nil {
		t.Fatalf("private reasoning/transport metadata survived cross-turn journal projection: %#v", history[0])
	}
	var persistedOutput []any
	matched, err := providers.DecodeContinuation(history[0].Extra, continuationConfig, &persistedOutput)
	if err != nil || !matched || len(persistedOutput) != 1 || len(history[0].Extra) != 1 {
		t.Fatalf("cross-turn protocol continuation was not preserved: extra=%#v matched=%t err=%v", history[0].Extra, matched, err)
	}
}

func TestToolResultContextRecorderSkipsMalformedCallAndResult(t *testing.T) {
	conversation := &recordedToolContextConversation{policy: toolresult.ContextPolicy{Enabled: true}}
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
	filtered := toolresult.ApplyContextPolicy(messages, toolresult.ContextPolicy{Enabled: true})
	if len(filtered) != 3 || filtered[0].Content != "useful narration" || len(filtered[0].ToolCalls) != 1 ||
		filtered[0].ToolCalls[0].ID != "missing" || filtered[1].Role != agent.ToolRole ||
		filtered[1].ToolCallID != "missing" || !toolresult.IsUnknownEffectResult(filtered[1].Content) || filtered[2].Role != agent.User {
		t.Fatalf("malformed/orphaned protocol must be dropped while a unique missing result is recovered: %#v", filtered)
	}
}

func TestApplyToolResultContextPolicyDoesNotUseToolNameCleanupWhitelist(t *testing.T) {
	messages := []*agent.Message{
		agent.AssistantMessage("", []agent.ToolCall{{ID: "call-list", Type: "function", Function: agent.FunctionCall{Name: "list_lore_items", Arguments: `{"keywords":["门"]}`}}}),
		agent.ToolMessage(agent.TextToolResult("很长的资料索引"), "call-list", agent.WithToolName("list_lore_items")),
		agent.AssistantMessage("继续故事", nil),
	}
	filtered := toolresult.ApplyContextPolicy(messages, toolresult.ContextPolicy{Enabled: true})
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
	filtered := toolresult.ApplyContextPolicy(messages, toolresult.ContextPolicy{AgentKind: config.AgentKindInteractiveStory, Enabled: true})
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
	filtered := toolresult.ApplyContextPolicy(messages, toolresult.ContextPolicy{Enabled: true})
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
	if filtered := toolresult.ApplyContextPolicy(messages, toolresult.ContextPolicy{Enabled: true}); len(filtered) != 0 {
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

	filtered := toolresult.ApplyContextPolicy(messages, toolresult.ContextPolicy{Enabled: true})
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

	filtered := toolresult.ApplyContextPolicy(messages, toolresult.ContextPolicy{Enabled: true})
	if len(filtered) != 3 || filtered[0].Role != agent.User || filtered[1].Role != agent.Assistant ||
		len(filtered[1].ToolCalls) != 1 || filtered[2].ToolCallID != "provider-local" ||
		filtered[2].Content != "unambiguous" {
		t.Fatalf("ambiguous batch poisoned a later valid ID occurrence: %#v", filtered)
	}
}

func TestToolResultContextKeepsLoreErrorsInsteadOfPositiveReceipt(t *testing.T) {
	raw := "读取资料失败：条目不存在"
	descriptor := producttools.BoundedReadDescriptor(agent.ToolSourceLore, config.AgentToolLoreRead)
	filtered := toolresult.FilterStructured(
		"read_lore_items", descriptor, `{}`, agent.ToolErrorResult(raw, raw), 0,
	)
	if filtered.Result.Status != agent.ToolResultError || filtered.Result.ModelContent != raw || filtered.Result.RetainedContent != "" {
		t.Fatalf("failed lore read must remain a protected rich error: %#v", filtered.Result)
	}
}

type recordedToolContextConversation struct {
	Conversation
	messages  []*agent.Message
	policy    toolresult.ContextPolicy
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

func (c *recordedToolContextConversation) ToolResultContextPolicy() toolresult.ContextPolicy {
	return c.policy
}
