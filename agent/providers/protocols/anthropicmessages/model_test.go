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

	"github.com/anthropics/anthropic-sdk-go"

	agent "github.com/alfredxw/denova/agent"
	"github.com/alfredxw/denova/agent/providers"
)

func TestDeepSeekThinkingRequestAndSignedContinuationReplay(t *testing.T) {
	protocolOptions, err := providers.EncodeProtocolOptions(Compatibility{
		ThinkingMode:           ThinkingModeBudget,
		DefaultThinkingBudget:  8192,
		DefaultMaxOutputTokens: 65536,
		EffortMap:              map[string]string{"minimal": "low", "medium": "high"},
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
	model := &ChatModel{config: config, compatibility: compatibility, options: &agent.Options{}}
	params, _, err := model.request(
		[]*agent.Message{agent.SystemMessage("system"), agent.UserMessage("question")},
		agent.WithTools([]*agent.ToolInfo{{Name: "lookup", Desc: "Lookup"}}),
		agent.WithToolChoice(agent.ToolChoiceForced, "lookup"),
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
}

func TestGenerateUsesMessagesStreamingEndpointWithoutSDKTimeout(t *testing.T) {
	requestBody := make(chan []byte, 1)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/messages" {
			t.Errorf("path = %q", request.URL.Path)
		}
		if request.Header.Get("X-Api-Key") != "secret" || request.Header.Get("X-Custom") != "custom" {
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
	})
	if err != nil {
		t.Fatal(err)
	}
	message, err := model.Generate(context.Background(), []*agent.Message{agent.UserMessage("ping")})
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
