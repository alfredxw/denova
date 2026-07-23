package openai

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
	"testing"

	"github.com/alfredxw/denova/adk"
)

func TestGenerateMapsCompleteRequestAndResponse(t *testing.T) {
	requestBody := make(chan []byte, 1)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/chat/completions" {
			t.Errorf("request path = %q", request.URL.Path)
		}
		if got := request.Header.Get("Authorization"); got != "Bearer secret" {
			t.Errorf("authorization = %q", got)
		}
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Errorf("read request: %v", err)
		}
		requestBody <- body
		writer.Header().Set("Content-Type", "application/json")
		writer.Header().Set("X-Request-ID", "req-generate-1")
		_, _ = io.WriteString(writer, `{
  "id":"chatcmpl-zero",
  "object":"chat.completion",
  "created":1700000000,
  "model":"provider-model",
  "service_tier":"priority",
  "system_fingerprint":"fp_test",
  "choices":[
    {"index":1,"finish_reason":"stop","message":{"role":"assistant","content":"wrong"}},
    {"index":0,"finish_reason":"provider_custom_stop","message":{"content":"answer","reasoning_content":"private thought","tool_calls":[{"id":"call_new","type":"function","function":{"name":"lookup","arguments":"{\"q\":\"new\"}"}}]}}
  ],
  "usage":{"prompt_tokens":11,"completion_tokens":7,"total_tokens":18,"prompt_tokens_details":{"cached_tokens":5},"completion_tokens_details":{"reasoning_tokens":3}}
}`)
	}))
	defer server.Close()

	temperature := float32(0.25)
	configuredMaxTokens := 321
	model, err := New(context.Background(), &Config{
		APIKey:          "secret",
		Model:           "test-model",
		BaseURL:         server.URL + "/v1",
		HTTPClient:      server.Client(),
		Temperature:     &temperature,
		MaxTokens:       &configuredMaxTokens,
		ReasoningEffort: ReasoningEffortLevelHigh,
		ResponseFormat: &ChatCompletionResponseFormat{
			Type: ChatCompletionResponseFormatTypeJSONObject,
		},
		ExtraFields: map[string]any{
			"enable_thinking": true,
			"reasoning_split": true,
		},
	})
	if err != nil {
		t.Fatalf("new model: %v", err)
	}

	bound, err := model.WithTools([]*adk.ToolInfo{
		{
			Name: "lookup",
			Desc: "Lookup",
			ParamsOneOf: adk.NewParamsOneOfByParams(map[string]*adk.ParameterInfo{
				"z": {Type: adk.Integer, Required: true},
				"a": {Type: adk.String, Required: true},
			}),
		},
		{Name: "not_allowed", ParamsOneOf: adk.NewParamsOneOfByParams(nil)},
	})
	if err != nil {
		t.Fatalf("bind tools: %v", err)
	}
	message, err := bound.Generate(context.Background(), []*adk.Message{
		{Role: adk.System, Content: "sys", Name: "director"},
		{Role: adk.User, Content: "hello", Name: "writer"},
		{
			Role: adk.Assistant,
			ToolCalls: []adk.ToolCall{{
				ID:   "call_prev",
				Type: "function",
				Function: adk.FunctionCall{
					Name:      "lookup",
					Arguments: `{"q":"old"}`,
				},
			}},
		},
		{Role: adk.Tool, ToolCallID: "call_prev", ToolName: "lookup", Content: `{"ok":true}`},
	}, adk.WithMaxTokens(99), adk.WithToolChoice(adk.ToolChoiceAllowed, "lookup"))
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	var gotRequest map[string]any
	if err := json.Unmarshal(<-requestBody, &gotRequest); err != nil {
		t.Fatalf("decode request: %v", err)
	}
	wantRequest := map[string]any{
		"messages": []any{
			map[string]any{"role": "system", "content": "sys", "name": "director"},
			map[string]any{"role": "user", "content": "hello", "name": "writer"},
			map[string]any{
				"role": "assistant",
				"tool_calls": []any{map[string]any{
					"id": "call_prev", "type": "function",
					"function": map[string]any{"name": "lookup", "arguments": `{"q":"old"}`},
				}},
			},
			map[string]any{"role": "tool", "content": `{"ok":true}`, "tool_call_id": "call_prev"},
		},
		"model":            "test-model",
		"temperature":      0.25,
		"max_tokens":       float64(99),
		"reasoning_effort": "high",
		"response_format":  map[string]any{"type": "json_object"},
		"enable_thinking":  true,
		"reasoning_split":  true,
		"tool_choice":      "auto",
		"tools": []any{map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        "lookup",
				"description": "Lookup",
				"parameters": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"a": map[string]any{"type": "string"},
						"z": map[string]any{"type": "integer"},
					},
					"required": []any{"a", "z"},
				},
			},
		}},
	}
	if !reflect.DeepEqual(gotRequest, wantRequest) {
		got, _ := json.MarshalIndent(gotRequest, "", "  ")
		want, _ := json.MarshalIndent(wantRequest, "", "  ")
		t.Fatalf("request mismatch\ngot:  %s\nwant: %s", got, want)
	}

	if message.Role != adk.Assistant || message.Content != "answer" || message.ReasoningContent != "private thought" {
		t.Fatalf("response message = %#v", message)
	}
	if len(message.ToolCalls) != 1 || message.ToolCalls[0].ID != "call_new" || message.ToolCalls[0].Function.Arguments != `{"q":"new"}` {
		t.Fatalf("response tool calls = %#v", message.ToolCalls)
	}
	if message.ResponseMeta == nil || message.ResponseMeta.FinishReason != "provider_custom_stop" {
		t.Fatalf("response finish metadata = %#v", message.ResponseMeta)
	}
	wantUsage := &adk.TokenUsage{
		PromptTokens:       11,
		CompletionTokens:   7,
		TotalTokens:        18,
		PromptTokenDetails: adk.PromptTokenDetails{CachedTokens: 5},
		CompletionTokensDetails: adk.CompletionTokensDetails{
			ReasoningTokens: 3,
		},
	}
	if !reflect.DeepEqual(message.ResponseMeta.Usage, wantUsage) {
		t.Fatalf("usage = %#v, want %#v", message.ResponseMeta.Usage, wantUsage)
	}
	wantExtra := map[string]any{
		ExtraKeyProvider:          "openai",
		ExtraKeyRequestID:         "req-generate-1",
		ExtraKeyResponseID:        "chatcmpl-zero",
		ExtraKeyModel:             "provider-model",
		ExtraKeyCreated:           int64(1700000000),
		ExtraKeyServiceTier:       "priority",
		ExtraKeySystemFingerprint: "fp_test",
	}
	if !reflect.DeepEqual(message.Extra, wantExtra) {
		t.Fatalf("extra = %#v, want %#v", message.Extra, wantExtra)
	}
	for _, value := range message.Extra {
		valueType := reflect.TypeOf(value)
		if valueType != nil && valueType.PkgPath() == "github.com/openai/openai-go/v3" {
			t.Fatalf("SDK value leaked into Message.Extra: %T", value)
		}
	}
}

