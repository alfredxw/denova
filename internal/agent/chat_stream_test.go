package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/alfredxw/denova/adk"
)

func TestProcessStreamingEventPreservesProviderThinkingVerbatim(t *testing.T) {
	reader, writer := adk.Pipe[*adk.Message](1)
	rawThinking := "开局：" + strings.Repeat("规划目标、约束与状态。", 300) + "供应商思考尾部必须完整展示"
	writer.Send(&adk.Message{Role: adk.Assistant, ReasoningContent: rawThinking}, nil)
	writer.Close()

	var content strings.Builder
	var thinking strings.Builder
	var events []Event
	_, err := processStreamingEvent(
		context.Background(),
		&adk.MessageVariant{IsStreaming: true, MessageStream: reader, Role: adk.Assistant},
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
	if got := thinking.String(); got != rawThinking {
		t.Fatalf("provider thinking was changed: got_bytes=%d want_bytes=%d\ngot_tail=%q", len(got), len(rawThinking), got[max(0, len(got)-80):])
	}
	if len(events) != 1 || events[0].Type != "thinking" || eventDataString(events[0].Data, "content") != rawThinking {
		t.Fatalf("visible thinking must match the provider output verbatim: %#v", events)
	}
}

func TestProcessNonStreamingEventPreservesToolArgumentsVerbatim(t *testing.T) {
	rawArgs := `{"path":"chapters/ch01.md","content":"` + strings.Repeat("正文", 300) + `工具输入尾部必须完整展示"}`
	message := &adk.Message{Role: adk.Assistant, ToolCalls: []adk.ToolCall{{
		ID: "call-write",
		Function: adk.FunctionCall{
			Name:      "write_file",
			Arguments: rawArgs,
		},
	}}}
	var content strings.Builder
	var thinking strings.Builder
	var events []Event

	processNonStreamingEvent(
		&adk.MessageVariant{Message: message, Role: adk.Assistant},
		&content,
		&thinking,
		0,
		agentEventMetadata{},
		nil,
		func(event Event) { events = append(events, event) },
	)

	if len(events) != 1 || events[0].Type != "tool_call" {
		t.Fatalf("tool call event = %#v", events)
	}
	if got := eventDataString(events[0].Data, "args"); got != rawArgs {
		t.Fatalf("tool arguments were changed: got_bytes=%d want_bytes=%d\ngot=%q", len(got), len(rawArgs), got)
	}
}

func TestProcessStreamingEventReclassifiesInteractiveToolPreambleAsThinking(t *testing.T) {
	reader, writer := adk.Pipe[*adk.Message](3)
	writer.Send(&adk.Message{Role: adk.Assistant, Content: "我先检查资料，再开始写正文。"}, nil)
	writer.Send(&adk.Message{Role: adk.Assistant, ToolCalls: []adk.ToolCall{{
		ID: "call-lore",
		Function: adk.FunctionCall{
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
		&adk.MessageVariant{IsStreaming: true, MessageStream: reader, Role: adk.Assistant},
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

func TestProcessStreamingEventStreamsFirstInteractiveNarrativeCandidate(t *testing.T) {
	reader, writer := adk.Pipe[*adk.Message](1)
	writer.Send(&adk.Message{Role: adk.Assistant, Content: "夜雨落在青石街上。"}, nil)
	writer.Close()

	var content strings.Builder
	var thinking strings.Builder
	var events []Event
	_, err := processStreamingEvent(
		context.Background(),
		&adk.MessageVariant{IsStreaming: true, MessageStream: reader, Role: adk.Assistant},
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
	if got := content.String(); got != "夜雨落在青石街上。" {
		t.Fatalf("first narrative candidate = %q", got)
	}
	if thinking.Len() != 0 {
		t.Fatalf("candidate leaked into thinking: %q", thinking.String())
	}
	if len(events) != 1 || events[0].Type != "chunk" {
		t.Fatalf("first candidate event = %#v, want chunk", events)
	}
}

func TestProcessStreamingEventKeepsFirstInteractiveCandidateWhenLaterProseArrives(t *testing.T) {
	reader, writer := adk.Pipe[*adk.Message](1)
	writer.Send(&adk.Message{Role: adk.Assistant, Content: "废弃料场里又出现了另一段正文。"}, nil)
	writer.Close()

	var content strings.Builder
	content.WriteString("乱石坡上的首个正文候选。")
	var thinking strings.Builder
	var events []Event
	_, err := processStreamingEvent(
		context.Background(),
		&adk.MessageVariant{IsStreaming: true, MessageStream: reader, Role: adk.Assistant},
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
	if got := content.String(); got != "乱石坡上的首个正文候选。" {
		t.Fatalf("later prose replaced the locked candidate: %q", got)
	}
	if got := thinking.String(); got != "废弃料场里又出现了另一段正文。" {
		t.Fatalf("later prose thinking = %q", got)
	}
	if len(events) != 1 || events[0].Type != "thinking" {
		t.Fatalf("later prose event = %#v, want thinking", events)
	}
}

func TestProcessNonStreamingEventKeepsFirstInteractiveCandidateWhenLaterProseArrives(t *testing.T) {
	var content strings.Builder
	content.WriteString("乱石坡上的首个正文候选。")
	var thinking strings.Builder
	var events []Event

	processNonStreamingEvent(
		&adk.MessageVariant{Message: adk.AssistantMessage("废弃料场里又出现了另一段正文。", nil), Role: adk.Assistant},
		&content,
		&thinking,
		0,
		agentEventMetadata{AgentKind: AgentKindInteractiveStory},
		nil,
		func(event Event) { events = append(events, event) },
	)

	if got := content.String(); got != "乱石坡上的首个正文候选。" {
		t.Fatalf("later prose replaced the locked candidate: %q", got)
	}
	if got := thinking.String(); got != "废弃料场里又出现了另一段正文。" {
		t.Fatalf("later prose thinking = %q", got)
	}
	if len(events) != 1 || events[0].Type != "thinking" {
		t.Fatalf("later prose event = %#v, want thinking", events)
	}
}

func TestProcessStreamingEventKeepsInteractiveCompletionRetryInternal(t *testing.T) {
	reader, writer := adk.Pipe[*adk.Message](2)
	writer.Send(&adk.Message{
		Role:    adk.Assistant,
		Content: "门后传来锁链拖地的声音。",
		ResponseMeta: &adk.ResponseMeta{Usage: &adk.TokenUsage{
			PromptTokens:     120,
			CompletionTokens: 12,
			TotalTokens:      132,
		}},
	}, nil)
	writer.Send(nil, interactiveRetryErrorForTest{reason: interactiveCompletionRetryReason{Code: interactiveCompletionRetryCode}})

	var content strings.Builder
	var thinking strings.Builder
	var events []Event
	message, err := processStreamingEvent(
		context.Background(),
		&adk.MessageVariant{IsStreaming: true, MessageStream: reader, Role: adk.Assistant},
		&content,
		&thinking,
		0,
		0,
		agentEventMetadata{AgentKind: AgentKindInteractiveStory},
		nil,
		func(event Event) { events = append(events, event) },
	)
	if _, retrying := interactiveCompletionRetryFromError(err); !retrying {
		t.Fatalf("expected internal protocol retry, got %v", err)
	}
	if message == nil || message.ResponseMeta == nil || message.ResponseMeta.Usage == nil || message.ResponseMeta.Usage.TotalTokens != 132 {
		t.Fatalf("rejected model response must retain usage for accounting: %#v", message)
	}
	if content.String() != "门后传来锁链拖地的声音。" || thinking.Len() != 0 {
		t.Fatalf("rejected candidate classification mismatch: content=%q thinking=%q", content.String(), thinking.String())
	}
	if !hasEvent(events, "chunk") {
		t.Fatalf("candidate must be visible before the submission retry: %#v", events)
	}
	if hasEvent(events, "error") {
		t.Fatalf("internal retry leaked as a user-visible error: %#v", events)
	}
}

func TestProcessStreamingEventKeepsContentBeforeSubmitAsNarrative(t *testing.T) {
	reader, writer := adk.Pipe[*adk.Message](2)
	writer.Send(&adk.Message{Role: adk.Assistant, Content: "石门在轰鸣中开启。"}, nil)
	writer.Send(&adk.Message{Role: adk.Assistant, ToolCalls: []adk.ToolCall{{
		ID: "call-submit",
		Function: adk.FunctionCall{
			Name:      "submit_actor_state_patches",
			Arguments: `{"patches":[]}`,
		},
	}}}, nil)
	writer.Close()

	var content strings.Builder
	var thinking strings.Builder
	var events []Event
	_, err := processStreamingEvent(
		context.Background(),
		&adk.MessageVariant{IsStreaming: true, MessageStream: reader, Role: adk.Assistant},
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
	if content.String() != "石门在轰鸣中开启。" || thinking.Len() != 0 {
		t.Fatalf("submit response classification mismatch: content=%q thinking=%q", content.String(), thinking.String())
	}
	if hasEvent(events, interactiveContentReclassifiedEvent) {
		t.Fatalf("submit must not retract the preceding narrative: %#v", events)
	}
	if len(events) == 0 || events[0].Type != "chunk" {
		t.Fatalf("narrative must precede submit tool events: %#v", events)
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
