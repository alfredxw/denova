package googlegenerativeai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"google.golang.org/genai"

	agent "github.com/alfredxw/denova/agent"
	"github.com/alfredxw/denova/agent/providers"
)

func TestRequestMapsThinkingToolsAndSignedContinuation(t *testing.T) {
	protocolOptions, err := providers.EncodeProtocolOptions(Compatibility{
		APIVersion:   "v1beta",
		ThinkingMode: ThinkingModeLevel,
		ThinkingLevels: map[string]string{
			"xhigh": "HIGH",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	maxTokens := 123
	config := providers.ModelConfig{
		Provider: "google", Protocol: providers.ProtocolGoogleGenerativeAI,
		BaseURL: "https://generativelanguage.googleapis.com", Model: "gemini-test",
		ThinkingLevel: providers.ThinkingLevelXHigh, MaxOutputTokens: &maxTokens,
		ProtocolOptions: protocolOptions,
	}
	compatibility, err := resolveCompatibility(config)
	if err != nil {
		t.Fatal(err)
	}
	model := &ChatModel{config: config, compatibility: compatibility, options: &agent.Options{}}
	contents, generationConfig, err := model.request([]*agent.Message{
		agent.SystemMessage("system"),
		agent.UserMessage("question"),
		{
			Role: agent.Assistant,
			ToolCalls: []agent.ToolCall{{
				ID: "call_1", Type: "function",
				Function: agent.FunctionCall{Name: "lookup", Arguments: `{"q":"x"}`},
			}},
		},
		agent.ToolMessage(agent.TextToolResult("result"), "call_1", agent.WithToolName("lookup")),
	}, agent.WithTools([]*agent.ToolInfo{{Name: "lookup"}}), agent.WithToolChoice(agent.ToolChoiceForced, "lookup"))
	if err != nil {
		t.Fatal(err)
	}
	if len(contents) != 3 || generationConfig.SystemInstruction == nil {
		t.Fatalf("contents = %#v system = %#v", contents, generationConfig.SystemInstruction)
	}
	if generationConfig.MaxOutputTokens != 123 || generationConfig.ThinkingConfig == nil ||
		generationConfig.ThinkingConfig.ThinkingLevel != genai.ThinkingLevelHigh || !generationConfig.ThinkingConfig.IncludeThoughts {
		t.Fatalf("generation config = %#v", generationConfig)
	}
	if generationConfig.ToolConfig == nil || generationConfig.ToolConfig.FunctionCallingConfig.Mode != genai.FunctionCallingConfigModeAny {
		t.Fatalf("tool config = %#v", generationConfig.ToolConfig)
	}
	toolResult := contents[2].Parts[0].FunctionResponse
	if toolResult == nil || toolResult.ID != "call_1" || toolResult.Name != "lookup" || toolResult.Response["output"] != "result" {
		t.Fatalf("tool result = %#v", toolResult)
	}

	response := &genai.GenerateContentResponse{
		Candidates: []*genai.Candidate{{
			Index: 0, FinishReason: genai.FinishReasonStop,
			Content: &genai.Content{Role: genai.RoleModel, Parts: []*genai.Part{
				{Text: "thought", Thought: true, ThoughtSignature: []byte("signature")},
				{Text: "answer"},
				{FunctionCall: &genai.FunctionCall{ID: "call_2", Name: "lookup", Args: map[string]any{"q": "y"}}},
			}},
		}},
		UsageMetadata: &genai.GenerateContentResponseUsageMetadata{
			PromptTokenCount: 10, CandidatesTokenCount: 4, ThoughtsTokenCount: 2, TotalTokenCount: 16,
		},
	}
	message, err := responseMessage(response, config)
	if err != nil {
		t.Fatal(err)
	}
	if message.Content != "answer" || message.ReasoningContent != "thought" || len(message.ToolCalls) != 1 {
		t.Fatalf("message = %#v", message)
	}
	if message.ResponseMeta == nil || message.ResponseMeta.Usage == nil || message.ResponseMeta.Usage.CompletionTokens != 6 {
		t.Fatalf("metadata = %#v", message.ResponseMeta)
	}
	replayed, err := assistantParts(message, config)
	if err != nil {
		t.Fatal(err)
	}
	if len(replayed) != 3 || string(replayed[0].ThoughtSignature) != "signature" {
		t.Fatalf("signed continuation = %#v", replayed)
	}
}

func TestGenerateUsesNativeGeminiEndpointAndHeaders(t *testing.T) {
	requestBody := make(chan []byte, 1)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1beta/models/gemini-test:generateContent" {
			t.Errorf("path = %q", request.URL.Path)
		}
		if request.Header.Get("X-Goog-Api-Key") != "secret" || request.Header.Get("X-Custom") != "custom" {
			t.Errorf("headers = %#v", request.Header)
		}
		body, _ := io.ReadAll(request.Body)
		requestBody <- body
		writer.Header().Set("Content-Type", "application/json")
		writer.Header().Set("X-Goog-Request-Id", "req-google")
		_, _ = io.WriteString(writer, `{
  "candidates":[{"index":0,"finishReason":"STOP","content":{"role":"model","parts":[{"text":"OK"}]}}],
  "usageMetadata":{"promptTokenCount":1,"candidatesTokenCount":1,"totalTokenCount":2},
  "modelVersion":"gemini-test-001","responseId":"response-google"
}`)
	}))
	defer server.Close()

	protocolOptions, err := providers.EncodeProtocolOptions(Compatibility{APIVersion: "v1beta"})
	if err != nil {
		t.Fatal(err)
	}
	model, err := NewAdapter().New(context.Background(), providers.ModelConfig{
		Provider: "google-test", Protocol: providers.ProtocolGoogleGenerativeAI,
		BaseURL: server.URL, Model: "gemini-test", APIKey: "secret",
		Headers: map[string]string{"X-Custom": "custom"}, HTTPClient: server.Client(),
		ProtocolOptions: protocolOptions,
	})
	if err != nil {
		t.Fatal(err)
	}
	message, err := model.Generate(context.Background(), []*agent.Message{agent.UserMessage("ping")})
	if err != nil {
		t.Fatal(err)
	}
	if message.Content != "OK" || message.ResponseMeta == nil || message.ResponseMeta.FinishReason != "STOP" {
		t.Fatalf("message = %#v", message)
	}
	if message.Extra[extraKeyResponseID] != "response-google" || message.Extra[extraKeyRequestID] != "req-google" {
		t.Fatalf("response identity = %#v", message.Extra)
	}
	var request map[string]any
	if err := json.Unmarshal(<-requestBody, &request); err != nil {
		t.Fatal(err)
	}
	contents, ok := request["contents"].([]any)
	if !ok || len(contents) != 1 {
		t.Fatalf("request = %#v", request)
	}
}
