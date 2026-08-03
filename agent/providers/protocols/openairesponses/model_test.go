package openairesponses

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	agent "github.com/alfredxw/denova/agent"
	"github.com/alfredxw/denova/agent/providers"
	"github.com/openai/openai-go/v3/responses"
	"github.com/openai/openai-go/v3/shared"
)

func TestApplyThinkingLevelCoversOpenAIResponsesEfforts(t *testing.T) {
	compatibility, err := resolveCompatibility(providers.ModelConfig{ProtocolOptions: mustProtocolOptions(t, Compatibility{
		ReasoningSummary: ReasoningSummaryAuto,
	})})
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		level       providers.ThinkingLevel
		wantEffort  shared.ReasoningEffort
		wantSummary shared.ReasoningSummary
	}{
		{level: providers.ThinkingLevelDefault},
		{level: providers.ThinkingLevelOff, wantEffort: shared.ReasoningEffortNone},
		{level: providers.ThinkingLevelMinimal, wantEffort: shared.ReasoningEffortMinimal, wantSummary: shared.ReasoningSummaryAuto},
		{level: providers.ThinkingLevelLow, wantEffort: shared.ReasoningEffortLow, wantSummary: shared.ReasoningSummaryAuto},
		{level: providers.ThinkingLevelMedium, wantEffort: shared.ReasoningEffortMedium, wantSummary: shared.ReasoningSummaryAuto},
		{level: providers.ThinkingLevelHigh, wantEffort: shared.ReasoningEffortHigh, wantSummary: shared.ReasoningSummaryAuto},
		{level: providers.ThinkingLevelXHigh, wantEffort: shared.ReasoningEffortXhigh, wantSummary: shared.ReasoningSummaryAuto},
		{level: providers.ThinkingLevelMax, wantEffort: shared.ReasoningEffortMax, wantSummary: shared.ReasoningSummaryAuto},
	}
	for _, test := range tests {
		t.Run(string(test.level), func(t *testing.T) {
			params := responses.ResponseNewParams{}
			applyThinkingLevel(&params, compatibility, test.level)
			if params.Reasoning.Effort != test.wantEffort || params.Reasoning.Summary != test.wantSummary {
				t.Fatalf("reasoning = %#v, want effort %q summary %q", params.Reasoning, test.wantEffort, test.wantSummary)
			}
		})
	}
}

func mustProtocolOptions(t *testing.T, compatibility Compatibility) json.RawMessage {
	t.Helper()
	options, err := providers.EncodeProtocolOptions(compatibility)
	if err != nil {
		t.Fatal(err)
	}
	return options
}

