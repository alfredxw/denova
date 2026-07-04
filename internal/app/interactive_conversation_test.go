package app

import (
	"testing"

	"github.com/cloudwego/eino/schema"

	"denova/config"
	"denova/internal/interactive"
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

func TestInteractiveConversationModelContextMessagesEnterNextTurnWithoutThinking(t *testing.T) {
	workspace := t.TempDir()
	store := interactive.NewStore(workspace)
	story, err := store.CreateStory(interactive.CreateStoryRequest{
		Title:         "工具上下文",
		Origin:        "主角站在一扇门前。",
		StoryTellerID: "classic",
	})
	if err != nil {
		t.Fatal(err)
	}

	conversation := newInteractiveConversation(store, t.TempDir(), workspace, story.ID, "main", "检查门", 800, &config.Config{})
	if err := conversation.AppendDisplayEvent(session.DisplayEvent{Role: "thinking", Content: "隐藏思考"}); err != nil {
		t.Fatal(err)
	}
	if err := conversation.AppendContextMessage(schema.AssistantMessage("", []schema.ToolCall{{
		ID:   "call-lore",
		Type: "function",
		Function: schema.FunctionCall{
			Name:      "list_lore_items",
			Arguments: `{"query":"门"}`,
		},
	}})); err != nil {
		t.Fatal(err)
	}
	if err := conversation.AppendContextMessage(schema.ToolMessage("找到门的机关设定", "call-lore", schema.WithToolName("list_lore_items"))); err != nil {
		t.Fatal(err)
	}
	if err := conversation.AppendAssistantWithThinking("门缝里透出蓝光。", "隐藏思考"); err != nil {
		t.Fatal(err)
	}

	next := newInteractiveConversation(store, t.TempDir(), workspace, story.ID, "main", "继续观察", 800, &config.Config{})
	messages, err := next.PrepareMessages("继续观察", "继续观察")
	if err != nil {
		t.Fatal(err)
	}

	var sawToolCall, sawToolResult bool
	for _, msg := range messages {
		if msg.Role == schema.Assistant && len(msg.ToolCalls) == 1 && msg.ToolCalls[0].Function.Name == "list_lore_items" {
			sawToolCall = true
		}
		if msg.Role == schema.Tool && msg.ToolName == "list_lore_items" && msg.Content == "找到门的机关设定" {
			sawToolResult = true
		}
		if msg.Content == "隐藏思考" || msg.ReasoningContent == "隐藏思考" {
			t.Fatalf("raw thinking must not enter model context: %#v", msg)
		}
	}
	if !sawToolCall || !sawToolResult {
		t.Fatalf("next turn should include retained tool context, sawToolCall=%v sawToolResult=%v messages=%#v", sawToolCall, sawToolResult, messages)
	}
}
