package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
)

func TestProcessStreamingEventReclassifiesInteractiveToolPreambleAsThinking(t *testing.T) {
	reader, writer := schema.Pipe[*schema.Message](3)
	writer.Send(&schema.Message{Role: schema.Assistant, Content: "我先检查资料，再开始写正文。"}, nil)
	writer.Send(&schema.Message{Role: schema.Assistant, ToolCalls: []schema.ToolCall{{
		ID: "call-lore",
		Function: schema.FunctionCall{
			Name:      "list_lore_items",
			Arguments: `{}`,
		},
	}}}, nil)
	writer.Close()

	var content strings.Builder
	var thinking strings.Builder
	var events []Event
	_, err := processStreamingEvent(
		context.Background(),
		&adk.MessageVariant{IsStreaming: true, MessageStream: reader, Role: schema.Assistant},
		&content,
		&thinking,
		0,
		0,
		agentEventMetadata{AgentKind: AgentKindInteractiveStory},
		nil,
		func(event Event) { events = append(events, event) },
	)
	if err != nil {
		t.Fatal(err)
	}
	if got := content.String(); got != "" {
		t.Fatalf("tool preamble leaked into interactive narrative: %q", got)
	}
	if got := thinking.String(); got != "我先检查资料，再开始写正文。" {
		t.Fatalf("thinking = %q", got)
	}
	if !hasEvent(events, "interactive_content_reclassified") {
		t.Fatalf("events = %#v, want interactive_content_reclassified", events)
	}
}

func hasEvent(events []Event, eventType string) bool {
	for _, event := range events {
		if event.Type == eventType {
			return true
		}
	}
	return false
}
