package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cloudwego/eino/schema"

	"denova/config"
	"denova/internal/book"
	"denova/internal/interactive"
	"denova/internal/session"
)

func TestInteractiveConversationBuildsHistoryAndPersistsAssistantToStory(t *testing.T) {
	workspace := t.TempDir()
	loreStore := book.NewLoreStore(workspace)
	if _, err := loreStore.Create(book.LoreItemInput{ID: "hero", Type: "character", Name: "林川", Importance: "major", LoadMode: book.LoreLoadModeResident, Content: "林川：谨慎的幸存者"}); err != nil {
		t.Fatal(err)
	}
	if _, err := loreStore.Create(book.LoreItemInput{ID: "world", Type: "world", Name: "黄昏末日", Importance: "major", LoadMode: book.LoreLoadModeResident, Content: "世界已进入黄昏末日。"}); err != nil {
		t.Fatal(err)
	}
	if _, err := loreStore.Create(book.LoreItemInput{ID: "base", Type: "location", Name: "黄泉酒馆", Importance: "important", LoadMode: book.LoreLoadModeAuto, BriefDescription: "黄泉酒馆据点索引", Content: "黄泉酒馆完整设定：柜台后的影子不能离开酒馆。"}); err != nil {
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

	conversation := newInteractiveConversation(store, novaDir, workspace, story.ID, "", "我在黄泉酒馆点燃火把", story.ReplyTargetChars, nil)
	history, err := conversation.PrepareMessages("我在黄泉酒馆点燃火把", "我在黄泉酒馆点燃火把")
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 4 {
		t.Fatalf("history length = %d, want 4", len(history))
	}
	if history[0].Role != schema.User || !strings.Contains(history[0].Content, "常驻资料库") || !strings.Contains(history[0].Content, "林川：谨慎的幸存者") || !strings.Contains(history[0].Content, "世界已进入黄昏末日") {
		t.Fatalf("history[0] should be stable resident lore: %#v", history[0])
	}
	if history[1].Role != schema.User || history[1].Content != "我推开酒馆的门" {
		t.Fatalf("history[0] mismatch: %#v", history[0])
	}
	if strings.Contains(history[1].Content, "历史 checkpoint") || strings.Contains(history[1].Content, "最高篇幅约束") {
		t.Fatalf("history[1] should remain plain story history, got: %#v", history[1])
	}
	if history[2].Role != schema.Assistant || history[2].Content != "门后传来低沉的风声。" {
		t.Fatalf("history[2] mismatch: %#v", history[2])
	}
	if history[3].Role != schema.User || !strings.Contains(history[3].Content, "我在黄泉酒馆点燃火把") {
		t.Fatalf("history[3] mismatch: %#v", history[3])
	}
	for _, want := range []string{
		"导演本轮上下文规则",
		"[本轮动态上下文]",
		"800 个中文字",
		"最高篇幅约束",
		"list_lore_items",
		"search_story_history",
		"turn_id",
		"正文 Agent 简报",
		"source: agent-brief.md",
		"bounded",
		"# Actor 状态手册",
		"Actor ID：`protagonist`",
		"字段说明：",
		"更新指引：",
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
		"互动故事",
		"故事标题",
		"末日开端",
		"开端",
		"主角醒来发现世界已末日",
		"导演注入规则",
		"本轮上下文",
		"DirectorPlan",
		"正文 Agent 简报",
	} {
		if !strings.Contains(sources, want) {
			t.Fatalf("context sources should include %q: %s", want, sources)
		}
	}
	ledgerParts := conversation.ContextLedgerParts()
	var sawResidentLore, sawActiveLore, sawCurrentAction bool
	for _, part := range ledgerParts {
		if part.Source == "ResidentLore" && part.Bytes > 0 && part.Limit > book.ResidentLoreSafetyMaxBytes && part.LimitUnit == "bytes" && strings.Contains(part.Note, "complete=true") && strings.Contains(part.Note, "revision=") && strings.Contains(part.Note, "exact_final_message=true") {
			sawResidentLore = true
		}
		if part.Source == "LoreContext" && part.Title == "当前分支活动资料工作集" && part.Bytes > 0 {
			sawActiveLore = true
		}
		if part.Source == "本轮行动" && part.Title == "当前用户行动" && part.Bytes > 0 {
			sawCurrentAction = true
		}
	}
	if !sawResidentLore || !sawActiveLore || !sawCurrentAction {
		t.Fatalf("durable context fragments should distinguish resident lore, active lore, and current action metadata: %#v", ledgerParts)
	}

	submitTestTurnResult(t, conversation, "点燃火把", "照亮酒馆墙面")
	if err := conversation.AppendAssistantWithThinking("火光照亮了墙上的新线索。", "先判断现场风险。"); err != nil {
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
		Ops: []interactive.StateOp{
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
	last = snapshot.Turns[1]
	directorInstruction, err := conversation.BuildDirectorInstruction(last)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"keep、patch 或 replan",
		"不得改写历史 Turn 或 Actor State",
		"资料库优先",
		"不负责替用户选择下一步行动",
	} {
		if !strings.Contains(directorInstruction, want) {
			t.Fatalf("director instruction should include maintenance guidance %q: %s", want, directorInstruction)
		}
	}
	if !strings.Contains(directorInstruction, "黄泉酒馆") || !strings.Contains(directorInstruction, "list_lore_items") {
		t.Fatalf("director instruction should include the bounded lore-name roster and retrieval workflow: %s", directorInstruction)
	}
	if strings.Contains(directorInstruction, "黄泉酒馆据点索引") || strings.Contains(directorInstruction, "黄泉酒馆完整设定") {
		t.Fatalf("director lore-name roster should not preload briefs or bodies: %s", directorInstruction)
	}
	for _, want := range []string{
		"近期剧情历史",
		"本回合 TurnResult / RuleResolution / StateDelta 审计 JSON",
		"turn_result",
		"我在黄泉酒馆点燃火把",
		"状态系统 Schema",
		"当前状态系统快照",
		"director.md",
	} {
		if !strings.Contains(directorInstruction, want) {
			t.Fatalf("director instruction should include maintenance context %q: %s", want, directorInstruction)
		}
	}
	if strings.Contains(directorInstruction, "经典叙事者") || strings.Contains(directorInstruction, "导演本轮上下文规则") {
		t.Fatalf("director instruction should not include story-only teller rules: %s", directorInstruction)
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
	if err := conversation.AppendAssistant("柜台后的影子露出一道能通往地窖的缝。"); err != nil {
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
		Ops: []interactive.StateOp{
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
	conversation := newInteractiveConversation(store, t.TempDir(), workspace, story.ID, "main", "继续前进", story.ReplyTargetChars, nil)
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

func TestInteractiveConversationInjectsStoryDirectorStrategyPrompt(t *testing.T) {
	workspace := t.TempDir()
	novaDir := t.TempDir()
	prompt := "- 避免连续两回合使用同类型突发事件。\n- 伏笔回收前至少给一次可感知征兆。"
	director, err := interactive.NewStoryDirectorLibrary(novaDir).Create(interactive.StoryDirector{
		ID:          "custom-strategy",
		Name:        "自定义策略导演",
		Description: "测试 Markdown 策略提示注入",
		Strategy: interactive.StoryDirectorStrategy{
			Enabled:        true,
			PromptMarkdown: prompt,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	store := interactive.NewStore(workspace)
	story, err := store.CreateStory(interactive.CreateStoryRequest{
		Title:            "策略测试",
		Origin:           "主角进入旧城",
		StoryTellerID:    "classic",
		StoryDirectorID:  director.ID,
		ReplyTargetChars: 800,
	})
	if err != nil {
		t.Fatal(err)
	}
	turn, err := store.AppendTurn(story.ID, interactive.AppendTurnRequest{
		User:      "我观察街角",
		Narrative: "街角的灯忽明忽暗。",
	})
	if err != nil {
		t.Fatal(err)
	}

	conversation := newInteractiveConversation(store, novaDir, workspace, story.ID, "", "我跟上灯影", story.ReplyTargetChars, nil)
	history, err := conversation.PrepareMessages("我跟上灯影", "我跟上灯影")
	if err != nil {
		t.Fatal(err)
	}
	turnInstruction := history[len(history)-1].Content
	for _, want := range []string{"故事导演 Markdown 策略提示", "source: StoryDirector.strategy.prompt_markdown", "bounded", "避免连续两回合", "伏笔回收前"} {
		if !strings.Contains(turnInstruction, want) {
			t.Fatalf("interactive turn instruction should include strategy prompt %q:\n%s", want, turnInstruction)
		}
	}
	sources := conversation.ContextSourceSummary()
	for _, want := range []string{"StoryDirector.strategy.prompt_markdown", "故事导演 Markdown 策略提示", "bounded"} {
		if !strings.Contains(sources, want) {
			t.Fatalf("context sources should include strategy prompt %q:\n%s", want, sources)
		}
	}
	directorInstruction, err := conversation.BuildDirectorInstruction(turn)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"故事导演 Markdown 策略提示", "source: StoryDirector.strategy.prompt_markdown", "bounded", "避免连续两回合", "伏笔回收前"} {
		if !strings.Contains(directorInstruction, want) {
			t.Fatalf("director instruction should include strategy prompt %q:\n%s", want, directorInstruction)
		}
	}
}

func TestInteractiveConversationKeepsEventCardsForDirectorOnly(t *testing.T) {
	workspace := t.TempDir()
	novaDir := t.TempDir()
	eventPackage, err := interactive.NewEventPackageLibrary(novaDir).Create(interactive.EventPackageModule{
		ID:   "academy-pack",
		Name: "学院事件包",
		Events: []interactive.TellerEventCard{{
			ID:                  "academy_trial",
			TypeName:            "外门考核打脸",
			DescriptionMarkdown: "## 触发场景\n外门考核中同门当众质疑主角。\n\n## 事件回收 / 后果\n以后续榜单与戒律回收。",
			Enabled:             true,
			Category:            "学院",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	director, err := interactive.NewStoryDirectorLibrary(novaDir).Create(interactive.StoryDirector{
		ID:          "event-card-director",
		Name:        "事件卡导演",
		Description: "测试事件系统只进入后台导演",
		ModuleRefs:  interactive.StoryDirectorModuleRefs{EventPackageIDs: []string{eventPackage.ID}},
		Strategy: interactive.StoryDirectorStrategy{
			Enabled: true,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	store := interactive.NewStore(workspace)
	story, err := store.CreateStory(interactive.CreateStoryRequest{
		Title:           "事件卡上下文",
		Origin:          "主角进入外门",
		StoryTellerID:   "classic",
		StoryDirectorID: director.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := store.DirectorPlan(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	docs := plan.Docs
	docs.AgentBrief = strings.Replace(docs.AgentBrief, "说明当前场景、主角处境、直接目标，以及可观察、对话、调查、冒险、交易或保守应对的空间。", "公开压力升高，同门质疑逼近；玩家可以反证、迂回或调查。", 1)
	if _, err := store.UpdateDirectorPlan(story.ID, interactive.UpdateDirectorPlanRequest{BranchID: "main", Docs: docs, BaseRevision: plan.Metadata.Revision}); err != nil {
		t.Fatal(err)
	}
	turn, err := store.AppendTurn(story.ID, interactive.AppendTurnRequest{
		User:      "我走进演武场",
		Narrative: "演武场上的人声停了一瞬。",
	})
	if err != nil {
		t.Fatal(err)
	}

	conversation := newInteractiveConversation(store, novaDir, workspace, story.ID, "", "我看向质疑我的同门", story.ReplyTargetChars, nil)
	history, err := conversation.PrepareMessages("我看向质疑我的同门", "我看向质疑我的同门")
	if err != nil {
		t.Fatal(err)
	}
	turnInstruction := history[len(history)-1].Content
	for _, want := range []string{"正文 Agent 简报", "公开压力升高", "同门质疑"} {
		if !strings.Contains(turnInstruction, want) {
			t.Fatalf("interactive turn instruction should include translated director plan %q:\n%s", want, turnInstruction)
		}
	}
	for _, forbidden := range []string{"外门考核打脸", "触发场景", "事件回收 / 后果", "事件卡:"} {
		if strings.Contains(turnInstruction, forbidden) {
			t.Fatalf("interactive turn instruction should not include raw event card %q:\n%s", forbidden, turnInstruction)
		}
	}

	directorInstruction, err := conversation.BuildDirectorInstruction(turn)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"本轮事件机会", "cadence_not_due", "事件运行态"} {
		if !strings.Contains(directorInstruction, want) {
			t.Fatalf("director instruction should include deterministic event context %q:\n%s", want, directorInstruction)
		}
	}
	for _, forbidden := range []string{"外门考核打脸", "事件回收 / 后果", "可选事件卡紧凑索引"} {
		if strings.Contains(directorInstruction, forbidden) {
			t.Fatalf("director instruction should not inject event cards before an opportunity is due %q:\n%s", forbidden, directorInstruction)
		}
	}
}

func TestInteractiveDirectorEventCatalogIncludesConfiguredEventCards(t *testing.T) {
	director := interactive.StoryDirector{
		ID:         "catalog",
		Name:       "事件目录",
		ModuleRefs: interactive.StoryDirectorModuleRefs{EventPackageIDs: []string{"academy-pack"}},
		EventPackages: []interactive.TellerEventPackage{{
			ID:      "academy-pack",
			Enabled: true,
			Events: []interactive.TellerEventCard{{
				ID:                  "academy_trial",
				TypeName:            "外门考核打脸",
				DescriptionMarkdown: "## 触发场景\n外门考核中同门当众质疑主角。\n\n## 事件回收 / 后果\n以后续榜单与戒律回收。",
				Enabled:             true,
				Category:            "学院",
			}},
		}},
	}
	catalog := interactiveDirectorEventCatalog(director)
	found := false
	for _, event := range catalog {
		if event.ID == "academy-pack/academy_trial" {
			found = true
			if !strings.Contains(event.Template, "外门考核") || event.Category != "学院" {
				t.Fatalf("event card catalog entry mismatch: %#v", event)
			}
		}
	}
	if !found {
		t.Fatalf("director catalog should include event card: %#v", catalog)
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
	conversation := newInteractiveConversation(store, novaDir, workspace, story.ID, "main", "我强闯秘境入口", story.ReplyTargetChars, &config.Config{})
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
	submitTestTurnResult(t, conversation, "闯入秘境", "裁定入口禁制")
	if err := conversation.AppendAssistant("秘境入口的白光猛然坍缩，主角被禁制震回台阶。"); err != nil {
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
		Strategy: interactive.StoryDirectorStrategy{Enabled: true},
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
	conversation := newInteractiveConversation(store, t.TempDir(), workspace, story.ID, "main", "检查档案柜", 800, &config.Config{})

	if err := conversation.AppendDisplayEvent(session.DisplayEvent{Role: "thinking", Content: "先分析档案室线索。"}); err != nil {
		t.Fatal(err)
	}
	if err := conversation.AppendDisplayEvent(session.DisplayEvent{ID: "call-1", Role: "tool_call", Name: "list_lore_items", Content: "list_lore_items", Status: "running"}); err != nil {
		t.Fatal(err)
	}
	if err := conversation.AppendDisplayToolArgs("call-1", "list_lore_items", `{"keywords":["档案室"]}`); err != nil {
		t.Fatal(err)
	}
	if err := conversation.UpdateDisplayToolResult("call-1", "list_lore_items", "success", "找到档案室设定"); err != nil {
		t.Fatal(err)
	}
	if err := conversation.AppendDisplayEvent(session.DisplayEvent{Role: "thinking", Content: "第二轮基于工具结果继续判断。"}); err != nil {
		t.Fatal(err)
	}
	if err := conversation.AppendDisplayEvent(session.DisplayEvent{ID: "call-2", Role: "tool_call", Name: "search_story_history", Content: "search_story_history", Args: `{"keywords":["钟楼"]}`, Status: "running"}); err != nil {
		t.Fatal(err)
	}
	if err := conversation.UpdateDisplayToolResult("call-2", "search_story_history", "success", "找到 1 个历史回合"); err != nil {
		t.Fatal(err)
	}
	submitTestTurnResult(t, conversation, "调查档案柜", "找到档案室线索")
	if err := conversation.AppendAssistantWithThinking("档案柜里露出一张潮湿的地图。", "先分析档案室线索。第二轮基于工具结果继续判断。"); err != nil {
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

	conversation := newInteractiveConversation(store, novaDir, workspace, story.ID, "", "我观察四周", story.ReplyTargetChars, nil)
	history, err := conversation.PrepareMessages("我观察四周", "我观察四周")
	if err != nil {
		t.Fatal(err)
	}
	if len(history) < 1 || !strings.Contains(history[len(history)-1].Content, "700 个中文字") {
		t.Fatalf("story reply target chars should be used: %#v", history)
	}
	if !strings.Contains(history[len(history)-1].Content, "最高篇幅约束") {
		t.Fatalf("story reply target chars should be marked as highest priority: %#v", history[len(history)-1])
	}
	if strings.Contains(history[len(history)-1].Content, "50 个中文字") {
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
	conversation := newInteractiveConversation(store, novaDir, workspace, story.ID, "", "我继续探索", story.ReplyTargetChars, cfg)
	history, err := conversation.PrepareMessages("我继续探索", "我继续探索")
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
	if _, err := store.AppendContextCompaction(story.ID, "main", interactive.ContextCompactionEvent{
		AgentKind:       config.AgentKindInteractiveStory,
		Summary:         "压缩摘要：主角已进入旧城。",
		SourceTurnCount: 10,
	}); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{}
	conversation := newInteractiveConversation(store, novaDir, workspace, story.ID, "", "我继续探索", story.ReplyTargetChars, cfg)
	history, err := conversation.PrepareMessages("我继续探索", "我继续探索")
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

func TestInteractiveDirectorInstructionUsesModelVisibleCompactedHistory(t *testing.T) {
	workspace := t.TempDir()
	novaDir := t.TempDir()
	store := interactive.NewStore(workspace)
	story, err := store.CreateStory(interactive.CreateStoryRequest{
		Title:            "记忆压缩测试",
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
	if _, err := store.AppendContextCompaction(story.ID, "main", interactive.ContextCompactionEvent{
		AgentKind:       config.AgentKindInteractiveStory,
		Summary:         "压缩摘要：主角已进入旧城。",
		SourceTurnCount: 10,
	}); err != nil {
		t.Fatal(err)
	}

	conversation := newInteractiveConversation(store, novaDir, workspace, story.ID, "", "我继续探索", story.ReplyTargetChars, &config.Config{})
	instruction, err := conversation.BuildDirectorInstruction(interactive.TurnEvent{User: "我继续探索", Narrative: "我发现新的石门"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(instruction, "[Denova Context Compaction]") || !strings.Contains(instruction, "压缩摘要：主角已进入旧城。") {
		t.Fatalf("director instruction should include active compaction summary: %s", instruction)
	}
	if strings.Contains(instruction, "第1次行动") || strings.Contains(instruction, "第9次行动") {
		t.Fatalf("director instruction should not include turns omitted by compaction: %s", instruction)
	}
	if !strings.Contains(instruction, "第10次行动") {
		t.Fatalf("director instruction should include retained model-visible tail: %s", instruction)
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
	compaction := &interactive.ContextCompactionEvent{
		Summary:         "压缩摘要：主角已进入旧城。",
		SourceTurnCount: 3,
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
	compaction := &interactive.ContextCompactionEvent{
		Summary:         "压缩摘要：主角已进入旧城。",
		SourceTurnCount: len(turns),
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
	compaction := &interactive.ContextCompactionEvent{
		Summary:         "旧压缩摘要：前两回合已整理。",
		SourceTurnCount: 2,
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
	narrative, err := parseInteractiveAssistantOutput("门后传来低沉的风声。")
	if err != nil {
		t.Fatal(err)
	}
	if narrative != "门后传来低沉的风声。" {
		t.Fatalf("unexpected parsed bare output narrative=%q", narrative)
	}

	// 思考前言 + 裸正文。
	narrative, err = parseInteractiveAssistantOutput("思考中...</think>\n真正的正文。")
	if err != nil || narrative != "真正的正文。" {
		t.Fatalf("expected orphan </think> without narrative stripped, narrative=%q err=%v", narrative, err)
	}

	_, err = parseInteractiveAssistantOutput("")
	if err == nil {
		t.Fatalf("expected empty narrative error")
	}
}

func submitTestTurnResult(t *testing.T, conversation *interactiveConversation, intent, goal string) {
	t.Helper()
	updates := []interactive.StateUpdate{}
	storyContext, err := conversation.store.StoryContext(conversation.storyID, conversation.branchID)
	if err != nil {
		t.Fatal(err)
	}
	actors, _ := storyContext.Snapshot.State["actors"].(map[string]any)
	_, hasStoryContext := actors[interactive.DefaultStoryContextActorID]
	actorState := conversation.storyDirectorForMeta(storyContext.Meta).ActorState
	if storyContext.Meta.ActorStateSchema != nil {
		actorState = storyContext.Meta.ActorStateSchema.System
	}
	if !hasStoryContext {
		for _, actor := range actorState.InitialActors {
			if actor.ID == interactive.DefaultStoryContextActorID && actor.TemplateID == interactive.ActorStateStoryContextTemplateID {
				hasStoryContext = true
				break
			}
		}
	}
	if hasStoryContext {
		event := strings.TrimSpace(goal)
		if event == "" {
			event = strings.TrimSpace(intent)
		}
		updates = append(updates,
			interactive.StateUpdate{Op: "replace", Path: "/story/当前详细地点", Value: "测试场景"},
			interactive.StateUpdate{Op: "replace", Path: "/story/当前事件", Value: event},
		)
	}
	input := testTurnSubmissionInput(updates, true)
	receipt, err := conversation.SubmitTurnResult(context.Background(), input)
	if err != nil || !receipt.Ready {
		t.Fatalf("SubmitTurnResult failed: receipt=%#v err=%v", receipt, err)
	}
}
