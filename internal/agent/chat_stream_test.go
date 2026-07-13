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
		true,
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

func TestProcessStreamingEventStreamsInteractivePreparationAsThinkingBeforeTurnResult(t *testing.T) {
	reader, writer := schema.Pipe[*schema.Message](1)
	writer.Send(&schema.Message{Role: schema.Assistant, Content: "我先检查当前剧情状态。"}, nil)
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
		false,
		nil,
		func(event Event) { events = append(events, event) },
	)
	if err != nil {
		t.Fatal(err)
	}
	if got := content.String(); got != "" {
		t.Fatalf("pre-TurnResult content must not enter narrative: %q", got)
	}
	if got := thinking.String(); got != "我先检查当前剧情状态。" {
		t.Fatalf("thinking = %q", got)
	}
	if len(events) != 1 || events[0].Type != "thinking" {
		t.Fatalf("first preparation event = %#v, want thinking", events)
	}
}

func TestProcessStreamingEventStreamsInteractiveNarrativeAfterTurnResult(t *testing.T) {
	reader, writer := schema.Pipe[*schema.Message](1)
	writer.Send(&schema.Message{Role: schema.Assistant, Content: "夜雨落在青石街上。"}, nil)
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
		true,
		nil,
		func(event Event) { events = append(events, event) },
	)
	if err != nil {
		t.Fatal(err)
	}
	if got := content.String(); got != "夜雨落在青石街上。" {
		t.Fatalf("narrative = %q", got)
	}
	if thinking.Len() != 0 {
		t.Fatalf("final narrative leaked into thinking: %q", thinking.String())
	}
	if len(events) != 1 || events[0].Type != "chunk" {
		t.Fatalf("final narrative event = %#v, want chunk", events)
	}
}

func TestProcessStreamingEventKeepsInteractiveCompletionRetryInternal(t *testing.T) {
	reader, writer := schema.Pipe[*schema.Message](2)
	writer.Send(&schema.Message{Role: schema.Assistant, Content: "门后传来锁链拖地的声音。"}, nil)
	writer.Send(nil, interactiveRetryErrorForTest{reason: interactiveCompletionRetryReason{Code: interactiveCompletionRetryCode}})

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
		false,
		nil,
		func(event Event) { events = append(events, event) },
	)
	if _, retrying := interactiveCompletionRetryFromError(err); !retrying {
		t.Fatalf("expected internal protocol retry, got %v", err)
	}
	if content.Len() != 0 || thinking.String() != "门后传来锁链拖地的声音。" {
		t.Fatalf("rejected candidate classification mismatch: content=%q thinking=%q", content.String(), thinking.String())
	}
	if hasEvent(events, "error") {
		t.Fatalf("internal retry leaked as a user-visible error: %#v", events)
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
