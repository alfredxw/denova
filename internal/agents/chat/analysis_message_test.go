package chat

import (
	"encoding/json"
	"strings"
	"testing"

	agent "github.com/alfredxw/denova/agent"
	"github.com/alfredxw/denova/agent/providers"
)

func TestContextAnalysisShowsReasoningAndSafeProviderContinuationMetadata(t *testing.T) {
	modelConfig := providers.ModelConfig{
		Provider: providers.ProviderOpenAI, Protocol: providers.ProtocolOpenAIResponses,
		Model: "gpt-5.6", BaseURL: "https://api.openai.com/v1",
	}
	continuation, err := providers.NewContinuation(modelConfig, []json.RawMessage{
		json.RawMessage(`{"id":"reasoning_1","type":"reasoning","encrypted_content":"must-not-leak","summary":[]}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	message := agent.AssistantMessage("I will inspect the file.", []agent.ToolCall{{
		ID: "call-read", Type: "function", Function: agent.FunctionCall{Name: "read_file", Arguments: `{"path":"chapter.md"}`},
	}})
	message.ReasoningContent = "The file contents are required."
	message.Extra = map[string]any{providers.ExtraKeyContinuation: continuation}

	part := contextAnalysisPartFromMessage("message_1", "test context", "assistant response", message)
	wantKinds := []string{"reasoning", "body", "tool_call", "provider_continuation"}
	if len(part.Parts) != len(wantKinds) {
		t.Fatalf("analysis parts = %#v, want kinds %v", part.Parts, wantKinds)
	}
	for index, kind := range wantKinds {
		if part.Parts[index].Kind != kind {
			t.Fatalf("analysis part %d kind = %q, want %q", index, part.Parts[index].Kind, kind)
		}
	}
	if part.Parts[0].Content != message.ReasoningContent {
		t.Fatalf("reasoning summary = %q, want %q", part.Parts[0].Content, message.ReasoningContent)
	}
	continuationPart := part.Parts[len(part.Parts)-1]
	visible := continuationPart.Title + continuationPart.Note + continuationPart.Content
	if strings.Contains(visible, "must-not-leak") {
		t.Fatalf("opaque provider continuation leaked into context analysis: %s", visible)
	}
	if !strings.Contains(continuationPart.Note, "provider=openai") ||
		!strings.Contains(continuationPart.Note, "protocol=openai-responses") ||
		!strings.Contains(continuationPart.Note, "payload_bytes=") {
		t.Fatalf("safe continuation metadata is incomplete: %#v", continuationPart)
	}
}
