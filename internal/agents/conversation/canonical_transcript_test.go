package conversation

import (
	"context"
	"testing"

	agent "github.com/alfredxw/denova/agent"

	"denova/config"
	"denova/internal/agents/session"
)

func TestSessionConversationCanonicalTranscriptKeepsCompleteRawToolHistory(t *testing.T) {
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.GetOrCreate("canonical-writing")
	if err != nil {
		t.Fatal(err)
	}
	messages := []*agent.Message{
		agent.UserMessage("inspect the outline"),
		agent.AssistantMessage("", []agent.ToolCall{{
			ID: "call-read", Function: agent.FunctionCall{Name: "read", Arguments: `{"path":"outline.md"}`},
		}}),
		agent.ToolMessage(agent.ToolResult{Status: agent.ToolResultSuccess, ModelContent: "RAW_OUTLINE_RESULT"}, "call-read", agent.WithToolName("read")),
		agent.AssistantMessage("outline inspected", nil),
	}
	for _, message := range messages {
		if err := sess.Append(message); err != nil {
			t.Fatal(err)
		}
	}
	conversation := NewSessionConversationForAgent(sess, &config.Config{}, config.AgentKindIDE)
	request, err := conversation.CanonicalTranscript(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if request.Source.Kind != sessionTranscriptSourceKind || request.Source.ConfigHash == "" ||
		request.SourceRevision != sess.ContextCursor().Revision || len(request.Messages) != len(messages) {
		t.Fatalf("canonical transcript = %#v", request)
	}
	if request.Messages[2].Content != "RAW_OUTLINE_RESULT" {
		t.Fatalf("canonical raw history lost rich tool result: %#v", request.Messages)
	}
	if _, err := agent.TranscriptHash(request.Messages); err != nil {
		t.Fatalf("canonical product transcript is not importable: %v", err)
	}
}
