package anthropicmessages

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	agentmodel "github.com/alfredxw/denova/agent/model"
	"github.com/alfredxw/denova/agent/model/providers"
	agentschema "github.com/alfredxw/denova/agent/schema"
	"github.com/anthropics/anthropic-sdk-go"
)

func TestDeepSeekThinkingRequestAndSignedContinuationReplay(t *testing.T) {
	protocolOptions, err := providers.EncodeProtocolOptions(Compatibility{
		ThinkingMode:           ThinkingModeBudget,
		DefaultThinkingBudget:  8192,
		DefaultMaxOutputTokens: 65536,
		EffortMap:              map[string]string{"medium": "high"},
	})
	if err != nil {
		t.Fatal(err)
	}
	config := providers.ModelConfig{
		Provider: "deepseek", Protocol: providers.ProtocolAnthropicMessages,
		BaseURL: "https://api.deepseek.com/anthropic", Model: "deepseek-v4-flash",
		ThinkingLevel: providers.ThinkingLevelXHigh, ProtocolOptions: protocolOptions,
	}
	compatibility, err := resolveCompatibility(config)
	if err != nil {
		t.Fatal(err)
	}
	model := &ChatModel{config: config, compatibility: compatibility, options: &agentmodel.Options{}}
	params, _, err := model.request(
		[]*agentschema.Message{agentschema.SystemMessage("system"), agentschema.UserMessage("question")},
		agentmodel.WithTools([]*agentschema.ToolInfo{{Name: "lookup", Desc: "Lookup"}}),
		agentmodel.WithToolChoice(agentmodel.ToolChoiceForced, "lookup"),
	)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(params)
	if err != nil {
		t.Fatal(err)
	}
	var request map[string]any
	if err := json.Unmarshal(encoded, &request); err != nil {
		t.Fatal(err)
	}
	if request["max_tokens"] != float64(65536) {
		t.Fatalf("max_tokens = %#v", request["max_tokens"])
	}
	thinking := request["thinking"].(map[string]any)
	if thinking["type"] != "enabled" || thinking["budget_tokens"] != float64(8192) {
		t.Fatalf("thinking = %#v", thinking)
	}
	outputConfig := request["output_config"].(map[string]any)
	if outputConfig["effort"] != "xhigh" {
		t.Fatalf("output_config = %#v", outputConfig)
	}
	toolChoice := request["tool_choice"].(map[string]any)
	if toolChoice["type"] != "any" {
		t.Fatalf("tool_choice = %#v", toolChoice)
	}

	var response anthropic.Message
	if err := json.Unmarshal([]byte(`{
  "id":"msg_1","type":"message","role":"assistant","model":"deepseek-v4-flash",
  "content":[
    {"type":"thinking","thinking":"checked","signature":"signed-state"},
    {"type":"text","text":"answer"},
    {"type":"tool_use","id":"tool_1","name":"lookup","input":{"q":"x"}}
  ],
  "stop_reason":"tool_use","usage":{"input_tokens":10,"output_tokens":5,"cache_read_input_tokens":3}
}`), &response); err != nil {
		t.Fatal(err)
	}
	message, err := responseMessage(&response, nil, config)
	if err != nil {
		t.Fatal(err)
	}
	if message.Content != "answer" || message.ReasoningContent != "checked" || len(message.ToolCalls) != 1 {
		t.Fatalf("message = %#v", message)
	}
	if message.ResponseMeta == nil || message.ResponseMeta.Usage == nil || message.ResponseMeta.Usage.PromptTokens != 13 {
		t.Fatalf("usage = %#v", message.ResponseMeta)
	}
	replayed, err := assistantBlocks(message, config)
	if err != nil {
		t.Fatal(err)
	}
	replayedJSON, err := json.Marshal(replayed)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(replayedJSON), `"signature":"signed-state"`) {
		t.Fatalf("signed thinking was not replayed: %s", replayedJSON)
	}
	message.ToolCalls[0].Function.Arguments = `{}`
	replayed, err = assistantBlocks(message, config)
	if err != nil {
		t.Fatal(err)
	}
	replayedJSON, err = json.Marshal(replayed)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(replayedJSON), `"signature":"signed-state"`) ||
		strings.Contains(string(replayedJSON), `"q":"x"`) {
		t.Fatalf("canonical tool input did not replace continuation input: %s", replayedJSON)
	}
}