func TestWithToolsReturnsImmutableCopy(t *testing.T) {
	bodies := make(chan []byte, 2)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, _ := io.ReadAll(request.Body)
		bodies <- body
		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, `{"id":"id","choices":[{"index":0,"finish_reason":"stop","message":{"role":"assistant","content":"ok"}}],"created":1,"model":"m","object":"chat.completion","usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`)
	}))
	defer server.Close()

	model, err := New(context.Background(), &Config{Model: "m", BaseURL: server.URL, HTTPClient: server.Client()})
	if err != nil {
		t.Fatalf("new model: %v", err)
	}
	tool := &adk.ToolInfo{Name: "stable", Desc: "before", ParamsOneOf: adk.NewParamsOneOfByParams(nil)}
	bound, err := model.WithTools([]*adk.ToolInfo{tool})
	if err != nil {
		t.Fatalf("bind tool: %v", err)
	}
	tool.Name = "mutated"
	tool.Desc = "after"
	if _, err := bound.Generate(context.Background(), []*adk.Message{adk.UserMessage("one")}); err != nil {
		t.Fatalf("bound generate: %v", err)
	}
	if _, err := model.Generate(context.Background(), []*adk.Message{adk.UserMessage("two")}); err != nil {
		t.Fatalf("base generate: %v", err)
	}

	var boundRequest, baseRequest map[string]any
	if err := json.Unmarshal(<-bodies, &boundRequest); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(<-bodies, &baseRequest); err != nil {
		t.Fatal(err)
	}
	tools, ok := boundRequest["tools"].([]any)
	if !ok || len(tools) != 1 {
		t.Fatalf("bound tools = %#v", boundRequest["tools"])
	}
	function := tools[0].(map[string]any)["function"].(map[string]any)
	if function["name"] != "stable" || function["description"] != "before" {
		t.Fatalf("bound tool mutated = %#v", function)
	}
	if _, exists := baseRequest["tools"]; exists {
		t.Fatalf("base model was mutated: %#v", baseRequest)
	}
}

