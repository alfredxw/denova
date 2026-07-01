package app

import (
	"testing"

	"denova/internal/session"
)

func TestInteractiveConversationToolResultFallsBackToNameWhenIDMissing(t *testing.T) {
	conversation := &interactiveConversation{}
	if err := conversation.AppendDisplayEvent(session.DisplayEvent{ID: "call-execute", Role: "tool_call", Name: "execute", Content: "execute", Status: "running"}); err != nil {
		t.Fatal(err)
	}
	if err := conversation.UpdateDisplayToolResult("", "execute", "success", "command done"); err != nil {
		t.Fatal(err)
	}

	if len(conversation.displayEvents) != 1 {
		t.Fatalf("展示事件数量不符合预期: %#v", conversation.displayEvents)
	}
	event := conversation.displayEvents[0]
	if event.Status != "success" || event.Result != "command done" {
		t.Fatalf("id 缺失时应按唯一工具名更新互动工具卡片: %#v", event)
	}
}

func TestInteractiveConversationToolResultDoesNotFallbackWhenIDDiffers(t *testing.T) {
	conversation := &interactiveConversation{}
	if err := conversation.AppendDisplayEvent(session.DisplayEvent{ID: "call-execute", Role: "tool_call", Name: "execute", Content: "execute", Status: "running"}); err != nil {
		t.Fatal(err)
	}
	if err := conversation.UpdateDisplayToolResult("stale-id", "execute", "success", "stale result"); err != nil {
		t.Fatal(err)
	}

	if len(conversation.displayEvents) != 1 {
		t.Fatalf("展示事件数量不符合预期: %#v", conversation.displayEvents)
	}
	event := conversation.displayEvents[0]
	if event.Result == "stale result" || event.Status != "running" {
		t.Fatalf("id 不一致时不应按工具名更新互动工具卡片: %#v", event)
	}
}

func TestInteractiveConversationToolResultDoesNotFallbackWhenNameIsAmbiguous(t *testing.T) {
	conversation := &interactiveConversation{}
	if err := conversation.AppendDisplayEvent(session.DisplayEvent{ID: "execute-1", Role: "tool_call", Name: "execute", Content: "execute", Status: "running"}); err != nil {
		t.Fatal(err)
	}
	if err := conversation.AppendDisplayEvent(session.DisplayEvent{ID: "execute-2", Role: "tool_call", Name: "execute", Content: "execute", Status: "running"}); err != nil {
		t.Fatal(err)
	}
	if err := conversation.UpdateDisplayToolResult("stale-id", "execute", "success", "ambiguous result"); err != nil {
		t.Fatal(err)
	}

	for _, event := range conversation.displayEvents {
		if event.Result == "ambiguous result" || event.Status != "running" {
			t.Fatalf("同名工具调用存在歧义时不应按工具名误更新: %#v", event)
		}
	}
}
