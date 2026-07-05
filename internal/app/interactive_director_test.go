package app

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

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
	if initialStatus.Status != interactive.DirectorPlanStatusWaitingOpening || !initialStatus.Blocking {
		t.Fatalf("first persisted turn should block until director planning starts: %#v", initialStatus)
	}
	started := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(release) })
	previous := generateInteractiveDirectorForPlan
	generateInteractiveDirectorForPlan = func(_ context.Context, _ *config.Config, _ *book.State, toolContext agent.InteractiveStoryToolContext, instruction string) (string, error) {
		close(started)
		<-release
		if !strings.Contains(instruction, "director.md") || strings.Contains(instruction, "mainline.md") || len(toolContext.DirectorPlanAllowedPaths) != 1 {
			t.Fatalf("director should receive plan paths and guard context: paths=%#v\n%s", toolContext.DirectorPlanAllowedPaths, instruction)
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
	defer func() { generateInteractiveDirectorForPlan = previous }()

	conversation := newInteractiveConversation(store, t.TempDir(), workspace, story.ID, "main", turn.User, story.ReplyTargetChars, &config.Config{})
	startInteractiveDirectorTask(&config.Config{}, book.NewState(workspace), conversation, turn, nil)

	waitForDirectorGoroutineStart(t, started)
	runningStatus := waitForDirectorPlanPublicStatus(t, store, story.ID, "main", interactive.DirectorPlanStatusRunning)
	if !runningStatus.Blocking || runningStatus.StartReady || runningStatus.CompletedDocs != 0 || runningStatus.PlannedDocs != 1 {
		t.Fatalf("initial director run should expose blocking progress only: %#v", runningStatus)
	}
	releaseOnce.Do(func() { close(release) })
	snapshot := waitForDirectorPlanRunSummary(t, store, story.ID, "main", "导演安排公开反转")
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
	previous := generateInteractiveDirectorForPlan
	generateInteractiveDirectorForPlan = func(context.Context, *config.Config, *book.State, agent.InteractiveStoryToolContext, string) (string, error) {
		return "", errors.New("director unavailable")
	}
	defer func() { generateInteractiveDirectorForPlan = previous }()

	conversation := newInteractiveConversation(store, t.TempDir(), workspace, story.ID, "main", turn.User, story.ReplyTargetChars, &config.Config{})
	startInteractiveDirectorTask(&config.Config{}, book.NewState(workspace), conversation, turn, nil)

	snapshot := waitForDirectorPlanRunStatus(t, store, story.ID, "main", "failed")
	if snapshot.CurrentTurn == nil || snapshot.CurrentTurn.ID != turn.ID {
		t.Fatalf("turn should remain current after director failure: %#v", snapshot.CurrentTurn)
	}
	if snapshot.DirectorPlan == nil || snapshot.DirectorPlan.Metadata.LastRun == nil || !strings.Contains(snapshot.DirectorPlan.Metadata.LastRun.Error, "director unavailable") {
		t.Fatalf("failure should be recorded: %#v", snapshot.DirectorPlan)
	}
	if snapshot.DirectorPlanStatus == nil || snapshot.DirectorPlanStatus.Status != interactive.DirectorPlanStatusFailed || !snapshot.DirectorPlanStatus.Blocking || snapshot.DirectorPlanStatus.StartReady {
		t.Fatalf("initial director failure should block until retry: %#v", snapshot.DirectorPlanStatus)
	}

	previous = generateInteractiveDirectorForPlan
	generateInteractiveDirectorForPlan = func(_ context.Context, _ *config.Config, _ *book.State, toolContext agent.InteractiveStoryToolContext, _ string) (string, error) {
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
	defer func() { generateInteractiveDirectorForPlan = previous }()

	startInteractiveDirectorTask(&config.Config{}, book.NewState(workspace), conversation, turn, nil)
	retried := waitForDirectorPlanRunSummary(t, store, story.ID, "main", "失败后重试成功")
	if retried.DirectorPlanStatus == nil || retried.DirectorPlanStatus.Status != interactive.DirectorPlanStatusReady || !retried.DirectorPlanStatus.StartReady || retried.DirectorPlanStatus.Blocking {
		t.Fatalf("retry should mark initial director plan ready: %#v", retried.DirectorPlanStatus)
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

func waitForDirectorPlanRunStatus(t *testing.T, store *interactive.Store, storyID, branchID, status string) interactive.Snapshot {
	t.Helper()
	deadline := time.Now().Add(500 * time.Millisecond)
	for {
		snapshot, err := store.Snapshot(storyID, branchID)
		if err != nil {
			t.Fatal(err)
		}
		if snapshot.DirectorPlan != nil && snapshot.DirectorPlan.Metadata.LastRun != nil && snapshot.DirectorPlan.Metadata.LastRun.Status == status {
			return snapshot
		}
		if time.Now().After(deadline) {
			t.Fatalf("director run did not reach status %q: %#v", status, snapshot.DirectorPlan)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func waitForDirectorPlanPublicStatus(t *testing.T, store *interactive.Store, storyID, branchID, status string) interactive.DirectorPlanStatus {
	t.Helper()
	deadline := time.Now().Add(500 * time.Millisecond)
	for {
		current, err := store.DirectorPlanStatus(storyID, branchID)
		if err != nil {
			t.Fatal(err)
		}
		if current.Status == status {
			return current
		}
		if time.Now().After(deadline) {
			t.Fatalf("director public status did not reach %q: %#v", status, current)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func waitForDirectorPlanRunSummary(t *testing.T, store *interactive.Store, storyID, branchID, summary string) interactive.Snapshot {
	t.Helper()
	deadline := time.Now().Add(500 * time.Millisecond)
	for {
		snapshot, err := store.Snapshot(storyID, branchID)
		if err != nil {
			t.Fatal(err)
		}
		if snapshot.DirectorPlan != nil && snapshot.DirectorPlan.Metadata.LastRun != nil && snapshot.DirectorPlan.Metadata.LastRun.Summary == summary {
			return snapshot
		}
		if time.Now().After(deadline) {
			t.Fatalf("director run did not reach summary %q: %#v", summary, snapshot.DirectorPlan)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func waitForDirectorGoroutineStart(t *testing.T, started <-chan struct{}) {
	t.Helper()
	select {
	case <-started:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("director goroutine did not start")
	}
}
