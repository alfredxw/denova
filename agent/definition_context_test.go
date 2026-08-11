package agent

import (
	"strings"
	"testing"
)

func TestAssembleCycleMessagesPreservesVerbatimHostRendering(t *testing.T) {
	transcript := []*Message{UserMessage("earlier"), AssistantMessage("answer", nil)}
	messages, persisted, err := assembleCycleMessages(transcript, "raw request", []ContextFragment{
		{
			Source: "denova.stable", Purpose: "preserve localized stable context", Resource: "CREATOR.md",
			Revision: "1", Placement: ContextLeadingMessage, Rendering: ContextRenderVerbatim,
			Content: "# 创作者指令\n\n完整内容", HardLimit: 64 << 10,
		},
		{
			Source: "denova.turn", Purpose: "preserve the exact localized turn assembly", Resource: "turn",
			Revision: "command-1", Placement: ContextFinalUserMessage, Rendering: ContextRenderVerbatim,
			Content: "# 本轮上下文\n\n状态\n\n---\n\n# 本轮用户请求（最高优先级）\n\nraw request", HardLimit: 64 << 10,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 4 || messages[0].Content != "# 创作者指令\n\n完整内容" ||
		messages[3].Content != "# 本轮上下文\n\n状态\n\n---\n\n# 本轮用户请求（最高优先级）\n\nraw request" {
		t.Fatalf("messages = %#v", messages)
	}
	if persisted == nil || persisted.Content != messages[3].Content {
		t.Fatalf("persisted user = %#v", persisted)
	}
}

func TestContextFinalUserMessageIsUnambiguous(t *testing.T) {
	base := ContextFragment{
		Source: "host", Purpose: "test", Resource: "turn", Placement: ContextFinalUserMessage,
		Rendering: ContextRenderVerbatim, Content: "request", HardLimit: 64 << 10,
	}
	if err := validateContextFragments([]ContextFragment{base, base}); err == nil || !strings.Contains(err.Error(), "at most one") {
		t.Fatalf("duplicate final user message error = %v", err)
	}
	prefix := base
	prefix.Placement = ContextFinalUserPrefix
	if err := validateContextFragments([]ContextFragment{base, prefix}); err == nil || !strings.Contains(err.Error(), "cannot be combined") {
		t.Fatalf("mixed final user context error = %v", err)
	}
}
