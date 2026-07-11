package app

import (
	"context"
	"strings"
	"testing"

	"github.com/cloudwego/eino/schema"

	"denova/config"
	"denova/internal/interactive"
	"denova/internal/session"
)

func TestSubmitTurnResultValidatesFrozenActorStateImmediately(t *testing.T) {
	workspace := t.TempDir()
	store := interactive.NewStore(workspace)
	actorState := interactive.StoryDirectorActorStateSystem{
		Templates: []interactive.ActorStateTemplate{{
			ID:   "protagonist",
			Name: "主角",
			Fields: []interactive.ActorStateField{
				{Name: "当前身体/精神 状态", Type: "string", Order: 10},
				{Name: "生命值", Type: "number", Order: 20},
			},
		}},
		InitialActors: []interactive.ActorStateInitialActor{{
			ID: "protagonist", Name: "主角", TemplateID: "protagonist",
			State: map[string]any{"当前身体/精神 状态": "正常", "生命值": 10},
		}},
	}
	story, err := store.CreateStory(interactive.CreateStoryRequest{Title: "即时校验", ActorState: &actorState})
	if err != nil {
		t.Fatal(err)
	}
	conversation := newInteractiveConversation(store, t.TempDir(), workspace, story.ID, "main", "休息", 800, &config.Config{})
	base := interactive.TurnResult{
		Contract:    interactive.TurnContract{PlayerIntent: "休息"},
		SceneResult: interactive.TurnSceneResult{Status: "continued"},
		PlanSignals: interactive.TurnPlanSignals{DeviationLevel: "none"},
	}
	unknown := base
	unknown.ActorStatePatches = []interactive.ActorStatePatch{{ActorID: "protagonist", State: map[string]any{"body.status": "良好"}}}
	if _, err := conversation.SubmitTurnResult(context.Background(), unknown); err == nil || !strings.Contains(err.Error(), "合法状态名称") {
		t.Fatalf("unknown field should fail at tool invocation with allowed names, err=%v", err)
	}
	wrongType := base
	wrongType.ActorStatePatches = []interactive.ActorStatePatch{{ActorID: "protagonist", State: map[string]any{"生命值": "很多"}}}
	if _, err := conversation.SubmitTurnResult(context.Background(), wrongType); err == nil || !strings.Contains(err.Error(), "生命值") {
		t.Fatalf("wrong field type should fail at tool invocation, err=%v", err)
	}
	valid := base
	valid.ActorStatePatches = []interactive.ActorStatePatch{{ActorID: "protagonist", State: map[string]any{"当前身体/精神 状态": "安定"}}}
	if _, err := conversation.SubmitTurnResult(context.Background(), valid); err != nil {
		t.Fatalf("valid localized field ID should be staged: %v", err)
	}
	if err := conversation.AppendAssistantWithThinking("主角平复了呼吸。", ""); err != nil {
		t.Fatalf("validated result and narrative should commit atomically: %v", err)
	}
	snapshot, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	actors, _ := snapshot.State["actors"].(map[string]any)
	actor, _ := actors["protagonist"].(map[string]any)
	state, _ := actor["state"].(map[string]any)
	if snapshot.CurrentTurn == nil || snapshot.CurrentTurn.Narrative != "主角平复了呼吸。" || state["当前身体/精神 状态"] != "安定" {
		t.Fatalf("narrative and actor state must survive the same commit: turn=%#v state=%#v", snapshot.CurrentTurn, state)
	}
}

func TestDirectorContextBudgetFollowsModelWindowAndCapsEachSource(t *testing.T) {
	small := newDirectorContextBudget(&config.Config{OpenAIContextWindowTokens: 128000}, interactiveDirectorTaskDirectorPlanUpdate)
	large := newDirectorContextBudget(&config.Config{OpenAIContextWindowTokens: 400000}, interactiveDirectorTaskDirectorPlanUpdate)
	if small.thresholdTokens != 115200 || large.thresholdTokens != 360000 {
		t.Fatalf("threshold tokens should follow the configured 90%% model window: small=%d large=%d", small.thresholdTokens, large.thresholdTokens)
	}
	if large.initialTokens <= small.initialTokens {
		t.Fatalf("larger model window should expose a larger source budget: small=%d large=%d", small.initialTokens, large.initialTokens)
	}
	value := strings.Repeat("完整导演来源。", interactive.DirectorContextMaxBytes)
	kept := large.take("large.source", value, interactive.DirectorContextMaxBytes*2)
	if len(kept) > interactive.DirectorContextMaxBytes {
		t.Fatalf("single source exceeded 64KB hard limit: %d", len(kept))
	}
}

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
	submitTestTurnResult(t, conversation, "观察门缝", "确认蓝光来源")
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
