package app

import (
	"context"
	"errors"
	"os"
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
		BriefDescription: "角色 沈凝。外门比试的关键见证者，与主角关系存在转折空间。上下文出现相关内容时，一定要参考本项详情。",
		Content:          "沈凝表面冷淡，实际在暗中调查外门资源分配不公。她不会无故帮助主角，但会被公开证据和胆识触动。",
	}); err != nil {
		t.Fatal(err)
	}
	turn, _, err := store.AppendTurnWithState(story.ID, interactive.AppendTurnWithStateRequest{
		BranchID:  "main",
		User:      "我报名参加公开比试",
		Narrative: "登记弟子抬头看了他一眼，压低声音笑了。",
		TurnBrief: &interactive.TurnBrief{
			UserAction:       "报名公开比试",
			TurnGoal:         "建立公开质疑",
			EventIntents:     []string{"face_slap"},
			StateExpectation: "公开比试即将开始",
		},
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
		if !strings.Contains(instruction, "director.md") || strings.Contains(instruction, "mainline.md") || len(toolContext.DirectorPlanAllowedPaths) != 1 {
			t.Fatalf("director should receive plan paths and guard context: paths=%#v\n%s", toolContext.DirectorPlanAllowedPaths, instruction)
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
		docs.Plan = strings.Replace(docs.Plan, "明确当前场景、主角处境、直接目标和可玩行动空间，让用户能观察、对话、调查、冒险、交易或保守应对。", "公开比试制造质疑与反证机会。", 1)
		if err := writeDirectorPlanDocsForTest(toolContext.DirectorPlanAllowedPaths, docs); err != nil {
			return "", err
		}
		return "导演安排公开反转", nil
	}
	conversation := newInteractiveConversation(store, t.TempDir(), workspace, story.ID, "main", turn.User, story.ReplyTargetChars, &config.Config{})
	conversation.directorGenerator = directorGenerator
	done := startInteractiveDirectorTask(&config.Config{}, book.NewState(workspace), conversation, turn, nil)

	<-started
	runningStatus, err := store.DirectorPlanStatus(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if runningStatus.Blocking || runningStatus.StartReady || runningStatus.CompletedDocs != 0 || runningStatus.PlannedDocs != 1 {
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
	if snapshot.DirectorPlan == nil || !strings.Contains(snapshot.DirectorPlan.Docs.Plan, "公开比试制造质疑") {
		t.Fatalf("director plan should include file update: %#v", snapshot.DirectorPlan)
	}
	if snapshot.DirectorPlanStatus == nil || snapshot.DirectorPlanStatus.Status != interactive.DirectorPlanStatusReady || !snapshot.DirectorPlanStatus.StartReady || snapshot.DirectorPlanStatus.Blocking || snapshot.DirectorPlanStatus.CompletedDocs != 1 {
		t.Fatalf("completed director run should unblock the story start: %#v", snapshot.DirectorPlanStatus)
	}
}

func TestInteractiveDirectorMaintenanceSeparatesMemoryAndDirectorPlan(t *testing.T) {
	workspace := t.TempDir()
	store := interactive.NewStore(workspace)
	story, err := store.CreateStory(interactive.CreateStoryRequest{
		Title:         "统一维护",
		Origin:        "主角进入旧城",
		StoryTellerID: "classic",
	})
	if err != nil {
		t.Fatal(err)
	}
	turn, _, err := store.AppendTurnWithState(story.ID, interactive.AppendTurnWithStateRequest{
		BranchID:  "main",
		User:      "我扶着林川避开钟楼射线",
		Narrative: "林川肩头被擦伤，但确认钟楼上有人盯梢。",
		TurnResult: committedTurnResultForTest(
			"扶着林川避开钟楼射线",
			"确认钟楼威胁",
			"林川确认钟楼上有人盯梢",
		),
	})
	if err != nil {
		t.Fatal(err)
	}
	directorGenerator := func(_ context.Context, _ *config.Config, _ *book.State, toolContext agent.InteractiveStoryToolContext, instruction string) (string, error) {
		switch toolContext.MaintenanceTask {
		case interactiveDirectorTaskMemoryUpdate:
			for _, want := range []string{"整理当前批次已提交回合的长期事实", "故事记忆结构与字段协议", "TurnResult / RuleResolution / StateDelta"} {
				if !strings.Contains(instruction, want) {
					t.Fatalf("memory instruction missing %q:\n%s", want, instruction)
				}
			}
			if len(toolContext.DirectorPlanAllowedPaths) != 0 {
				t.Fatalf("memory recorder should not receive director paths: %#v", toolContext.DirectorPlanAllowedPaths)
			}
			if err := applyDirectorMemoryForTest(toolContext); err != nil {
				return "", err
			}
			return "已整理故事记忆", nil
		case interactiveDirectorTaskDirectorPlanUpdate:
			for _, want := range []string{"director_plan_update", "keep、patch 或 replan", "状态系统 Schema", "director.md"} {
				if !strings.Contains(instruction, want) {
					t.Fatalf("director instruction missing %q:\n%s", want, instruction)
				}
			}
			plan, err := toolContext.Store.DirectorPlan(toolContext.StoryID, toolContext.BranchID)
			if err != nil {
				return "", err
			}
			docs := plan.Docs
			docs.Plan = strings.Replace(docs.Plan, "明确当前场景、主角处境、直接目标和可玩行动空间，让用户能观察、对话、调查、冒险、交易或保守应对。", "钟楼盯梢者成为下一轮可调查压力点。", 1)
			if err := writeDirectorPlanDocsForTest(toolContext.DirectorPlanAllowedPaths, docs); err != nil {
				return "", err
			}
			return `{"mode":"patch","triggers":["new_clue"],"reason":"钟楼威胁成为新的调查压力点"}`, nil
		default:
			return "", errors.New("unexpected maintenance task: " + toolContext.MaintenanceTask)
		}
	}
	conversation := newInteractiveConversation(store, t.TempDir(), workspace, story.ID, "main", turn.User, story.ReplyTargetChars, &config.Config{})
	conversation.directorGenerator = directorGenerator
	result, err := runInteractiveDirectorMaintenance(context.Background(), &config.Config{}, book.NewState(workspace), conversation, turn, nil, interactiveDirectorTaskTurnMaintenance)
	if err != nil {
		t.Fatal(err)
	}
	if result.AppliedStoryMemoryPatches != 1 {
		t.Fatalf("applied memory patches = %d, want 1", result.AppliedStoryMemoryPatches)
	}
	snapshot, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.CurrentTurn == nil || snapshot.CurrentTurn.MemoryStatus != "ready" || snapshot.CurrentTurn.StateStatus != "ready" {
		t.Fatalf("turn memory/state should be ready: %#v", snapshot.CurrentTurn)
	}
	if snapshot.DirectorPlanStatus == nil || snapshot.DirectorPlanStatus.Status != interactive.DirectorPlanStatusReady {
		t.Fatalf("director plan should be ready: %#v", snapshot.DirectorPlanStatus)
	}
	if snapshot.DirectorPlan == nil || !strings.Contains(snapshot.DirectorPlan.Docs.Plan, "钟楼盯梢者") {
		t.Fatalf("director plan should include updated file docs: %#v", snapshot.DirectorPlan)
	}
	memory, err := store.StoryMemory(story.ID, "main", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(memory.Records) != 1 || memory.Records[0].StructureID != "important_character" || memory.Records[0].Key != "林川" {
		t.Fatalf("story memory record mismatch: %#v", memory.Records)
	}
}

func TestInteractiveDirectorMaintenanceKeepsDirectorReadyWhenMemoryToolFails(t *testing.T) {
	workspace := t.TempDir()
	store := interactive.NewStore(workspace)
	story, err := store.CreateStory(interactive.CreateStoryRequest{Title: "记忆失败", Origin: "主角进入旧城", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	turn, _, err := store.AppendTurnWithState(story.ID, interactive.AppendTurnWithStateRequest{
		BranchID:  "main",
		User:      "我观察钟楼",
		Narrative: "钟楼上有反光一闪。",
		TurnResult: committedTurnResultForTest(
			"观察钟楼",
			"确认反光来源",
			"钟楼上出现可疑反光",
		),
	})
	if err != nil {
		t.Fatal(err)
	}
	directorGenerator := func(_ context.Context, _ *config.Config, _ *book.State, toolContext agent.InteractiveStoryToolContext, _ string) (string, error) {
		if toolContext.MaintenanceTask == interactiveDirectorTaskMemoryUpdate {
			toolContext.OnStateMaintenanceFailed(errors.New("story memory write failed"))
			return "故事记忆写入失败", nil
		}
		plan, err := toolContext.Store.DirectorPlan(toolContext.StoryID, toolContext.BranchID)
		if err != nil {
			return "", err
		}
		docs := plan.Docs
		docs.Plan = strings.Replace(docs.Plan, "明确当前场景、主角处境、直接目标和可玩行动空间，让用户能观察、对话、调查、冒险、交易或保守应对。", "钟楼反光作为下一轮调查入口。", 1)
		if err := writeDirectorPlanDocsForTest(toolContext.DirectorPlanAllowedPaths, docs); err != nil {
			return "", err
		}
		return `{"mode":"patch","triggers":["new_clue"],"reason":"反光成为调查入口"}`, nil
	}
	conversation := newInteractiveConversation(store, t.TempDir(), workspace, story.ID, "main", turn.User, story.ReplyTargetChars, &config.Config{})
	conversation.directorGenerator = directorGenerator
	if _, err := runInteractiveDirectorMaintenance(context.Background(), &config.Config{}, book.NewState(workspace), conversation, turn, nil, interactiveDirectorTaskTurnMaintenance); err == nil || !strings.Contains(err.Error(), "story memory write failed") {
		t.Fatalf("maintenance should report memory failure, got %v", err)
	}
	snapshot, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.DirectorPlanStatus == nil || snapshot.DirectorPlanStatus.Status != interactive.DirectorPlanStatusReady || snapshot.DirectorPlan == nil || !strings.Contains(snapshot.DirectorPlan.Docs.Plan, "钟楼反光") {
		t.Fatalf("director plan should stay ready after memory failure: status=%#v plan=%#v", snapshot.DirectorPlanStatus, snapshot.DirectorPlan)
	}
	if snapshot.CurrentTurn == nil || snapshot.CurrentTurn.MemoryStatus != "failed" || !strings.Contains(snapshot.CurrentTurn.MemoryError, "story memory write failed") {
		t.Fatalf("memory failure should be recorded without hiding director success: %#v", snapshot.CurrentTurn)
	}
}

func TestInteractiveDirectorMaintenanceKeepsMemoryWhenDirectorPlanValidationFails(t *testing.T) {
	workspace := t.TempDir()
	store := interactive.NewStore(workspace)
	story, err := store.CreateStory(interactive.CreateStoryRequest{Title: "规划失败", Origin: "主角进入旧城", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	turn, _, err := store.AppendTurnWithState(story.ID, interactive.AppendTurnWithStateRequest{
		BranchID:  "main",
		User:      "我检查破损路标",
		Narrative: "路标背面刻着林川留下的警告。",
		TurnResult: committedTurnResultForTest(
			"检查破损路标",
			"识别路标信息",
			"路标背面有林川留下的警告",
		),
	})
	if err != nil {
		t.Fatal(err)
	}
	directorGenerator := func(_ context.Context, _ *config.Config, _ *book.State, toolContext agent.InteractiveStoryToolContext, _ string) (string, error) {
		if toolContext.MaintenanceTask == interactiveDirectorTaskMemoryUpdate {
			if err := applyDirectorMemoryForTest(toolContext); err != nil {
				return "", err
			}
			return "已整理故事记忆", nil
		}
		if len(toolContext.DirectorPlanAllowedPaths) != 1 {
			return "", errors.New("missing director path")
		}
		if err := os.WriteFile(toolContext.DirectorPlanAllowedPaths[0], []byte("# 缺少固定标题\n\n这份规划不合法。\n"), 0o644); err != nil {
			return "", err
		}
		return "记忆已写入，导演规划校验失败", nil
	}
	conversation := newInteractiveConversation(store, t.TempDir(), workspace, story.ID, "main", turn.User, story.ReplyTargetChars, &config.Config{})
	conversation.directorGenerator = directorGenerator
	if _, err := runInteractiveDirectorMaintenance(context.Background(), &config.Config{}, book.NewState(workspace), conversation, turn, nil, interactiveDirectorTaskTurnMaintenance); err == nil || !strings.Contains(err.Error(), "完成导演规划运行失败") {
		t.Fatalf("maintenance should report director validation failure, got %v", err)
	}
	snapshot, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.CurrentTurn == nil || snapshot.CurrentTurn.MemoryStatus != "ready" || snapshot.CurrentTurn.StateStatus != "ready" {
		t.Fatalf("memory/state should remain ready after director failure: %#v", snapshot.CurrentTurn)
	}
	if snapshot.DirectorPlanStatus == nil || snapshot.DirectorPlanStatus.Status != interactive.DirectorPlanStatusFailed {
		t.Fatalf("director plan should be failed: %#v", snapshot.DirectorPlanStatus)
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
		TurnBrief: &interactive.TurnBrief{
			UserAction: "强行穿过禁制",
			TurnGoal:   "制造失败代价",
		},
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
		if err := writeDirectorPlanDocsForTest(toolContext.DirectorPlanAllowedPaths, docs); err != nil {
			return "", err
		}
		return "失败后重试成功", nil
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
		BriefDescription: "角色 沈凝。外门比试的关键见证者。上下文出现沈凝相关内容时，一定要参考本项详情。",
		Content:          "沈凝是外门比试的关键见证者，会被公开证据触动。",
	}); err != nil {
		t.Fatal(err)
	}
	turn, _, err := store.AppendTurnWithState(story.ID, interactive.AppendTurnWithStateRequest{
		BranchID:  "main",
		User:      "我邀请沈凝旁观公开比试",
		Narrative: "沈凝停下脚步，示意我继续说。",
		TurnBrief: &interactive.TurnBrief{
			UserAction:   "邀请沈凝旁观公开比试",
			EventIntents: []string{"face_slap"},
		},
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
		if part.Title == "当前导演规划文档快照" && strings.Contains(part.Content, "正文Agent可读") {
			sawDirectorPlan = true
		}
	}
	if !sawLore || !sawTurnAudit || !sawDirectorPlan {
		t.Fatalf("director context should include lore, turn audit, and director.md snapshot: lore=%v audit=%v plan=%v parts=%#v", sawLore, sawTurnAudit, sawDirectorPlan, analysis.ContextMessages)
	}
}

func writeDirectorPlanDocsForTest(paths []string, docs interactive.DirectorPlanDocs) error {
	if len(paths) != 1 {
		return errors.New("expected one director plan path")
	}
	if err := os.WriteFile(paths[0], []byte(strings.TrimSpace(docs.Plan)+"\n"), 0o644); err != nil {
		return err
	}
	return nil
}

func applyDirectorMemoryForTest(toolContext agent.InteractiveStoryToolContext) error {
	records, err := toolContext.Store.ApplyStoryMemoryPatches(toolContext.StoryID, toolContext.BranchID, toolContext.TurnID, []interactive.StoryMemoryPatch{{
		Op:          "upsert",
		StructureID: "important_character",
		Values: map[string]string{
			"name":                        "林川",
			"current_status":              "确认钟楼上有人盯梢",
			"relationship_to_protagonist": "与主角共同戒备钟楼威胁",
		},
	}})
	if err != nil {
		return err
	}
	if toolContext.OnStoryMemoryApplied != nil {
		toolContext.OnStoryMemoryApplied(len(records))
	}
	return nil
}

func committedTurnResultForTest(playerIntent, sceneGoal, fact string) *interactive.TurnResult {
	result := &interactive.TurnResult{
		Contract: interactive.TurnContract{PlayerIntent: playerIntent, SceneGoal: sceneGoal},
		SceneResult: interactive.TurnSceneResult{
			Status:  "continued",
			Summary: fact,
		},
	}
	if strings.TrimSpace(fact) != "" {
		result.FactCandidates = []interactive.StoryFactCandidate{{
			Kind:       "plot",
			Subject:    "当前场景",
			Fact:       fact,
			Visibility: "player_known",
			Importance: "medium",
		}}
	}
	return result
}
