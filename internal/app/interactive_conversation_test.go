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

func TestSubmitTurnResultToleratesUnknownFrozenActorStateFields(t *testing.T) {
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
	invalidConversation := newInteractiveConversation(store, t.TempDir(), workspace, story.ID, "main", "休息", 800, &config.Config{})
	if invalidConversation.InteractiveNarrativeReady() {
		t.Fatal("narrative must stay closed before TurnResult is staged")
	}
	base := interactive.TurnResult{
		Contract:    interactive.TurnContract{PlayerIntent: "休息"},
		SceneResult: interactive.TurnSceneResult{Status: "continued"},
		PlanSignals: interactive.TurnPlanSignals{DeviationLevel: "none"},
		Choices:     []string{"继续休息", "检查当前状态"},
	}
	wrongType := base
	wrongType.ActorStatePatches = []interactive.ActorStatePatch{{ActorID: "protagonist", State: map[string]any{"生命值": "很多"}}}
	receipt, err := invalidConversation.SubmitTurnResult(context.Background(), wrongType)
	if err != nil || receipt.Accepted || !receipt.Retryable || len(receipt.Diagnostics) != 1 || !strings.Contains(receipt.Diagnostics[0].Message, "生命值") {
		t.Fatalf("wrong field type should return model-correctable feedback: receipt=%#v err=%v", receipt, err)
	}
	if invalidConversation.InteractiveNarrativeReady() {
		t.Fatal("rejected submission must not open the narrative phase")
	}

	conversation := newInteractiveConversation(store, t.TempDir(), workspace, story.ID, "main", "休息", 800, &config.Config{})
	withUnknownExtra := base
	withUnknownExtra.ActorStatePatches = []interactive.ActorStatePatch{{ActorID: "protagonist", State: map[string]any{
		"当前身体/精神 状态":  "安定",
		"body.status": "良好",
	}}}
	receipt, err = conversation.SubmitTurnResult(context.Background(), withUnknownExtra)
	if err != nil || !receipt.Accepted || receipt.Retryable || len(receipt.Diagnostics) != 1 || receipt.Diagnostics[0].Code != interactive.TurnSubmissionDiagnosticUnknownActorStateField {
		t.Fatalf("unknown extra field should be dropped with an accepted warning: receipt=%#v err=%v", receipt, err)
	}
	if !conversation.InteractiveNarrativeReady() {
		t.Fatal("narrative must open immediately after TurnResult is staged")
	}
	replacement := base
	replacement.ActorStatePatches = []interactive.ActorStatePatch{{ActorID: "protagonist", State: map[string]any{"当前身体/精神 状态": "躁动"}}}
	receipt, err = conversation.SubmitTurnResult(context.Background(), replacement)
	if err != nil || !receipt.Accepted || len(receipt.Diagnostics) != 1 || receipt.Diagnostics[0].Code != "turn_result_already_accepted" {
		t.Fatalf("duplicate accepted result should be idempotent: receipt=%#v err=%v", receipt, err)
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

func TestSubmitTurnResultRequiresAndCommitsStoryContext(t *testing.T) {
	workspace := t.TempDir()
	store := interactive.NewStore(workspace)
	actorState := interactive.DefaultActorStateModule().ActorState
	story, err := store.CreateStory(interactive.CreateStoryRequest{Title: "故事上下文提交", ActorState: &actorState})
	if err != nil {
		t.Fatal(err)
	}
	result := interactive.TurnResult{
		Contract:    interactive.TurnContract{PlayerIntent: "进入酒馆", SceneGoal: "确认酒馆内的局势"},
		SceneResult: interactive.TurnSceneResult{Status: "continued"},
		PlanSignals: interactive.TurnPlanSignals{DeviationLevel: "none"},
		Choices:     []string{"观察堂内客人", "询问掌柜"},
	}

	missing := newInteractiveConversation(store, t.TempDir(), workspace, story.ID, "main", "进入酒馆", 800, &config.Config{})
	receipt, err := missing.SubmitTurnResult(context.Background(), result)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Accepted || !receipt.Retryable || len(receipt.Diagnostics) != 1 || receipt.Diagnostics[0].Code != interactive.TurnSubmissionDiagnosticStoryContextRequired {
		t.Fatalf("missing story context should request a corrected TurnResult: %#v", receipt)
	}
	if missing.InteractiveNarrativeReady() {
		t.Fatal("narrative must remain closed until story context is submitted")
	}

	eventOnly := result
	eventOnly.ActorStatePatches = []interactive.ActorStatePatch{{
		ActorID: interactive.DefaultStoryContextActorID,
		State:   map[string]any{"当前事件": "主角进入黄泉酒馆并观察堂内局势"},
	}}
	withoutLocation := newInteractiveConversation(store, t.TempDir(), workspace, story.ID, "main", "进入酒馆", 800, &config.Config{})
	receipt, err = withoutLocation.SubmitTurnResult(context.Background(), eventOnly)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Accepted || !receipt.Retryable || len(receipt.Diagnostics) != 1 || receipt.Diagnostics[0].Code != interactive.TurnSubmissionDiagnosticStoryContextRequired || receipt.Diagnostics[0].Field != "当前详细地点" {
		t.Fatalf("uninitialized story context should require a current location even without scene_id: %#v", receipt)
	}

	result.ActorStatePatches = []interactive.ActorStatePatch{{
		ActorID: interactive.DefaultStoryContextActorID,
		State: map[string]any{
			"当前详细地点": "黄泉酒馆",
			"当前事件":   "主角进入黄泉酒馆并观察堂内局势",
		},
		Reason: "同步本回合结束后的场景与事件。",
	}}
	conversation := newInteractiveConversation(store, t.TempDir(), workspace, story.ID, "main", "进入酒馆", 800, &config.Config{})
	receipt, err = conversation.SubmitTurnResult(context.Background(), result)
	if err != nil || !receipt.Accepted || receipt.Retryable {
		t.Fatalf("complete story context should be accepted: receipt=%#v err=%v", receipt, err)
	}
	if err := conversation.AppendAssistantWithThinking("主角推门走进黄泉酒馆。", ""); err != nil {
		t.Fatalf("commit narrative with story context: %v", err)
	}

	snapshot, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	actors, _ := snapshot.State["actors"].(map[string]any)
	storyActor, _ := actors[interactive.DefaultStoryContextActorID].(map[string]any)
	storyState, _ := storyActor["state"].(map[string]any)
	if snapshot.CurrentTurn == nil || snapshot.CurrentTurn.Narrative != "主角推门走进黄泉酒馆。" || storyState["当前详细地点"] != "黄泉酒馆" || storyState["当前事件"] != "主角进入黄泉酒馆并观察堂内局势" {
		t.Fatalf("narrative and story context must survive the same commit: turn=%#v state=%#v", snapshot.CurrentTurn, storyState)
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

func TestInteractiveConversationDropsTransientIndexAndThinkingFromNextTurn(t *testing.T) {
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
			Arguments: `{"keywords":["门"]}`,
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

	for _, msg := range messages {
		if msg.Role == schema.Assistant && len(msg.ToolCalls) == 1 && msg.ToolCalls[0].Function.Name == "list_lore_items" {
			t.Fatalf("transient Lore index call must not enter next-turn context: %#v", msg)
		}
		if msg.Role == schema.Tool && msg.ToolName == "list_lore_items" {
			t.Fatalf("transient Lore index result must not enter next-turn context: %#v", msg)
		}
		if msg.Content == "隐藏思考" || msg.ReasoningContent == "隐藏思考" {
			t.Fatalf("raw thinking must not enter model context: %#v", msg)
		}
	}
}