func TestRequestToolChoiceMapping(t *testing.T) {
	for _, test := range []struct {
		name      string
		choice    adk.ToolChoice
		toolCount int
		want      string
		wantError bool
	}{
		{name: "forbidden", choice: adk.ToolChoiceForbidden, toolCount: 1, want: "none"},
		{name: "allowed", choice: adk.ToolChoiceAllowed, toolCount: 1, want: "auto"},
		{name: "forced", choice: adk.ToolChoiceForced, toolCount: 1, want: "required"},
		{name: "forced without tools", choice: adk.ToolChoiceForced, wantError: true},
		{name: "unknown", choice: adk.ToolChoice("future"), toolCount: 1, wantError: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			choice := test.choice
			got, err := requestToolChoice(&choice, test.toolCount)
			if (err != nil) != test.wantError {
				t.Fatalf("error = %v, wantError=%t", err, test.wantError)
			}
			if err == nil && got.OfAuto.Value != test.want {
				t.Fatalf("tool choice = %q, want %q", got.OfAuto.Value, test.want)
			}
		})
	}
}

func TestStreamMapsToolCallDeltasReasoningAndUsageTail(t *testing.T) {
	requestBody := make(chan []byte, 1)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, _ := io.ReadAll(request.Body)
		requestBody <- body
		writer.Header().Set("Content-Type", "text/event-stream")
		writer.Header().Set("X-Request-ID", "req-stream-1")
		flusher := writer.(http.Flusher)
		frames := []string{
			`{"id":"stream-id","object":"chat.completion.chunk","created":2,"model":"stream-model","choices":[{"index":1,"delta":{"content":"BAD"},"finish_reason":""}]}`,
			`{"id":"stream-id","object":"chat.completion.chunk","created":2,"model":"stream-model","choices":[{"index":0,"delta":{"content":"Hel","reasoning_content":"think "},"finish_reason":""}]}`,
			`{"id":"stream-id","object":"chat.completion.chunk","created":2,"model":"stream-model","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"lookup","arguments":"{\"q\":\""}}]},"finish_reason":""}]}`,
			`{"id":"stream-id","object":"chat.completion.chunk","created":2,"model":"stream-model","choices":[{"index":0,"delta":{"content":"lo","reasoning_content":"hard","tool_calls":[{"index":0,"function":{"arguments":"x\"}"}}]},"finish_reason":"provider_pause"}]}`,
			`{"id":"stream-id","object":"chat.completion.chunk","created":2,"model":"stream-model","choices":[],"usage":{"prompt_tokens":8,"completion_tokens":6,"total_tokens":14,"prompt_tokens_details":{"cached_tokens":4},"completion_tokens_details":{"reasoning_tokens":2}}}`,
		}
		for _, frame := range frames {
			_, _ = fmt.Fprintf(writer, "data: %s\n\n", frame)
			flusher.Flush()
		}
		_, _ = io.WriteString(writer, "data: [DONE]\n\n")
		flusher.Flush()
	}))
	defer server.Close()

	model, err := New(context.Background(), &Config{Model: "m", BaseURL: server.URL, HTTPClient: server.Client()})
	if err != nil {
		t.Fatalf("new model: %v", err)
	}
	stream, err := model.Stream(context.Background(), []*adk.Message{adk.UserMessage("hello")})
	if err != nil {
		t.Fatalf("stream: %v", err)
	}
	defer stream.Close()
	var chunks []*adk.Message
	for {
		chunk, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("receive stream: %v", err)
		}
		chunks = append(chunks, chunk)
	}
	if len(chunks) != 4 {
		t.Fatalf("chunks = %d, want 4: %#v", len(chunks), chunks)
	}
	for index, chunk := range chunks {
		if chunk.Role != adk.Assistant {
			t.Fatalf("chunk %d role = %q", index, chunk.Role)
		}
	}
	if chunks[0].Extra[ExtraKeyRequestID] != "req-stream-1" || chunks[0].Extra[ExtraKeyResponseID] != "stream-id" {
		t.Fatalf("first chunk metadata = %#v", chunks[0].Extra)
	}
	for index := 1; index < len(chunks); index++ {
		if chunks[index].Extra != nil {
			t.Fatalf("metadata repeated on chunk %d: %#v", index, chunks[index].Extra)
		}
	}
	if chunks[3].ResponseMeta == nil || chunks[3].ResponseMeta.Usage == nil || chunks[3].ResponseMeta.Usage.TotalTokens != 14 {
		t.Fatalf("usage-only tail = %#v", chunks[3])
	}
	merged, err := adk.ConcatMessages(chunks)
	if err != nil {
		t.Fatalf("concat chunks: %v", err)
	}
	if merged.Content != "Hello" || merged.ReasoningContent != "think hard" {
		t.Fatalf("merged content/reasoning = %q/%q", merged.Content, merged.ReasoningContent)
	}
	if len(merged.ToolCalls) != 1 || merged.ToolCalls[0].Index == nil || *merged.ToolCalls[0].Index != 0 || merged.ToolCalls[0].Function.Name != "lookup" || merged.ToolCalls[0].Function.Arguments != `{"q":"x"}` {
		t.Fatalf("merged tool calls = %#v", merged.ToolCalls)
	}
	if merged.ResponseMeta == nil || merged.ResponseMeta.FinishReason != "provider_pause" || merged.ResponseMeta.Usage == nil || merged.ResponseMeta.Usage.PromptTokenDetails.CachedTokens != 4 || merged.ResponseMeta.Usage.CompletionTokensDetails.ReasoningTokens != 2 {
		t.Fatalf("merged metadata = %#v", merged.ResponseMeta)
	}

	var request map[string]any
	if err := json.Unmarshal(<-requestBody, &request); err != nil {
		t.Fatalf("decode streaming request: %v", err)
	}
	if request["stream"] != true {
		t.Fatalf("stream request missing stream=true: %#v", request)
	}
	streamOptions, ok := request["stream_options"].(map[string]any)
	if !ok || streamOptions["include_usage"] != true {
		t.Fatalf("stream options = %#v", request["stream_options"])
	}
}

func TestGenerateReturnsAdapterAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(writer, `{"error":{"message":"bad input","type":"invalid_request_error","param":"messages","code":"bad_request"}}`)
	}))
	defer server.Close()

	model, err := New(context.Background(), &Config{Model: "m", BaseURL: server.URL, HTTPClient: server.Client()})
	if err != nil {
		t.Fatalf("new model: %v", err)
	}
	_, err = model.Generate(context.Background(), []*adk.Message{adk.UserMessage("hello")})
	if err == nil {
		t.Fatal("expected provider error")
	}
	var apiError *APIError
	if !errors.As(err, &apiError) {
		t.Fatalf("error type = %T, want *openai.APIError: %v", err, err)
	}
	if apiError.StatusCode != http.StatusBadRequest {
		t.Fatalf("typed error = %#v", apiError)
	}
	if apiError.Unwrap() == nil {
		t.Fatal("adapter error did not retain the provider error as its cause")
	}
	if !strings.Contains(apiError.Error(), "bad_request") {
		t.Fatalf("adapter error lost provider details: %v", apiError)
	}
}

func TestGenerateReturnsContextCancellation(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		<-request.Context().Done()
		return nil, request.Context().Err()
	})}
	model, err := New(context.Background(), &Config{Model: "m", BaseURL: "https://example.invalid/v1", HTTPClient: client})
	if err != nil {
		t.Fatalf("new model: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = model.Generate(ctx, []*adk.Message{adk.UserMessage("hello")})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %T %v, want context.Canceled", err, err)
	}
	if err != context.Canceled {
		t.Fatalf("cancellation was wrapped or replaced: %T %v", err, err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}
