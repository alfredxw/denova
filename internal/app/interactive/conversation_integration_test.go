package interactiveapp

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"denova/config"
	agents "denova/internal/agents"
	agentcontext "denova/internal/agents/context"
	agentcompaction "denova/internal/agents/context/compaction"
	"denova/internal/agents/session"
	"denova/internal/book/lore"
	"denova/internal/interactive"
	interactivestate "denova/internal/interactive/state"

	agent "github.com/alfredxw/denova/agent"
	publiccontext "github.com/alfredxw/denova/agent/context"
)

func TestInteractiveConversationBuildsHistoryAndPersistsAssistantToStory(t *testing.T) {
	workspace := t.TempDir()
	loreStore := lore.NewStore(workspace)
	if _, err := loreStore.Create(lore.ItemInput{ID: "hero", Type: "character", Name: "林川", Importance: "major", LoadMode: lore.LoadModeResident, Content: "林川：谨慎的幸存者"}); err != nil {
		t.Fatal(err)
	}
	if _, err := loreStore.Create(lore.ItemInput{ID: "world", Type: "world", Name: "黄昏末日", Importance: "major", LoadMode: lore.LoadModeResident, Content: "世界已进入黄昏末日。"}); err != nil {
		t.Fatal(err)
	}
	if _, err := loreStore.Create(lore.ItemInput{ID: "base", Type: "location", Name: "黄泉酒馆", Importance: "important", LoadMode: lore.LoadModeAuto, BriefDescription: "黄泉酒馆据点索引", Content: "黄泉酒馆完整设定：柜台后的影子不能离开酒馆。"}); err != nil {
		t.Fatal(err)
	}
	novaDir := t.TempDir()
	store := interactive.NewStore(workspace)
	story, err := store.CreateStory(interactive.CreateStoryRequest{
		Title:            "末日开端",
		Origin:           "主角醒来发现世界已末日",
		StoryTellerID:    "classic",
		ReplyTargetChars: 800,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.AppendTurn(story.ID, interactive.AppendTurnRequest{
		User:      "我推开酒馆的门",
		Narrative: "门后传来低沉的风声。",
	}); err != nil {
		t.Fatal(err)
	}

	conversation := NewConversation(store, novaDir, workspace, story.ID, "", "我在黄泉酒馆点燃火把", story.ReplyTargetChars, nil)
	assembled, err := conversation.AssembleModelContext(context.Background(), "我在黄泉酒馆点燃火把", agentcontext.ModelContextInput{
		UserMessage: "我在黄泉酒馆点燃火把",
		Budget:      conversation.ModelContextBudget(),
	})
	if err != nil {
		t.Fatal(err)
	}
	lifecycleFragments, err := publiccontext.ExportLifecycleFragments(assembled.Context)
	if err != nil {
		t.Fatal(err)
	}
	residentStable := false
	for _, fragment := range lifecycleFragments {
		if fragment.Source != "interactive.resident_lore" {
			continue
		}
		residentStable = fragment.Stability == agent.ContextStablePrefix && fragment.Placement == agent.ContextLeadingMessage && fragment.StateID == ""
	}
	if !residentStable {
		t.Fatalf("resident lore must be one replaceable stable-prefix fragment: %#v", lifecycleFragments)
	}
	if err := conversation.CommitModelInput(context.Background(), "我在黄泉酒馆点燃火把", assembled); err != nil {
		t.Fatal(err)
	}
	history := assembled.Messages
	if len(history) != 4 {
		t.Fatalf("history length = %d, want 4", len(history))
	}
	if history[0].Role != agents.RoleUser || !strings.Contains(history[0].Content, "Resident Lore") || !strings.Contains(history[0].Content, "林川：谨慎的幸存者") || !strings.Contains(history[0].Content, "世界已进入黄昏末日") {
		t.Fatalf("history[0] should be stable resident lore: %#v", history[0])
	}
	if history[1].Role != agents.RoleUser || history[1].Content != "我推开酒馆的门" {
		t.Fatalf("history[0] mismatch: %#v", history[0])
	}
	if strings.Contains(history[1].Content, "History Checkpoint") || strings.Contains(history[1].Content, "Highest length constraint") {
		t.Fatalf("history[1] should remain plain story history, got: %#v", history[1])
	}
	if history[2].Role != agents.RoleAssistant || history[2].Content != "门后传来低沉的风声。" {
		t.Fatalf("history[2] mismatch: %#v", history[2])
	}
	if history[3].Role != agents.RoleUser || !strings.Contains(history[3].Content, "我在黄泉酒馆点燃火把") {
		t.Fatalf("history[3] mismatch: %#v", history[3])
	}
	for _, want := range []string{
		"Storyteller Rules for This Turn",
		"[Current Turn Runtime Context]",
		"800 Chinese characters",
		"Highest length constraint",
		"list_lore_items",
		"search_story_history",
		"turn_id",
		"Game Agent Planning",
		"Status: disabled",
		"bounded",
		"# Actor State Handbook",
		"Actor ID: `protagonist`",
		"Field description:",
		"Update instruction:",
		"submit_interactive_turn",
		`"state_changes"`,
	} {
		if !strings.Contains(history[3].Content, want) {
			t.Fatalf("history[3] should include %q: %#v", want, history[3])
		}
	}
	if strings.Contains(history[3].Content, "随机事件率") {
		t.Fatalf("story prose prompt should not receive event probability controls: %#v", history[2])
	}
	for _, rawSchemaMarker := range []string{`"create_templates"`, `"state_system"`, `"writable_fields"`} {
		if strings.Contains(history[3].Content, rawSchemaMarker) {
			t.Fatalf("game prompt should contain the Markdown state guide, not duplicated raw schema marker %q", rawSchemaMarker)
		}
	}
	for _, forbidden := range []string{"经典叙事者", "林川：谨慎的幸存者", "世界已进入黄昏末日。"} {
		if strings.Contains(history[3].Content, forbidden) {
			t.Fatalf("history[3] should not include %q: %#v", forbidden, history[3])
		}
	}
	if strings.Contains(history[3].Content, "维护当前阶段的隐藏真相、阶段高潮") {
		t.Fatalf("Game Agent model input must not contain private director.md content: %#v", history[3])
	}
	for _, forbidden := range []string{"末日开端", "主角醒来发现世界已末日"} {
		if strings.Contains(history[3].Content, forbidden) {
			t.Fatalf("history[3] should keep story metadata out of the turn instruction %q: %#v", forbidden, history[3])
		}
	}
	sources := conversation.ContextSourceSummary()
	for _, want := range []string{
		"InteractiveStory",
		"Story Title",
		"末日开端",
		"Opening",
		"主角醒来发现世界已末日",
		"StorytellerRule",
		"本轮上下文",
		"GamePreset",
		"Game Preset Rule Catalog",
	} {
		if !strings.Contains(sources, want) {
			t.Fatalf("context sources should include %q: %s", want, sources)
		}
	}
	ledgerParts := conversation.ContextLedgerParts()
	var sawResidentLore, sawActiveLore, sawCurrentAction bool
	for _, part := range ledgerParts {
		if part.Source == "ResidentLore" && part.Bytes > 0 && part.Limit > lore.ResidentLoreSafetyMaxBytes && part.LimitUnit == "bytes" && strings.Contains(part.Note, "complete=true") && strings.Contains(part.Note, "revision=") && strings.Contains(part.Note, "exact_final_message=true") {
			sawResidentLore = true
		}
		if part.Source == "LoreContext" && part.Title == "Current Branch Active Lore Working Set" && part.Bytes > 0 {
			sawActiveLore = true
		}
		if part.Source == "CurrentTurn" && part.Title == "Current User Action" && part.Bytes > 0 {
			sawCurrentAction = true
		}
	}
	if !sawResidentLore || !sawActiveLore || !sawCurrentAction {
		t.Fatalf("durable context fragments should distinguish resident lore, active lore, and current action metadata: %#v", ledgerParts)
	}

	submitTestTurnResult(t, conversation, "点燃火把", "照亮酒馆墙面")
	if err := commitInteractiveAssistantForTest(t, conversation, "火光照亮了墙上的新线索。", "先判断现场风险。"); err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.Snapshot(story.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Turns) != 2 {
		t.Fatalf("turn count = %d, want 2", len(snapshot.Turns))
	}
	last := snapshot.Turns[1]
	if last.User != "我在黄泉酒馆点燃火把" || last.Narrative != "火光照亮了墙上的新线索。" {
		t.Fatalf("last turn mismatch: %#v", last)
	}
	traceMetadata := conversation.RunTraceMetadata()
	if traceMetadata.StoryID != story.ID || traceMetadata.BranchID != last.BranchID || traceMetadata.TurnID != last.ID {
		t.Fatalf("committed turn trace metadata mismatch: %#v", traceMetadata)
	}
	if last.Thinking != "先判断现场风险。" {
		t.Fatalf("last thinking = %q, want persisted thinking", last.Thinking)
	}
	storyEventCommitted := false
	if last.StateDelta != nil {
		for _, op := range last.StateDelta.ActorOps {
			storyEventCommitted = storyEventCommitted || op.ActorID == interactive.DefaultStoryContextActorID && op.FieldID == "当前事件"
		}
	}
	if !storyEventCommitted {
		t.Fatalf("turn should atomically persist the required story context: %#v", last.StateDelta)
	}
	if _, err := store.AppendStateDelta(story.ID, interactive.AppendStateDeltaRequest{
		ParentID: last.ID,
		BranchID: last.BranchID,
		Ops: []interactivestate.Op{
			{Op: "set", Path: "on_stage", Value: []any{"林川"}},
			{Op: "merge", Path: "characters.林川", Value: map[string]any{"location": "黄泉酒馆"}},
		},
	}); err != nil {
		t.Fatal(err)
	}
	snapshot, err = store.Snapshot(story.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	onStage := snapshot.State["on_stage"].([]any)
	if len(onStage) != 1 || onStage[0] != "林川" {
		t.Fatalf("unexpected on_stage: %#v", onStage)
	}
	characters := snapshot.State["characters"].(map[string]any)
	linchuan := characters["林川"].(map[string]any)
	if linchuan["location"] != "黄泉酒馆" {
		t.Fatalf("unexpected character state: %#v", linchuan)
	}

	submitTestTurnResult(t, conversation, "继续调查", "确认柜台后的通道")
	if err := commitInteractiveAssistantForTest(t, conversation, "柜台后的影子露出一道能通往地窖的缝。", ""); err != nil {
		t.Fatal(err)
	}
	snapshot, err = store.Snapshot(story.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	nextTurn := snapshot.Turns[len(snapshot.Turns)-1]
	if _, err := store.AppendStateDelta(story.ID, interactive.AppendStateDeltaRequest{
		ParentID: nextTurn.ID,
		BranchID: nextTurn.BranchID,
		Ops: []interactivestate.Op{
			{Op: "merge", Path: "scene", Value: map[string]any{"danger_level": "升高", "interactive_objects": []any{"柜台", "地窖门"}}},
			{Op: "push", Path: "action_space", Value: map[string]any{"target": "地窖门", "risk": "可能惊动柜台后的影子"}},
			{Op: "push", Path: "threads", Value: map[string]any{"title": "柜台后的影子", "status": "未解决"}},
			{Op: "push", Path: "world_flags", Value: "黄泉酒馆会回应火光"},
		},
	}); err != nil {
		t.Fatal(err)
	}
	snapshot, err = store.Snapshot(story.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	scene := snapshot.State["scene"].(map[string]any)
	if scene["danger_level"] != "升高" {
		t.Fatalf("unexpected scene state: %#v", scene)
	}
	actionSpace := snapshot.State["action_space"].([]any)
	if len(actionSpace) != 1 {
		t.Fatalf("unexpected action_space: %#v", actionSpace)
	}
	threads := snapshot.State["threads"].([]any)
	if len(threads) != 1 {
		t.Fatalf("unexpected threads: %#v", threads)
	}
}

func TestInteractiveConversationRejectsAssistantWithoutTurnResult(t *testing.T) {
	workspace := t.TempDir()
	store := interactive.NewStore(workspace)
	story, err := store.CreateStory(interactive.CreateStoryRequest{
		Title:            "不完整回合",
		StoryTellerID:    "classic",
		ReplyTargetChars: 800,
	})
	if err != nil {
		t.Fatal(err)
	}
	conversation := NewConversation(store, t.TempDir(), workspace, story.ID, "main", "继续前进", story.ReplyTargetChars, nil)
	if err := conversation.AppendAssistant("主角向前走去。"); err == nil || !strings.Contains(err.Error(), "state_changes") || !strings.Contains(err.Error(), "choices") {
		t.Fatalf("assistant without TurnResult should be rejected, got %v", err)
	}
	snapshot, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Turns) != 0 {
		t.Fatalf("rejected assistant must not persist a partial turn: %#v", snapshot.Turns)
	}
}

func TestGamePresetProjectsCreatorPlanningStyleWithoutBackendPacing(t *testing.T) {
	prompt := "- 避免连续两回合使用同类型突发事件。\n- 伏笔回收前至少给一次可感知征兆。"
	guide := interactive.StoryPlanningGuideMarkdown(interactive.StoryDirector{
		ID: "custom-strategy", Name: "自定义游戏预设",
		Strategy: interactive.StoryDirectorStrategy{PromptMarkdown: prompt},
	}, StoryRuntimeContextMaxBytes)
	for _, want := range []string{"Planning document template", "unique ATX H2", "避免连续两回合", "伏笔回收前"} {
		if !strings.Contains(guide, want) {
			t.Fatalf("planning guide should include creator-authored style %q:\n%s", want, guide)
		}
	}
	for _, forbidden := range []string{"pacing_curve", "event_frequency", "branch_planning_turns", "cadence"} {
		if strings.Contains(guide, forbidden) {
			t.Fatalf("planning guide should not contain backend pacing field %q:\n%s", forbidden, guide)
		}
	}
}

func TestGamePresetProjectsEnabledEventCardsIntoPlanningGuide(t *testing.T) {
	preset := interactive.StoryDirector{
		ID: "event-card-preset", Name: "事件素材预设",
		EventPackages: []interactive.EventPackage{{
			ID: "academy-pack", Name: "学院事件包", Enabled: true,
			Events: []interactive.EventCard{{
				ID: "academy_trial", TypeName: "外门考核打脸", Enabled: true,
				DescriptionMarkdown: "## 触发场景\n外门考核中同门当众质疑主角。\n\n## 事件回收 / 后果\n以后续榜单与戒律回收。",
			}},
		}},
	}
	guide := interactive.StoryPlanningGuideMarkdown(preset, StoryRuntimeContextMaxBytes)
	for _, want := range []string{"Optional event material", "学院事件包", "外门考核打脸", "同门当众质疑主角"} {
		if !strings.Contains(guide, want) {
			t.Fatalf("planning guide should include enabled event material %q:\n%s", want, guide)
		}
	}
	for _, forbidden := range []string{"cadence_not_due", "Event Runtime", "event_frequency"} {
		if strings.Contains(guide, forbidden) {
			t.Fatalf("planning guide should not include backend scheduling state %q:\n%s", forbidden, guide)
		}
	}
}

func TestInteractiveConversationPersistsRuleResolution(t *testing.T) {
	workspace := t.TempDir()
	novaDir := filepath.Join(workspace, ".nova")
	store, director := newInteractiveStoreWithHPTestDirector(t, workspace, novaDir)
	story, err := store.CreateStory(interactive.CreateStoryRequest{
		Title:           "规则审计",
		Origin:          "主角站在秘境入口",
		StoryTellerID:   "classic",
		StoryDirectorID: director.ID,
		ActorState:      &director.ActorState,
	})
	if err != nil {
		t.Fatal(err)
	}
	conversation := NewConversation(store, novaDir, workspace, story.ID, "main", "我强闯秘境入口", story.ReplyTargetChars, &config.Config{})
	resolution, err := conversation.PrepareInteractiveTurn(
		context.Background(),
		interactive.TurnCheckRequest{
			Action:     "我强闯秘境入口",
			Intent:     "冒险",
			Challenge:  "秘境禁制",
			Cost:       "失败会导致禁制反噬",
			State:      "主角站在秘境入口，禁制正在收束。",
			Difficulty: "very_hard",
			Outcomes: interactive.TurnCheckOutcomes{
				CriticalSuccess: interactive.TurnCheckOutcome{Result: "强闯成功。", StateChanges: []interactive.TurnStateChange{{ActorID: "protagonist", FieldID: "生命", Change: -1, Reason: "禁制擦伤。"}}},
				Success:         interactive.TurnCheckOutcome{Result: "勉强闯入。", StateChanges: []interactive.TurnStateChange{{ActorID: "protagonist", FieldID: "生命", Change: -1, Reason: "硬闯消耗生命。"}}},
				Failure:         interactive.TurnCheckOutcome{Result: "被禁制震回。", StateChanges: []interactive.TurnStateChange{{ActorID: "protagonist", FieldID: "生命", Change: -1, Reason: "禁制反震。"}}},
				CriticalFailure: interactive.TurnCheckOutcome{Result: "禁制彻底反噬。", StateChanges: []interactive.TurnStateChange{{ActorID: "protagonist", FieldID: "生命", Change: -1, Reason: "禁制严重反噬。"}}},
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	lockedResolution, err := conversation.PrepareInteractiveTurn(
		context.Background(),
		interactive.TurnCheckRequest{
			Action: "我改为绕开入口", Intent: "规避风险", Challenge: "寻找侧路", Cost: "浪费时间", State: "主角仍在入口。",
			Difficulty: "easy",
			Outcomes: interactive.TurnCheckOutcomes{
				CriticalSuccess: interactive.TurnCheckOutcome{Result: "找到捷径。"},
				Success:         interactive.TurnCheckOutcome{Result: "找到侧路。"},
				Failure:         interactive.TurnCheckOutcome{Result: "没有找到。"},
				CriticalFailure: interactive.TurnCheckOutcome{Result: "触发新的禁制。"},
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if lockedResolution.ID != resolution.ID || lockedResolution.Result.ID != resolution.Result.ID || lockedResolution.Seed != resolution.Seed {
		t.Fatalf("repeated turn checks must return the first locked resolution: first=%#v repeated=%#v", resolution, lockedResolution)
	}
	submitTestTurnResult(t, conversation, "闯入秘境", "裁定入口禁制")
	if err := commitInteractiveAssistantForTest(t, conversation, "秘境入口的白光猛然坍缩，主角被禁制震回台阶。", ""); err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.CurrentTurn == nil || snapshot.CurrentTurn.RuleResolution == nil {
		t.Fatalf("turn audit missing: %#v", snapshot.CurrentTurn)
	}
	if snapshot.CurrentTurn.RuleResolution.ID != resolution.ID {
		t.Fatalf("rule resolution id mismatch: %#v", snapshot.CurrentTurn.RuleResolution)
	}
	if snapshot.CurrentTurn.RuleResolution.StateConsumption == nil || snapshot.CurrentTurn.RuleResolution.StateConsumption.Status != "applied" {
		t.Fatalf("state consumption audit missing: %#v", snapshot.CurrentTurn.RuleResolution)
	}
	if snapshot.CurrentTurn.StateDelta == nil || len(snapshot.CurrentTurn.StateDelta.ActorOps) != 1 || snapshot.CurrentTurn.StateDelta.ActorOps[0].SourceKind != interactive.StateOpSourceRuleResolution {
		t.Fatalf("rule state op missing: %#v", snapshot.CurrentTurn.StateDelta)
	}
}

func newInteractiveStoreWithHPTestDirector(t *testing.T, workspace, novaDir string) (*interactive.Store, interactive.StoryDirector) {
	t.Helper()
	hpMin, hpMax := 0.0, 10.0
	actorState, err := interactive.NewActorStateLibrary(novaDir).Create(interactive.ActorStateModule{
		ID:   "hp-test-state",
		Name: "生命测试状态",
		ActorState: interactive.StoryDirectorActorStateSystem{
			Templates: []interactive.ActorStateTemplate{{
				ID:   "protagonist",
				Name: "主角",
				Fields: []interactive.ActorStateField{{
					ID:      "hp",
					Path:    "resources.hp",
					Name:    "生命",
					Type:    "number",
					Default: 10.0,
					Min:     &hpMin,
					Max:     &hpMax,
				}},
			}},
			InitialActors: []interactive.ActorStateInitialActor{{
				ID:         interactive.DefaultActorID,
				Name:       "主角",
				TemplateID: "protagonist",
				Role:       "protagonist",
			}},
		},
	})
	if err != nil {
		t.Fatalf("create hp actor state failed: %v", err)
	}
	director, err := interactive.NewStoryDirectorLibrary(novaDir).Create(interactive.StoryDirector{
		ID:   "hp-test-director",
		Name: "生命测试导演",
		ModuleRefs: interactive.StoryDirectorModuleRefs{
			NarrativeStyleDisabled: true,
			EventPackagesDisabled:  true,
			RuleSystemDisabled:     true,
			ActorStateID:           actorState.ID,
			ImagePresetDisabled:    true,
		},
	})
	if err != nil {
		t.Fatalf("create hp test director failed: %v", err)
	}
	return interactive.NewStoreWithNovaDir(workspace, novaDir), director
}

func TestInteractiveConversationPersistsDisplayEventTimeline(t *testing.T) {
	workspace := t.TempDir()
	store := interactive.NewStore(workspace)
	story, err := store.CreateStory(interactive.CreateStoryRequest{
		Title:         "工具时间线",
		Origin:        "主角进入档案室",
		StoryTellerID: "classic",
	})
	if err != nil {
		t.Fatal(err)
	}
	conversation := NewConversation(store, t.TempDir(), workspace, story.ID, "main", "检查档案柜", 800, &config.Config{})

	if err := conversation.AppendDisplayEvent(session.DisplayEvent{Role: "thinking", Content: "先分析档案室线索。"}); err != nil {
		t.Fatal(err)
	}
	if err := conversation.AppendDisplayEvent(session.DisplayEvent{ID: "call-1", Role: "tool_call", Name: "list_lore_items", Content: "list_lore_items", Status: "running"}); err != nil {
		t.Fatal(err)
	}
	if err := conversation.AppendDisplayToolArgs("call-1", "list_lore_items", `{"keywords":["档案室"]}`); err != nil {
		t.Fatal(err)
	}
	if err := conversation.UpdateDisplayToolResult("call-1", "list_lore_items", "success", "找到档案室设定", nil); err != nil {
		t.Fatal(err)
	}
	if err := conversation.AppendDisplayEvent(session.DisplayEvent{Role: "thinking", Content: "第二轮基于工具结果继续判断。"}); err != nil {
		t.Fatal(err)
	}
	if err := conversation.AppendDisplayEvent(session.DisplayEvent{ID: "call-2", Role: "tool_call", Name: "search_story_history", Content: "search_story_history", Args: `{"keywords":["钟楼"]}`, Status: "running"}); err != nil {
		t.Fatal(err)
	}
	if err := conversation.UpdateDisplayToolResult("call-2", "search_story_history", "success", "找到 1 个历史回合", nil); err != nil {
		t.Fatal(err)
	}
	submitTestTurnResult(t, conversation, "调查档案柜", "找到档案室线索")
	if err := commitInteractiveAssistantForTest(t, conversation, "档案柜里露出一张潮湿的地图。", "先分析档案室线索。第二轮基于工具结果继续判断。"); err != nil {
		t.Fatal(err)
	}

	snapshot, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	events := snapshot.Turns[0].DisplayEvents
	if len(events) != 4 {
		t.Fatalf("display event count = %d, want 4: %#v", len(events), events)
	}
	if events[0].Role != "thinking" || events[1].Name != "list_lore_items" || events[2].Role != "thinking" || events[3].Name != "search_story_history" {
		t.Fatalf("display events order mismatch: %#v", events)
	}
	if events[1].Args != `{"keywords":["档案室"]}` || events[1].Result != "找到档案室设定" || events[1].Status != "success" {
		t.Fatalf("first tool event details mismatch: %#v", events[1])
	}
	if events[3].Args == "" || events[3].Result != "找到 1 个历史回合" || events[3].Status != "success" {
		t.Fatalf("second tool event details mismatch: %#v", events[3])
	}
}

func TestInteractiveConversationIgnoresLegacyTellerReplyTargetChars(t *testing.T) {
	workspace := t.TempDir()
	novaDir := t.TempDir()
	tellerDir := filepath.Join(novaDir, "story-tellers")
	if err := os.MkdirAll(tellerDir, 0o755); err != nil {
		t.Fatal(err)
	}
	legacyTeller := `{
  "version": 3,
  "id": "legacy",
  "name": "旧字段导演",
  "description": "包含旧字数字段",
  "random_event_rate": 0.15,
  "reply_target_chars": 50,
  "tags": ["测试"],
  "context_policy": {
    "creator": "always",
    "lore": "relevant",
    "runtime_state": "always"
  },
  "slots": [
    {
      "id": "identity",
      "name": "系统提示",
      "target": "system",
      "enabled": true,
      "content": "旧字段导演系统规则"
    },
    {
      "id": "turn_context",
      "name": "本轮上下文",
      "target": "turn_context",
      "enabled": true,
      "content": "旧字段导演本轮规则"
    }
  ]
}`
	if err := os.WriteFile(filepath.Join(tellerDir, "legacy.json"), []byte(legacyTeller), 0o644); err != nil {
		t.Fatal(err)
	}

	store := interactive.NewStore(workspace)
	story, err := store.CreateStory(interactive.CreateStoryRequest{
		Title:            "旧字段测试",
		StoryTellerID:    "legacy",
		ReplyTargetChars: 700,
	})
	if err != nil {
		t.Fatal(err)
	}

	conversation := NewConversation(store, novaDir, workspace, story.ID, "", "我观察四周", story.ReplyTargetChars, nil)
	history, err := assembleAndCommitInteractiveContextForTest(conversation, "我观察四周", "我观察四周")
	if err != nil {
		t.Fatal(err)
	}
	if len(history) < 1 || !strings.Contains(history[len(history)-1].Content, "700 Chinese characters") {
		t.Fatalf("story reply target chars should be used: %#v", history)
	}
	if !strings.Contains(history[len(history)-1].Content, "Highest length constraint") {
		t.Fatalf("story reply target chars should be marked as highest priority: %#v", history[len(history)-1])
	}
	if strings.Contains(history[len(history)-1].Content, "50 Chinese characters") {
		t.Fatalf("legacy teller reply target chars should be ignored: %#v", history[len(history)-1])
	}
}

func TestInteractiveConversationKeepsFullHistoryWithoutSlidingWindow(t *testing.T) {
	workspace := t.TempDir()
	novaDir := t.TempDir()
	store := interactive.NewStore(workspace)
	story, err := store.CreateStory(interactive.CreateStoryRequest{
		Title:            "窗口测试",
		Origin:           "主角进入旧城",
		StoryTellerID:    "classic",
		ReplyTargetChars: 700,
	})
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 4; i++ {
		if _, err := store.AppendTurn(story.ID, interactive.AppendTurnRequest{
			User:      "第" + string(rune('0'+i)) + "次行动",
			Narrative: "第" + string(rune('0'+i)) + "段剧情",
		}); err != nil {
			t.Fatal(err)
		}
	}
	cfg := &config.Config{}
	conversation := NewConversation(store, novaDir, workspace, story.ID, "", "我继续探索", story.ReplyTargetChars, cfg)
	history, err := assembleAndCommitInteractiveContextForTest(conversation, "我继续探索", "我继续探索")
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 9 {
		t.Fatalf("history length = %d, want all 4 turns + instruction", len(history))
	}
	if history[0].Content != "第1次行动" || history[2].Content != "第2次行动" || history[6].Content != "第4次行动" {
		t.Fatalf("interactive story history should keep the full pre-compaction chain: %#v", history)
	}
	if strings.Contains(history[8].Content, "[历史上下文检查点]") || strings.Contains(history[8].Content, "第1次行动") {
		t.Fatalf("turn instruction should not carry sliding-window summaries or duplicate raw history: %s", history[8].Content)
	}
}

func TestInteractiveConversationUsesDefaultCompactionRetainedTurns(t *testing.T) {
	workspace := t.TempDir()
	novaDir := t.TempDir()
	store := interactive.NewStore(workspace)
	story, err := store.CreateStory(interactive.CreateStoryRequest{
		Title:            "压缩窗口测试",
		Origin:           "主角进入旧城",
		StoryTellerID:    "classic",
		ReplyTargetChars: 700,
	})
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 10; i++ {
		if _, err := store.AppendTurn(story.ID, interactive.AppendTurnRequest{
			User:      fmt.Sprintf("第%d次行动", i),
			Narrative: fmt.Sprintf("第%d段剧情", i),
		}); err != nil {
			t.Fatal(err)
		}
	}
	cfg := &config.Config{}
	conversation := NewConversation(store, novaDir, workspace, story.ID, "", "我继续探索", story.ReplyTargetChars, cfg)
	projection, err := conversation.PrepareAgentCompaction(context.Background(), agent.CompactionCompactRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if err := conversation.BindAgentCompaction(&agent.CompactionState{
		ID: "agent-compaction-default-tail", Revision: 1,
		Summary: "压缩摘要：主角已进入旧城。", ContextData: projection.ContextData,
	}); err != nil {
		t.Fatal(err)
	}
	history, err := assembleAndCommitInteractiveContextForTest(conversation, "我继续探索", "我继续探索")
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 4 {
		t.Fatalf("history length = %d, want compaction summary + 1 retained turn + instruction", len(history))
	}
	if history[1].Content != "第10次行动" || history[2].Content != "第10段剧情" {
		t.Fatalf("history should use default retained tail after compaction: %#v", history)
	}
}

func TestInteractiveTurnMemoryKeepsFullTurnChain(t *testing.T) {
	turns := []interactive.TurnEvent{
		{User: "第1次行动", Narrative: "第1段剧情"},
		{User: "第2次行动", Narrative: "第2段剧情"},
		{User: "第3次行动", Narrative: "第3段剧情"},
		{User: "第4次行动", Narrative: "第4段剧情"},
		{User: "第5次行动", Narrative: "第5段剧情"},
	}
	memory := buildInteractiveTurnHistory(turns)
	if len(memory.Turns) != len(turns) {
		t.Fatalf("turns = %d, want full chain %d", len(memory.Turns), len(turns))
	}
	if memory.Turns[0].User != "第1次行动" || memory.Turns[4].User != "第5次行动" {
		t.Fatalf("unexpected full turn chain: %#v", memory.Turns)
	}
	if memory.PreviousSummary != "" || memory.PreviousCount != 0 || memory.OmittedCount != 0 {
		t.Fatalf("sliding-window summary should be disabled: %#v", memory)
	}
}

func TestInteractiveTurnHistoryWithCompactionUsesSingleCheckpointAndRetainedTail(t *testing.T) {
	turns := []interactive.TurnEvent{
		{User: "第1次行动", Narrative: "第1段剧情"},
		{User: "第2次行动", Narrative: "第2段剧情"},
		{User: "第3次行动", Narrative: "第3段剧情"},
		{User: "第4次行动", Narrative: "第4段剧情"},
		{User: "第5次行动", Narrative: "第5段剧情"},
	}
	compaction := &interactive.ContextCompactionProjection{
		CompactionCheckpoint: agentcompaction.NewCheckpoint("", agentcompaction.Result{Summary: "压缩摘要：主角已进入旧城。"}),
		SourceTurnCount:      3,
	}
	history := buildInteractiveTurnHistoryWithCompaction(turns, compaction, 1)
	if history.PreviousSummary != "" {
		t.Fatalf("previous summary should stay empty when the history checkpoint is a model message, got %q", history.PreviousSummary)
	}
	if len(history.Turns) != 3 ||
		history.Turns[0].User != "第3次行动" ||
		history.Turns[1].User != "第4次行动" ||
		history.Turns[2].User != "第5次行动" {
		t.Fatalf("retained tail should keep retained source turns plus post-compaction turns: %#v", history.Turns)
	}
	if history.PreviousCount != 3 || history.OmittedCount != 3 {
		t.Fatalf("unexpected compaction counts: %#v", history)
	}
}

func TestInteractiveTurnHistoryWithCompactionRetainsSourceTailImmediatelyAfterCompaction(t *testing.T) {
	turns := []interactive.TurnEvent{
		{User: "第1次行动", Narrative: "第1段剧情"},
		{User: "第2次行动", Narrative: "第2段剧情"},
		{User: "第3次行动", Narrative: "第3段剧情"},
	}
	compaction := &interactive.ContextCompactionProjection{
		CompactionCheckpoint: agentcompaction.NewCheckpoint("", agentcompaction.Result{Summary: "压缩摘要：主角已进入旧城。"}),
		SourceTurnCount:      len(turns),
	}
	history := buildInteractiveTurnHistoryWithCompaction(turns, compaction, 2)
	if history.PreviousSummary != "" {
		t.Fatalf("history checkpoint should not be duplicated in previous summary: %q", history.PreviousSummary)
	}
	if len(history.Turns) != 2 || history.Turns[0].User != "第2次行动" || history.Turns[1].User != "第3次行动" {
		t.Fatalf("retained tail should remain available immediately after compaction: %#v", history.Turns)
	}
}

func TestInteractiveCompactionSourceUsesOnlyTurnsAfterPreviousCompaction(t *testing.T) {
	turns := []interactive.TurnEvent{
		{ID: "turn-1", BranchID: "main", User: "已压缩行动1", Narrative: "已压缩剧情1"},
		{ID: "turn-2", BranchID: "main", User: "已压缩行动2", Narrative: "已压缩剧情2"},
		{ID: "turn-3", BranchID: "main", User: "新增行动3", Narrative: "新增剧情3"},
	}
	compaction := &interactive.ContextCompactionProjection{
		CompactionCheckpoint: agentcompaction.NewCheckpoint("", agentcompaction.Result{Summary: "旧压缩摘要：前两回合已整理。"}),
		SourceTurnCount:      2,
	}
	source, checkpoint := interactiveCompactionSource(turns, compaction)
	if checkpoint != compaction.Summary {
		t.Fatalf("existing checkpoint = %q", checkpoint)
	}
	if len(source) != 2 {
		t.Fatalf("source len = %d, want user+narrative for one new turn: %#v", len(source), source)
	}
	if !strings.Contains(source[0].Content, "[source turn_id=turn-3 branch_id=main]") || !strings.HasSuffix(source[0].Content, "新增行动3") ||
		!strings.Contains(source[1].Content, "[source turn_id=turn-3 branch_id=main]") || !strings.HasSuffix(source[1].Content, "新增剧情3") {
		t.Fatalf("source should contain only new turn messages: %#v", source)
	}
	for _, msg := range source {
		if strings.Contains(msg.Content, "已压缩") {
			t.Fatalf("source should not repeat previously compacted turns: %#v", source)
		}
	}
}

func TestParseInteractiveAssistantOutput(t *testing.T) {
	narrative, err := ParseAssistantOutput("门后传来低沉的风声。")
	if err != nil {
		t.Fatal(err)
	}
	if narrative != "门后传来低沉的风声。" {
		t.Fatalf("unexpected parsed bare output narrative=%q", narrative)
	}

	// 思考前言 + 裸正文。
	narrative, err = ParseAssistantOutput("思考中...</think>\n真正的正文。")
	if err != nil || narrative != "真正的正文。" {
		t.Fatalf("expected orphan </think> without narrative stripped, narrative=%q err=%v", narrative, err)
	}

	_, err = ParseAssistantOutput("")
	if err == nil {
		t.Fatalf("expected empty narrative error")
	}
}
