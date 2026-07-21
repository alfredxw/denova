package app

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"denova/config"
	"denova/internal/agent"
	"denova/internal/book"
	"denova/internal/interactive"
)

func TestInteractiveDirectorTaskCompletesPlanMetadataAfterFileUpdate(t *testing.T) {
	workspace := t.TempDir()
	store := interactive.NewStore(workspace)
	story, err := store.CreateStory(interactive.CreateStoryRequest{
		Title:         "外门逆袭",
		Origin:        "主角被同门轻视",
		StoryTellerID: "classic",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := book.NewLoreStore(workspace).Create(book.LoreItemInput{
		ID:               "shen-ning",
		Type:             "character",
		Name:             "沈凝",
		Importance:       "major",
		LoadMode:         book.LoreLoadModeAuto,
		BriefDescription: "角色 沈凝。外门比试的关键见证者，与主角关系存在转折空间。",
		Content:          "沈凝表面冷淡，实际在暗中调查外门资源分配不公。她不会无故帮助主角，但会被公开证据和胆识触动。",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := book.NewLoreStore(workspace).Create(book.LoreItemInput{
		ID: "contest-rule", Type: "rule", Name: "公开比试规则", LoadMode: book.LoreLoadModeResident, Content: "公开比试禁止场外偷袭。",
	}); err != nil {
		t.Fatal(err)
	}
	turn, _, err := store.AppendTurnWithState(story.ID, interactive.AppendTurnWithStateRequest{
		BranchID:  "main",
		User:      "我报名参加公开比试",
		Narrative: "登记弟子抬头看了他一眼，压低声音笑了。",
	})
	if err != nil {
		t.Fatal(err)
	}
	initialStatus, err := store.DirectorPlanStatus(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if initialStatus.Status != interactive.DirectorPlanStatusWaitingOpening || initialStatus.Blocking {
		t.Fatalf("first persisted turn should stay available while director planning is pending: %#v", initialStatus)
	}
	started := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(release) })
	directorGenerator := func(_ context.Context, _ *config.Config, _ *book.State, toolContext agent.InteractiveStoryToolContext, instruction string) (string, error) {
		close(started)
		<-release
		if !strings.Contains(toolContext.StableContextTitle, "complete=true") || !strings.Contains(toolContext.StableContext, "公开比试禁止场外偷袭") || toolContext.StableContextMaxBytes < len([]byte(toolContext.StableContext)) {
			t.Fatalf("director should receive complete resident Lore as a bounded stable prefix: %#v", toolContext)
		}
		if strings.Contains(instruction, "公开比试禁止场外偷袭") {
			t.Fatalf("dynamic Director instruction must not duplicate resident Lore bodies:\n%s", instruction)
		}
		if !strings.Contains(instruction, "director.md") || !strings.Contains(instruction, "agent-brief.md") || !strings.Contains(instruction, "lore-context.md") || strings.Contains(instruction, "mainline.md") {
			t.Fatalf("director should receive all three complete plan documents:\n%s", instruction)
		}
		if toolContext.DisplayConversation == nil {
			t.Fatalf("director should receive display conversation for background progress")
		}
		if !strings.Contains(instruction, "资料库导演上下文") || !strings.Contains(instruction, "沈凝") {
			t.Fatalf("director should receive bounded lore context:\n%s", instruction)
		}
		plan, err := toolContext.Store.DirectorPlan(toolContext.StoryID, toolContext.BranchID)
		if err != nil {
			return "", err
		}
		docs := plan.Docs
		docs.AgentBrief = strings.Replace(docs.AgentBrief, "说明当前场景、主角处境、直接目标，以及可观察、对话、调查、冒险、交易或保守应对的空间。", "公开比试制造质疑与反证机会。", 1)
		if err := submitDirectorPlanForTest(toolContext, interactive.PlanDecision{Mode: interactive.PlanDecisionPatch, Triggers: []string{"public_challenge"}, Reason: "导演安排公开反转"}, &docs); err != nil {
			return "", err
		}
		return "已完成导演规划。", nil
	}
	conversation := newInteractiveConversation(store, t.TempDir(), workspace, story.ID, "main", turn.User, story.ReplyTargetChars, &config.Config{})
	conversation.directorGenerator = directorGenerator
	done := startInteractiveDirectorTask(&config.Config{}, book.NewState(workspace), conversation, turn, nil)

	<-started
	runningStatus, err := store.DirectorPlanStatus(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if runningStatus.Blocking || runningStatus.StartReady || runningStatus.CompletedDocs != 0 || runningStatus.PlannedDocs != 3 {
		t.Fatalf("initial director run should expose non-blocking progress: %#v", runningStatus)
	}
	releaseOnce.Do(func() { close(release) })
	<-done
	snapshot, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.DirectorPlan == nil || snapshot.DirectorPlan.Metadata.LastRun == nil || snapshot.DirectorPlan.Metadata.LastRun.Summary != "导演安排公开反转" {
		t.Fatalf("director summary was not persisted: %#v", snapshot.DirectorPlan)
	}
	if snapshot.CurrentTurn == nil || snapshot.CurrentTurn.ID != turn.ID {
		t.Fatalf("turn should remain current after director update: %#v", snapshot.CurrentTurn)
	}
	if snapshot.DirectorPlan == nil || !strings.Contains(snapshot.DirectorPlan.Docs.AgentBrief, "公开比试制造质疑") {
		t.Fatalf("agent brief should include file update: %#v", snapshot.DirectorPlan)
	}
	if snapshot.DirectorPlanStatus == nil || snapshot.DirectorPlanStatus.Status != interactive.DirectorPlanStatusReady || !snapshot.DirectorPlanStatus.StartReady || snapshot.DirectorPlanStatus.Blocking || snapshot.DirectorPlanStatus.CompletedDocs != 3 {
		t.Fatalf("completed director run should unblock the story start: %#v", snapshot.DirectorPlanStatus)
	}
}

func TestPrepareInteractiveDirectorBeforeOpeningBuildsLoreWorksetForFirstGameTurn(t *testing.T) {
	workspace := t.TempDir()
	store := interactive.NewStore(workspace)
	for _, input := range []book.LoreItemInput{
		{ID: "witness", Type: "character", Name: "沈凝", Importance: "major", LoadMode: book.LoreLoadModeAuto, Content: "沈凝不会无证据帮助任何人。"},
		{ID: "faction", Type: "faction", Name: "戒律堂", Importance: "important", LoadMode: book.LoreLoadModeAuto, Content: "戒律堂控制公开比试秩序。"},
	} {
		if _, err := book.NewLoreStore(workspace).Create(input); err != nil {
			t.Fatal(err)
		}
	}
	story, err := store.CreateStory(interactive.CreateStoryRequest{Title: "开局预规划", Origin: "主角报名公开比试"})
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Workspace: workspace}
	conversation := newInteractiveConversation(store, "", workspace, story.ID, "main", "我报名公开比试", story.ReplyTargetChars, cfg)
	generated := 0
	conversation.directorGenerator = func(_ context.Context, _ *config.Config, _ *book.State, toolContext agent.InteractiveStoryToolContext, instruction string) (string, error) {
		generated++
		if toolContext.MaintenanceTask != interactiveDirectorTaskOpeningPlan || toolContext.TurnID != interactiveDirectorOpeningSourceID {
			t.Fatalf("unexpected opening tool context: %#v", toolContext)
		}
		for _, want := range []string{"开局正文生成前", "资料名称目录", "沈凝", "戒律堂", "我报名公开比试"} {
			if !strings.Contains(instruction, want) {
				t.Fatalf("opening director instruction missing %q:\n%s", want, instruction)
			}
		}
		plan, err := toolContext.Store.DirectorPlan(toolContext.StoryID, toolContext.BranchID)
		if err != nil {
			return "", err
		}
		docs := plan.Docs
		docs.Plan = strings.Replace(docs.Plan, "维护当前阶段的隐藏真相、阶段高潮、反转条件与阅读钩子投放顺序，保证每个可玩回合都能产生有效推进。", "开局阶段围绕公开比试秩序、见证者立场与资源分配证据安排高潮和反转。", 1)
		docs.AgentBrief = strings.Replace(docs.AgentBrief, "说明当前场景、主角处境、直接目标，以及可观察、对话、调查、冒险、交易或保守应对的空间。", "公开比试开局围绕沈凝见证与戒律堂秩序展开。", 1)
		docs.LoreContext = strings.Replace(docs.LoreContext, "## 当前\n", "## 当前\n\n- [[沈凝]]：开局见证者\n", 1)
		if toolContext.OnLoreItemsRead != nil {
			toolContext.OnLoreItemsRead([]string{"witness"})
		}
		if err := submitDirectorPlanForTest(toolContext, interactive.PlanDecision{Mode: interactive.PlanDecisionReplan, Triggers: []string{"story_opening"}, Reason: "开局资料工作集已建立"}, &docs); err != nil {
			return "", err
		}
		return "开局资料工作集已建立。", nil
	}

	prepared, err := prepareInteractiveDirectorBeforeOpening(context.Background(), cfg, book.NewState(workspace), conversation, "我报名公开比试", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !prepared || generated != 1 {
		t.Fatalf("opening director should run exactly once: prepared=%v generated=%d", prepared, generated)
	}
	snapshot, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Turns) != 0 || snapshot.DirectorPlanStatus == nil || snapshot.DirectorPlanStatus.Status != interactive.DirectorPlanStatusReady || snapshot.DirectorPlanStatus.SourceTurnID != interactiveDirectorOpeningSourceID {
		t.Fatalf("opening director should finish before the first turn: %#v", snapshot.DirectorPlanStatus)
	}
	messages, err := conversation.PrepareMessages("", "我报名公开比试")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(messages[len(messages)-1].Content, "沈凝不会无证据帮助任何人") {
		t.Fatalf("first Game Agent turn should receive the prepared active lore:\n%s", messages[len(messages)-1].Content)
	}

	prepared, err = prepareInteractiveDirectorBeforeOpening(context.Background(), cfg, book.NewState(workspace), conversation, "我报名公开比试", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !prepared || generated != 1 {
		t.Fatalf("prepared opening should be reused without another model call: prepared=%v generated=%d", prepared, generated)
	}
}

func TestInteractiveDirectorTaskMarksFailureWithoutBlockingTurn(t *testing.T) {
	workspace := t.TempDir()
	store := interactive.NewStore(workspace)
	story, err := store.CreateStory(interactive.CreateStoryRequest{
		Title:         "失败落盘",
		Origin:        "主角探索秘境",
		StoryTellerID: "classic",
	})
	if err != nil {
		t.Fatal(err)
	}
	turn, _, err := store.AppendTurnWithState(story.ID, interactive.AppendTurnWithStateRequest{
		BranchID:  "main",
		User:      "我强行穿过禁制",
		Narrative: "禁制轰然亮起。",
	})
	if err != nil {
		t.Fatal(err)
	}
	directorGenerator := func(context.Context, *config.Config, *book.State, agent.InteractiveStoryToolContext, string) (string, error) {
		return "", errors.New("director unavailable")
	}

	conversation := newInteractiveConversation(store, t.TempDir(), workspace, story.ID, "main", turn.User, story.ReplyTargetChars, &config.Config{})
	conversation.directorGenerator = directorGenerator
	done := startInteractiveDirectorTask(&config.Config{}, book.NewState(workspace), conversation, turn, nil)
	<-done

	snapshot, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.CurrentTurn == nil || snapshot.CurrentTurn.ID != turn.ID {
		t.Fatalf("turn should remain current after director failure: %#v", snapshot.CurrentTurn)
	}
	if snapshot.DirectorPlan == nil || snapshot.DirectorPlan.Metadata.LastRun == nil || !strings.Contains(snapshot.DirectorPlan.Metadata.LastRun.Error, "director unavailable") {
		t.Fatalf("failure should be recorded: %#v", snapshot.DirectorPlan)
	}
	if snapshot.DirectorPlanStatus == nil || snapshot.DirectorPlanStatus.Status != interactive.DirectorPlanStatusFailed || snapshot.DirectorPlanStatus.Blocking || snapshot.DirectorPlanStatus.StartReady {
		t.Fatalf("initial director failure should be recorded without blocking retry: %#v", snapshot.DirectorPlanStatus)
	}

	conversation.directorGenerator = func(_ context.Context, _ *config.Config, _ *book.State, toolContext agent.InteractiveStoryToolContext, _ string) (string, error) {
		plan, err := toolContext.Store.DirectorPlan(toolContext.StoryID, toolContext.BranchID)
		if err != nil {
			return "", err
		}
		docs := plan.Docs
		docs.Plan += "\n\n失败后重试成功，准备继续推进。"
		if err := submitDirectorPlanForTest(toolContext, interactive.PlanDecision{Mode: interactive.PlanDecisionPatch, Reason: "失败后重试成功"}, &docs); err != nil {
			return "", err
		}
		return "失败后重试成功。", nil
	}
	done = startInteractiveDirectorTask(&config.Config{}, book.NewState(workspace), conversation, turn, nil)
	<-done
	retried, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if retried.DirectorPlanStatus == nil || retried.DirectorPlanStatus.Status != interactive.DirectorPlanStatusReady || !retried.DirectorPlanStatus.StartReady || retried.DirectorPlanStatus.Blocking {
		t.Fatalf("retry should mark initial director plan ready: %#v", retried.DirectorPlanStatus)
	}
}

func TestAnalyzeInteractiveDirectorContextUsesCurrentDirectorInputs(t *testing.T) {
	workspace := t.TempDir()
	novaDir := t.TempDir()
	store := interactive.NewStore(workspace)
	story, err := store.CreateStory(interactive.CreateStoryRequest{
		Title:         "外门逆袭",
		Origin:        "主角被同门轻视",
		StoryTellerID: "classic",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := book.NewLoreStore(workspace).Create(book.LoreItemInput{
		ID:               "shen-ning",
		Type:             "character",
		Name:             "沈凝",
		Importance:       "major",
		BriefDescription: "角色 沈凝。外门比试的关键见证者。",
		Content:          "沈凝是外门比试的关键见证者，会被公开证据触动。",
	}); err != nil {
		t.Fatal(err)
	}
	turn, _, err := store.AppendTurnWithState(story.ID, interactive.AppendTurnWithStateRequest{
		BranchID:  "main",
		User:      "我邀请沈凝旁观公开比试",
		Narrative: "沈凝停下脚步，示意我继续说。",
	})
	if err != nil {
		t.Fatal(err)
	}
	app := &App{
		cfg:         &config.Config{Workspace: workspace, NovaDir: novaDir},
		workspace:   workspace,
		bookState:   book.NewState(workspace),
		bookService: book.NewService(workspace),
		interactive: store,
	}

	analysis, err := app.AnalyzeInteractiveDirectorContext(story.ID, "main", turn.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	if analysis.AgentKind != config.AgentKindInteractiveDirector || analysis.Mode != "interactive_director" {
		t.Fatalf("unexpected director analysis identity: %#v", analysis)
	}
	var sawLore, sawTurnAudit, sawDirectorPlan bool
	for _, part := range analysis.ContextMessages {
		if strings.Contains(part.Source, "lore") && strings.Contains(part.Content, "沈凝") {
			sawLore = true
		}
		if part.Title == "本回合 TurnResult / RuleResolution / StateDelta 审计 JSON" && strings.Contains(part.Content, turn.ID) {
			sawTurnAudit = true
		}
		if (part.Title == "当前导演规划文档快照" && strings.Contains(part.Content, "agent-brief.md")) || strings.Contains(part.Title, "agent-brief.md") {
			sawDirectorPlan = true
		}
	}
	if !sawLore || !sawTurnAudit || !sawDirectorPlan {
		t.Fatalf("director context should include lore, turn audit, and director.md snapshot: lore=%v audit=%v plan=%v parts=%#v", sawLore, sawTurnAudit, sawDirectorPlan, analysis.ContextMessages)
	}
}

func TestShouldRunInteractiveDirectorAgentAllowsManualRunForLegacyOffMode(t *testing.T) {
	strategy := interactive.StoryDirectorStrategy{Enabled: true, DirectorAgentMode: interactive.DirectorAgentModeOff}
	if decision := shouldRunInteractiveDirectorAgent(strategy); !decision.ShouldRun {
		t.Fatalf("manual runs should remain available when automatic scheduling is off: %#v", decision)
	}
	strategy.Enabled = false
	if decision := shouldRunInteractiveDirectorAgent(strategy); decision.ShouldRun || decision.Reason != "disabled" {
		t.Fatalf("disabled Director must reject manual runs: %#v", decision)
	}
}

func submitDirectorPlanForTest(toolContext agent.InteractiveStoryToolContext, decision interactive.PlanDecision, docs *interactive.DirectorPlanDocs) error {
	if toolContext.SubmitDirectorPlanUpdate == nil {
		return errors.New("submit director plan callback missing")
	}
	if docs == nil {
		return errors.New("director plan docs missing")
	}
	baseline, err := toolContext.Store.DirectorPlan(toolContext.StoryID, toolContext.BranchID)
	if err != nil {
		return err
	}
	updates := make([]interactive.DirectorPlanDocumentUpdate, 0, 3)
	appendDocument := func(document, kind, before, after string) {
		if strings.TrimSpace(before) == strings.TrimSpace(after) {
			return
		}
		updates = append(updates, interactive.DirectorPlanDocumentUpdate{
			Document: document,
			BaseHash: baseline.Metadata.Docs[kind].Hash,
			Edits: []interactive.DirectorMarkdownEdit{{
				Op:      interactive.DirectorMarkdownEditReplaceDocument,
				Content: after,
			}},
		})
	}
	appendDocument(interactive.DirectorDocumentPlan, interactive.DirectorPlanDocPlan, baseline.Docs.Plan, docs.Plan)
	appendDocument(interactive.DirectorDocumentAgentBrief, interactive.DirectorPlanDocAgentBrief, baseline.Docs.AgentBrief, docs.AgentBrief)
	appendDocument(interactive.DirectorDocumentLoreContext, interactive.DirectorPlanDocLoreContext, baseline.Docs.LoreContext, docs.LoreContext)
	receipt, err := toolContext.SubmitDirectorPlanUpdate(context.Background(), interactive.DirectorPlanUpdateSubmission{Decision: decision, Updates: updates, Finalize: true})
	if err != nil {
		return err
	}
	if !receipt.Finalized {
		return errors.New("director plan test patch was not finalized")
	}
	return nil
}

func committedTurnResultForTest(playerIntent, sceneGoal, fact string) *interactive.TurnResult {
	_, _, _ = playerIntent, sceneGoal, fact
	return &interactive.TurnResult{
		StateUpdates: []interactive.StateUpdate{},
		Choices:      []string{"继续推进", "观察周围", "询问同伴", "检查状态", "暂时等待"},
	}
}
