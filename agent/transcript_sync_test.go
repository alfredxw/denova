package agent

import (
	"context"
	"encoding/json"
	"testing"

	agentsession "github.com/alfredxw/denova/agent/session"
)

func TestSyncTranscriptPreservesAgentOwnedToolHistoryOnForwardProductRevision(t *testing.T) {
	ctx := context.Background()
	owner, err := New(ctx, Definition{Name: "transcript-sync-test", Model: &lifecycleModel{}}, WithSessionStore(agentsession.Memory()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close(ctx) })
	session, err := owner.Session(ctx, NamedSession("preserve-tool-history"))
	if err != nil {
		t.Fatal(err)
	}
	source := CapabilityIdentity{Kind: "test.product.transcript", Version: 1}
	firstCanonical := []*Message{UserMessage("First question"), AssistantMessage("First answer", nil)}
	if _, err := session.SyncTranscript(ctx, TranscriptSyncRequest{Source: source, SourceRevision: 1, Messages: firstCanonical}); err != nil {
		t.Fatal(err)
	}

	toolCall := ToolCall{ID: "call-read", Type: "function", Function: FunctionCall{Name: "read_file", Arguments: `{}`}}
	raw := []*Message{
		firstCanonical[0], firstCanonical[1],
		UserMessage("Second question"),
		AssistantMessage("", []ToolCall{toolCall}),
		ToolMessage(TextToolResult("tool evidence"), toolCall.ID),
		AssistantMessage("Second answer", nil),
	}
	encoded, err := json.Marshal(engineTranscript{Version: engineTranscriptVersion, Messages: cloneMessages(raw)})
	if err != nil {
		t.Fatal(err)
	}
	session.mu.Lock()
	session.engineState = encoded
	if err := session.persistTranscriptLocked(ctx); err != nil {
		session.mu.Unlock()
		t.Fatal(err)
	}
	session.mu.Unlock()

	secondCanonical := []*Message{
		firstCanonical[0], firstCanonical[1],
		UserMessage("Second question"), AssistantMessage("Second answer", nil),
	}
	result, err := session.SyncTranscript(ctx, TranscriptSyncRequest{Source: source, SourceRevision: 2, Messages: secondCanonical})
	if err != nil {
		t.Fatal(err)
	}
	if result.Rebuilt {
		t.Fatal("forward product settlement rebuilt and discarded the Agent-owned tool batch")
	}
	transcript, err := decodeEngineTranscript(session.engineState)
	if err != nil {
		t.Fatal(err)
	}
	if len(transcript.Messages) != len(raw) || len(transcript.Messages[3].ToolCalls) != 1 || transcript.Messages[4].Content != "tool evidence" {
		t.Fatalf("Agent-owned tool history changed after product settlement: %#v", transcript.Messages)
	}
}

func TestTranscriptSourceEquivalentAllowsAgentOwnedCompleteToolBatches(t *testing.T) {
	toolCall := ToolCall{
		ID:   "call-read-1",
		Type: "function",
		Function: FunctionCall{
			Name:      "read_file",
			Arguments: `{"path":"chapter.md"}`,
		},
	}
	assistantCall := AssistantMessage("I will inspect the chapter.", []ToolCall{toolCall})
	assistantCall.ReasoningContent = "The chapter must be read before answering."
	assistantCall.Extra = map[string]any{"provider-continuation": map[string]any{"opaque": "state"}}
	final := AssistantMessage("The chapter is consistent.", nil)
	final.ReasoningContent = "I checked the relevant passage."
	final.AgentMeta = &AgentMessageMeta{ModelResponseOrdinal: 2}
	final.ResponseMeta = &ResponseMeta{FinishReason: "stop"}

	current := []*Message{
		UserMessage("Check the chapter."),
		assistantCall,
		ToolMessage(TextToolResult("chapter contents"), toolCall.ID),
		final,
	}
	canonical := []*Message{
		UserMessage("Check the chapter."),
		AssistantMessage("The chapter is consistent.", nil),
	}

	equivalent, err := transcriptSourceEquivalent(current, canonical)
	if err != nil {
		t.Fatal(err)
	}
	if !equivalent {
		t.Fatal("complete Agent-owned tool batch made the same product transcript non-equivalent")
	}
}

func TestTranscriptSourceEquivalentComparesProductOwnedToolBatchesExactly(t *testing.T) {
	toolCall := ToolCall{
		ID:   "call-read-1",
		Type: "function",
		Function: FunctionCall{
			Name:      "read_file",
			Arguments: `{"path":"chapter.md"}`,
		},
	}
	current := []*Message{
		UserMessage("Check the chapter."),
		AssistantMessage("", []ToolCall{toolCall}),
		ToolMessage(TextToolResult("original contents"), toolCall.ID),
		AssistantMessage("Done.", nil),
	}
	canonical := []*Message{
		UserMessage("Check the chapter."),
		AssistantMessage("", []ToolCall{toolCall}),
		ToolMessage(TextToolResult("changed contents"), toolCall.ID),
		AssistantMessage("Done.", nil),
	}

	equivalent, err := transcriptSourceEquivalent(current, canonical)
	if err != nil {
		t.Fatal(err)
	}
	if equivalent {
		t.Fatal("changed product-owned tool result was treated as equivalent")
	}
}

func TestTranscriptSourceEquivalentRejectsChangedVisibleMessage(t *testing.T) {
	current := []*Message{UserMessage("Question"), AssistantMessage("Original", nil)}
	canonical := []*Message{UserMessage("Question"), AssistantMessage("Edited", nil)}

	equivalent, err := transcriptSourceEquivalent(current, canonical)
	if err != nil {
		t.Fatal(err)
	}
	if equivalent {
		t.Fatal("changed visible assistant response was treated as equivalent")
	}
}