func TestGenerateMapsRequestResponseAndReplaysOutputItems(t *testing.T) {
	requests := make(chan []byte, 2)
	var callCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/responses" {
			t.Errorf("request path = %q", request.URL.Path)
		}
		if got := request.Header.Get("Authorization"); got != "Bearer secret" {
			t.Errorf("authorization = %q", got)
		}
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Errorf("read request: %v", err)
		}
		requests <- body
		writer.Header().Set("Content-Type", "application/json")
		writer.Header().Set("X-Request-ID", fmt.Sprintf("req-%d", callCount.Load()+1))
		if callCount.Add(1) == 1 {
			_, _ = io.WriteString(writer, `{
  "id":"resp_1","object":"response","created_at":1700000000,"status":"completed","model":"provider-model",
  "output":[
    {"id":"reason_1","type":"reasoning","summary":[{"type":"summary_text","text":"checked facts"}],"content":[],"encrypted_content":"encrypted-state","status":"completed"},
    {"id":"message_1","type":"message","status":"completed","role":"assistant","content":[{"type":"output_text","text":"answer","annotations":[],"logprobs":[]}]},
    {"id":"function_1","type":"function_call","call_id":"call_new","name":"lookup","arguments":"{\"q\":\"new\"}","status":"completed"}
  ],
  "usage":{"input_tokens":11,"input_tokens_details":{"cached_tokens":5},"output_tokens":7,"output_tokens_details":{"reasoning_tokens":3},"total_tokens":18}
}`)
			return
		}
		_, _ = io.WriteString(writer, `{
  "id":"resp_2","object":"response","created_at":1700000001,"status":"completed","model":"provider-model",
  "output":[{"id":"message_2","type":"message","status":"completed","role":"assistant","content":[{"type":"output_text","text":"done","annotations":[],"logprobs":[]}]}],
  "usage":{"input_tokens":20,"input_tokens_details":{"cached_tokens":10},"output_tokens":2,"output_tokens_details":{"reasoning_tokens":0},"total_tokens":22}
}`)
	}))
	defer server.Close()

	temperature := float32(0.25)
	configuredMaxTokens := 321
	modelConfig := providers.ModelConfig{
		Provider:        providers.ProviderOpenAI,
		Protocol:        providers.ProtocolOpenAIResponses,
		APIKey:          "secret",
		Model:           "test-model",
		BaseURL:         server.URL + "/v1",
		HTTPClient:      server.Client(),
		Temperature:     &temperature,
		MaxOutputTokens: &configuredMaxTokens,
		ThinkingLevel:   providers.ThinkingLevelHigh,
		OutputFormat:    &providers.OutputFormat{Type: providers.OutputFormatJSONObject},
		ProtocolOptions: mustProtocolOptions(t, Compatibility{
			Store:                     StoreModeFalse,
			IncludeEncryptedReasoning: true,
			ReasoningSummary:          ReasoningSummaryAuto,
		}),
	}
	model, err := NewAdapter().New(context.Background(), modelConfig)
	if err != nil {
		t.Fatalf("new model: %v", err)
	}
	bound, err := model.WithTools([]*agent.ToolInfo{{
		Name: "lookup",
		Desc: "Lookup",
		ParamsOneOf: agent.NewParamsOneOfByParams(map[string]*agent.ParameterInfo{
			"z": {Type: agent.Integer, Required: true},
			"a": {Type: agent.String, Required: true},
		}),
	}})
	if err != nil {
		t.Fatalf("bind tools: %v", err)
	}

	first, err := bound.Generate(context.Background(), []*agent.Message{
		agent.SystemMessage("sys"),
		agent.UserMessage("hello"),
	}, agent.WithMaxTokens(99), agent.WithToolChoice(agent.ToolChoiceAllowed, "lookup"))
	if err != nil {
		t.Fatalf("first generate: %v", err)
	}
	if first.Content != "answer" || first.ReasoningContent != "checked facts" || len(first.ToolCalls) != 1 {
		t.Fatalf("first response = %#v", first)
	}
	if first.ToolCalls[0].ID != "call_new" || first.ToolCalls[0].Function.Arguments != `{"q":"new"}` {
		t.Fatalf("tool call = %#v", first.ToolCalls)
	}
	if first.ResponseMeta == nil || first.ResponseMeta.FinishReason != "tool_calls" || first.ResponseMeta.Usage == nil {
		t.Fatalf("response metadata = %#v", first.ResponseMeta)
	}
	wantUsage := &agent.TokenUsage{
		PromptTokens:       11,
		CompletionTokens:   7,
		TotalTokens:        18,
		PromptTokenDetails: agent.PromptTokenDetails{CachedTokens: 5},
		CompletionTokensDetails: agent.CompletionTokensDetails{
			ReasoningTokens: 3,
		},
	}
	if !reflect.DeepEqual(first.ResponseMeta.Usage, wantUsage) {
		t.Fatalf("usage = %#v, want %#v", first.ResponseMeta.Usage, wantUsage)
	}
	if first.Extra[ExtraKeyProvider] != string(providers.ProviderOpenAI) ||
		first.Extra[ExtraKeyProtocol] != string(providers.ProtocolOpenAIResponses) ||
		first.Extra[ExtraKeyRequestID] != "req-1" {
		t.Fatalf("response identity = %#v", first.Extra)
	}
	var outputItems []any
	matched, err := providers.DecodeContinuation(first.Extra, modelConfig, &outputItems)
	if err != nil || !matched || len(outputItems) != 3 {
		t.Fatalf("stored output items = %#v matched=%t err=%v", outputItems, matched, err)
	}

	second, err := bound.Generate(context.Background(), []*agent.Message{
		first,
		agent.ToolMessage(agent.TextToolResult(`{"ok":true}`), "call_new", agent.WithToolName("lookup")),
		agent.UserMessage("continue"),
	})
	if err != nil {
		t.Fatalf("second generate: %v", err)
	}
	if second.Content != "done" {
		t.Fatalf("second response = %#v", second)
	}

	var firstRequest map[string]any
	if err := json.Unmarshal(<-requests, &firstRequest); err != nil {
		t.Fatalf("decode first request: %v", err)
	}
	if firstRequest["model"] != "test-model" || firstRequest["store"] != false || firstRequest["max_output_tokens"] != float64(99) {
		t.Fatalf("first request config = %#v", firstRequest)
	}
	include, _ := firstRequest["include"].([]any)
	if len(include) != 1 || include[0] != "reasoning.encrypted_content" {
		t.Fatalf("include = %#v", firstRequest["include"])
	}
	reasoning, _ := firstRequest["reasoning"].(map[string]any)
	if reasoning["effort"] != "high" || reasoning["summary"] != "auto" {
		t.Fatalf("reasoning = %#v", reasoning)
	}
	textConfig, _ := firstRequest["text"].(map[string]any)
	format, _ := textConfig["format"].(map[string]any)
	if format["type"] != "json_object" {
		t.Fatalf("text format = %#v", textConfig)
	}
	tools, _ := firstRequest["tools"].([]any)
	if len(tools) != 1 {
		t.Fatalf("tools = %#v", firstRequest["tools"])
	}
	tool := tools[0].(map[string]any)
	if tool["type"] != "function" || tool["name"] != "lookup" || tool["description"] != "Lookup" {
		t.Fatalf("tool = %#v", tool)
	}
	parameters := tool["parameters"].(map[string]any)
	if !reflect.DeepEqual(parameters["required"], []any{"a", "z"}) {
		t.Fatalf("required parameters = %#v", parameters["required"])
	}

	var secondRequest map[string]any
	if err := json.Unmarshal(<-requests, &secondRequest); err != nil {
		t.Fatalf("decode second request: %v", err)
	}
	input := secondRequest["input"].([]any)
	if len(input) != 5 {
		t.Fatalf("replayed input length = %d: %#v", len(input), input)
	}
	wantTypes := []string{"reasoning", "message", "function_call", "function_call_output"}
	for index, want := range wantTypes {
		item := input[index].(map[string]any)
		if item["type"] != want {
			t.Fatalf("input[%d] type = %#v, want %q", index, item["type"], want)
		}
	}
	if input[4].(map[string]any)["role"] != "user" {
		t.Fatalf("last input is not the next user message: %#v", input[4])
	}
	if input[0].(map[string]any)["encrypted_content"] != "encrypted-state" {
		t.Fatalf("encrypted reasoning was not replayed: %#v", input[0])
	}
	if input[3].(map[string]any)["output"] != `{"ok":true}` {
		t.Fatalf("tool output changed type or content: %#v", input[3])
	}
}

