package app

import (
	"context"
	"errors"
	"os"
	"strings"
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
	previous := generateInteractiveDirectorForPlan
	generateInteractiveDirectorForPlan = func(_ context.Context, _ *config.Config, _ *book.State, toolContext agent.InteractiveStoryToolContext, instruction string) (string, error) {
		if !strings.Contains(instruction, "mainline.md") || len(toolContext.DirectorPlanAllowedPaths) != 3 {
			t.Fatalf("director should receive plan paths and guard context: paths=%#v\n%s", toolContext.DirectorPlanAllowedPaths, instruction)
		}
		plan, err := toolContext.Store.DirectorPlan(toolContext.StoryID, toolContext.BranchID)
		if err != nil {
			return "", err
		}
		docs := plan.Docs
		docs.CurrentEvent = strings.Replace(docs.CurrentEvent, "明确当前事件的可玩目标，让用户知道能采取行动。", "公开比试制造质疑与反证机会。", 1)
		if err := writeDirectorPlanDocsForTest(toolContext.DirectorPlanAllowedPaths, docs); err != nil {
			return "", err
		}
		return "导演安排公开反转", nil
	}
	defer func() { generateInteractiveDirectorForPlan = previous }()

	conversation := newInteractiveConversation(store, t.TempDir(), workspace, story.ID, "main", turn.User, story.ReplyTargetChars, &config.Config{})
	startInteractiveDirectorTask(&config.Config{}, book.NewState(workspace), conversation, turn, nil)

	snapshot := waitForDirectorPlanRunSummary(t, store, story.ID, "main", "导演安排公开反转")
	if snapshot.CurrentTurn == nil || snapshot.CurrentTurn.ID != turn.ID {
		t.Fatalf("turn should remain current after director update: %#v", snapshot.CurrentTurn)
	}
	if snapshot.DirectorPlan == nil || !strings.Contains(snapshot.DirectorPlan.Docs.CurrentEvent, "公开比试制造质疑") {
		t.Fatalf("director plan should include file update: %#v", snapshot.DirectorPlan)
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
}

func writeDirectorPlanDocsForTest(paths []string, docs interactive.DirectorPlanDocs) error {
	if len(paths) != 3 {
		return errors.New("expected three director plan paths")
	}
	for i, content := range []string{docs.Mainline, docs.CurrentEvent, docs.NextBranches} {
		if err := os.WriteFile(paths[i], []byte(strings.TrimSpace(content)+"\n"), 0o644); err != nil {
			return err
		}
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