func TestGenerateUsesMessagesStreamingEndpointWithoutSDKTimeout(t *testing.T) {
	requestBody := make(chan []byte, 1)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/messages" {
			t.Errorf("path = %q", request.URL.Path)
		}
		if request.Header.Get("X-Api-Key") != "secret" || request.Header.Get("X-Custom") != "custom" || request.Header.Get("X-Session-Id") != "conversation-123" {
			t.Errorf("headers = %#v", request.Header)
		}
		body, _ := io.ReadAll(request.Body)
		requestBody <- body
		writer.Header().Set("Content-Type", "text/event-stream")
		writer.Header().Set("Request-Id", "req-anthropic")
		frames := []struct{ event, data string }{
			{"message_start", `{"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","model":"test-model","content":[],"stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":1,"output_tokens":0}}}`},
			{"content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`},
			{"content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"OK"}}`},
			{"content_block_stop", `{"type":"content_block_stop","index":0}`},
			{"message_delta", `{"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":1}}`},
			{"message_stop", `{"type":"message_stop"}`},
		}
		for _, frame := range frames {
			_, _ = fmt.Fprintf(writer, "event: %s\ndata: %s\n\n", frame.event, frame.data)
		}
	}))
	defer server.Close()

	model, err := NewAdapter().New(context.Background(), providers.ModelConfig{
		Provider: "anthropic-test", Protocol: providers.ProtocolAnthropicMessages,
		BaseURL: server.URL, Model: "test-model", APIKey: "secret",
		Headers: map[string]string{"X-Custom": "custom"}, HTTPClient: server.Client(),
		SessionKeyMapping: &providers.SessionKeyMapping{
			Location: providers.SessionKeyLocationHeader, Name: "X-Session-Id",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	message, err := model.Generate(
		context.Background(), []*agentschema.Message{agentschema.UserMessage("ping")}, agentmodel.WithSessionKey("conversation-123"),
	)
	if err != nil {
		t.Fatal(err)
	}
	if message.Content != "OK" || message.ResponseMeta == nil || message.ResponseMeta.FinishReason != "end_turn" {
		t.Fatalf("message = %#v", message)
	}
	var request map[string]any
	if err := json.Unmarshal(<-requestBody, &request); err != nil {
		t.Fatal(err)
	}
	if request["model"] != "test-model" || request["stream"] != true {
		t.Fatalf("request = %#v", request)
	}
}

func TestPromptCacheBreakpointsToolMergeAndCacheWriteUsage(t *testing.T) {
	config := providers.ModelConfig{
		Provider: "anthropic", Protocol: providers.ProtocolAnthropicMessages,
		BaseURL: "https://api.anthropic.com", Model: "claude-sonnet-4-6",
	}
	compatibility, err := resolveCompatibility(config)
	if err != nil {
		t.Fatal(err)
	}
	model := &ChatModel{config: config, compatibility: compatibility, options: &agentmodel.Options{}}
	// Turn 1 committed history + Turn 2 active user + parallel tool results.
	// Input indices:
	//   0: System
	//   1: Leading User (Lore Index)
	//   2: Turn 1 User
	//   3: Turn 1 Assistant (committed history tail, index < 4)
	//   4: Turn 2 Active User
	//   5: Turn 2 Assistant (2 parallel tool calls)
	//   6: ToolResult 1
	//   7: ToolResult 2 (merged with ToolResult 1 into one user message)
	input := []*agentschema.Message{
		agentschema.SystemMessage("system prompt"),
		agentschema.UserMessage("lore index"),
		agentschema.UserMessage("turn 1 user"),
		agentschema.AssistantMessage("turn 1 reply", nil),
		agentschema.UserMessage("turn 2 active user"),
		agentschema.AssistantMessage("checking", []agentschema.ToolCall{
			{ID: "call-1", Type: "function", Function: agentschema.FunctionCall{Name: "read", Arguments: `{"path":"a.md"}`}},
			{ID: "call-2", Type: "function", Function: agentschema.FunctionCall{Name: "read", Arguments: `{"path":"b.md"}`}},
		}),
		agentschema.ToolMessage(agentschema.TextToolResult("content a"), "call-1"),
		agentschema.ToolMessage(agentschema.TextToolResult("content b"), "call-2"),
	}
	params, _, err := model.request(
		input,
		agentmodel.WithTools([]*agentschema.ToolInfo{
			{Name: "read", Desc: "Read file"},
			{Name: "edit", Desc: "Edit file"},
		}),
		agentmodel.WithCachePrefixMessages(4),
	)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(params)
	if err != nil {
		t.Fatal(err)
	}
	var payload struct {
		Tools []struct {
			Name         string          `json:"name"`
			CacheControl json.RawMessage `json:"cache_control"`
		} `json:"tools"`
		System []struct {
			Text         string          `json:"text"`
			CacheControl json.RawMessage `json:"cache_control"`
		} `json:"system"`
		Messages []struct {
			Role    string `json:"role"`
			Content []struct {
				Type         string          `json:"type"`
				ToolUseID    string          `json:"tool_use_id"`
				CacheControl json.RawMessage `json:"cache_control"`
			} `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(encoded, &payload); err != nil {
		t.Fatal(err)
	}
	// Parallel ToolRole messages must merge into a single user message (6 messages total, not 7).
	if len(payload.Messages) != 6 {
		t.Fatalf("messages count = %d, want 6 (merged tool results): %s", len(payload.Messages), encoded)
	}
	lastMsg := payload.Messages[5]
	if lastMsg.Role != "user" || len(lastMsg.Content) != 2 || lastMsg.Content[0].ToolUseID != "call-1" || lastMsg.Content[1].ToolUseID != "call-2" {
		t.Fatalf("parallel tool results were not merged: %#v", lastMsg)
	}
	// Exactly 4 breakpoints: last tool, last system, committed history tail (msg[2]), last tool result (msg[5].content[1]).
	if len(payload.Tools[0].CacheControl) != 0 || !strings.Contains(string(payload.Tools[1].CacheControl), `"ephemeral"`) {
		t.Fatalf("unexpected tool cache_control: %#v", payload.Tools)
	}
	if !strings.Contains(string(payload.System[0].CacheControl), `"ephemeral"`) {
		t.Fatalf("missing system cache_control: %#v", payload.System)
	}
	if !strings.Contains(string(payload.Messages[2].Content[0].CacheControl), `"ephemeral"`) {
		t.Fatalf("missing committed history tail breakpoint at messages[2]: %s", encoded)
	}
	if len(lastMsg.Content[0].CacheControl) != 0 || !strings.Contains(string(lastMsg.Content[1].CacheControl), `"ephemeral"`) {
		t.Fatalf("missing final tool_result breakpoint: %#v", lastMsg.Content)
	}
	for _, idx := range []int{0, 1, 3, 4} {
		for _, block := range payload.Messages[idx].Content {
			if len(block.CacheControl) != 0 {
				t.Fatalf("unexpected breakpoint at messages[%d]: %s", idx, encoded)
			}
		}
	}

	// Verify compatibility toggles: disabled mode emits zero cache_control markers.
	disabledOpts, err := providers.EncodeProtocolOptions(Compatibility{PromptCacheMode: PromptCacheModeDisabled})
	if err != nil {
		t.Fatal(err)
	}
	disabledCompat, err := resolveCompatibility(providers.ModelConfig{ProtocolOptions: disabledOpts})
	if err != nil {
		t.Fatal(err)
	}
	disabledModel := &ChatModel{config: config, compatibility: disabledCompat, options: &agentmodel.Options{}}
	disabledParams, _, err := disabledModel.request(input, agentmodel.WithTools([]*agentschema.ToolInfo{{Name: "read"}}), agentmodel.WithCachePrefixMessages(4))
	if err != nil {
		t.Fatal(err)
	}
	disabledJSON, err := json.Marshal(disabledParams)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(disabledJSON), "cache_control") {
		t.Fatalf("disabled prompt cache mode still emitted cache_control: %s", disabledJSON)
	}

	// Verify responseUsage preserves CacheCreationInputTokens as CacheWriteTokens.
	var response anthropic.Message
	if err := json.Unmarshal([]byte(`{
  "id":"msg_cache","type":"message","role":"assistant","model":"claude-sonnet-4-6",
  "content":[{"type":"text","text":"ok"}],
  "stop_reason":"end_turn",
  "usage":{"input_tokens":100,"cache_creation_input_tokens":2000,"cache_read_input_tokens":8000,"output_tokens":50}
}`), &response); err != nil {
		t.Fatal(err)
	}
	parsed, err := responseMessage(&response, nil, config)
	if err != nil {
		t.Fatal(err)
	}
	usage := parsed.ResponseMeta.Usage
	if usage.PromptTokens != 10100 || usage.PromptTokenDetails.CachedTokens != 8000 || usage.PromptTokenDetails.CacheWriteTokens != 2000 || usage.CompletionTokens != 50 || usage.TotalTokens != 10150 {
		t.Fatalf("unexpected parsed usage = %#v", usage)
	}
}