func TestStreamMapsReasoningTextToolCallsAndUsage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "text/event-stream")
		writer.Header().Set("X-Request-ID", "req-stream")
		flusher := writer.(http.Flusher)
		frames := []string{
			`{"type":"response.created","sequence_number":0,"response":{"id":"resp_stream","object":"response","created_at":2,"status":"in_progress","model":"stream-model","output":[]}}`,
			`{"type":"response.reasoning_summary_text.delta","sequence_number":1,"item_id":"reason_1","output_index":0,"summary_index":0,"delta":"think "}`,
			`{"type":"response.output_item.added","sequence_number":2,"output_index":1,"item":{"id":"function_item","type":"function_call","call_id":"call_1","name":"lookup","arguments":"","status":"in_progress"}}`,
			`{"type":"response.function_call_arguments.delta","sequence_number":3,"item_id":"function_item","output_index":1,"delta":"{\"q\":\""}`,
			`{"type":"response.function_call_arguments.delta","sequence_number":4,"item_id":"function_item","output_index":1,"delta":"x\"}"}`,
			`{"type":"response.output_item.added","sequence_number":5,"output_index":2,"item":{"id":"message_item","type":"message","status":"in_progress","role":"assistant","content":[]}}`,
			`{"type":"response.output_text.delta","sequence_number":6,"item_id":"message_item","output_index":2,"content_index":0,"delta":"Hel","logprobs":[]}`,
			`{"type":"response.output_text.delta","sequence_number":7,"item_id":"message_item","output_index":2,"content_index":0,"delta":"lo","logprobs":[]}`,
			`{"type":"response.completed","sequence_number":8,"response":{"id":"resp_stream","object":"response","created_at":2,"status":"completed","model":"stream-model","output":[{"id":"reason_1","type":"reasoning","summary":[{"type":"summary_text","text":"think "}],"content":[],"encrypted_content":"enc","status":"completed"},{"id":"function_item","type":"function_call","call_id":"call_1","name":"lookup","arguments":"{\"q\":\"x\"}","status":"completed"},{"id":"message_item","type":"message","status":"completed","role":"assistant","content":[{"type":"output_text","text":"Hello","annotations":[],"logprobs":[]}]}],"usage":{"input_tokens":8,"input_tokens_details":{"cached_tokens":4},"output_tokens":6,"output_tokens_details":{"reasoning_tokens":2},"total_tokens":14}}}`,
		}
		for _, frame := range frames {
			_, _ = fmt.Fprintf(writer, "data: %s\n\n", frame)
			flusher.Flush()
		}
	}))
	defer server.Close()

	modelConfig := providers.ModelConfig{
		Provider:   providers.ProviderOpenAI,
		Protocol:   providers.ProtocolOpenAIResponses,
		Model:      "m",
		BaseURL:    server.URL,
		HTTPClient: server.Client(),
	}
	model, err := NewAdapter().New(context.Background(), modelConfig)
	if err != nil {
		t.Fatal(err)
	}
	stream, err := model.Stream(context.Background(), []*agent.Message{agent.UserMessage("hello")})
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	var chunks []*agent.Message
	for {
		chunk, receiveErr := stream.Recv()
		if errors.Is(receiveErr, io.EOF) {
			break
		}
		if receiveErr != nil {
			t.Fatalf("receive stream: %v", receiveErr)
		}
		chunks = append(chunks, chunk)
	}
	merged, err := agent.ConcatMessages(chunks)
	if err != nil {
		t.Fatalf("concat chunks: %v", err)
	}
	if merged.Content != "Hello" || merged.ReasoningContent != "think " {
		t.Fatalf("merged content = %q reasoning = %q", merged.Content, merged.ReasoningContent)
	}
	if len(merged.ToolCalls) != 1 || merged.ToolCalls[0].ID != "call_1" ||
		merged.ToolCalls[0].Function.Name != "lookup" || merged.ToolCalls[0].Function.Arguments != `{"q":"x"}` {
		t.Fatalf("merged tool calls = %#v", merged.ToolCalls)
	}
	if merged.ResponseMeta == nil || merged.ResponseMeta.FinishReason != "tool_calls" ||
		merged.ResponseMeta.Usage == nil || merged.ResponseMeta.Usage.TotalTokens != 14 ||
		merged.ResponseMeta.Usage.PromptTokenDetails.CachedTokens != 4 {
		t.Fatalf("merged response metadata = %#v", merged.ResponseMeta)
	}
	if merged.Extra[ExtraKeyRequestID] != "req-stream" || merged.Extra[ExtraKeyResponseID] != "resp_stream" {
		t.Fatalf("stream identity = %#v", merged.Extra)
	}
	var items []any
	matched, err := providers.DecodeContinuation(merged.Extra, modelConfig, &items)
	if err != nil || !matched || len(items) != 3 {
		t.Fatalf("stream output replay items = %#v matched=%t err=%v", items, matched, err)
	}
}

func TestGenerateReturnsProviderAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusTooManyRequests)
		_, _ = io.WriteString(writer, `{"error":{"message":"slow down","type":"rate_limit_error","code":"rate_limit"}}`)
	}))
	defer server.Close()

	model, err := NewAdapter().New(context.Background(), providers.ModelConfig{
		Provider: providers.ProviderOpenAI, Protocol: providers.ProtocolOpenAIResponses,
		Model: "m", BaseURL: server.URL, HTTPClient: server.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = model.Generate(context.Background(), []*agent.Message{agent.UserMessage("hello")})
	var apiError *providers.APIError
	if !errors.As(err, &apiError) || apiError.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("error = %T %v", err, err)
	}
	if !strings.Contains(apiError.Error(), "rate_limit") {
		t.Fatalf("provider detail lost: %v", apiError)
	}
}
