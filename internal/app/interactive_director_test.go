package app

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"denova/config"
	"denova/internal/agent"
	"denova/internal/book"
	"denova/internal/interactive"
)

func TestInteractiveDirectorTaskAppliesGeneratedPatch(t *testing.T) {
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
	generateInteractiveDirectorForPlan = func(context.Context, *config.Config, *book.State, agent.InteractiveStoryToolContext, string) (string, error) {
		return `{"summary":"导演安排公开反转","stage_plan":"下一阶段围绕公开比试制造打脸反转。","beat_queue":[{"id":"beat_test","summary":"公开比试开场","pressure":"同门质疑","payoff":"证明实力","status":"planned"}],"event_queue":[{"id":"face_slap","name":"打脸反转","category":"打脸","status":"planned","enabled":true,"summary":"公开反证轻视者。"}]}`, nil
	}
	defer func() { generateInteractiveDirectorForPlan = previous }()

	conversation := newInteractiveConversation(store, t.TempDir(), workspace, story.ID, "main", turn.User, story.ReplyTargetChars, &config.Config{})
	startInteractiveDirectorTask(&config.Config{}, book.NewState(workspace), conversation, turn, nil)

	snapshot := waitForDirectorRunSummary(t, store, story.ID, "main", "导演安排公开反转")
	if !strings.Contains(snapshot.DirectorState.StagePlan, "公开比试") {
		t.Fatalf("stage plan should come from director patch: %#v", snapshot.DirectorState)
	}
	if snapshot.DirectorState.LastDirectorRun == nil || snapshot.DirectorState.LastDirectorRun.Summary != "导演安排公开反转" {
		t.Fatalf("director run summary mismatch: %#v", snapshot.DirectorState.LastDirectorRun)
	}
	if len(snapshot.DirectorState.BeatQueue) == 0 || snapshot.DirectorState.BeatQueue[0].ID != "beat_test" {
		t.Fatalf("beat queue mismatch: %#v", snapshot.DirectorState.BeatQueue)
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

	snapshot := waitForDirectorRunStatus(t, store, story.ID, "main", "failed")
	if snapshot.CurrentTurn == nil || snapshot.CurrentTurn.ID != turn.ID {
		t.Fatalf("turn should remain current after director failure: %#v", snapshot.CurrentTurn)
	}
	if snapshot.DirectorState.LastDirectorRun == nil || !strings.Contains(snapshot.DirectorState.LastDirectorRun.Error, "director unavailable") {
		t.Fatalf("failure should be recorded: %#v", snapshot.DirectorState.LastDirectorRun)
	}
}

func TestParseInteractiveDirectorPatchAcceptsFencedJSON(t *testing.T) {
	patch, err := parseInteractiveDirectorPatch("```json\n{\"summary\":\"已更新\",\"stage_plan\":\"安排误会消解\"}\n```")
	if err != nil {
		t.Fatal(err)
	}
	if patch.StagePlan == nil || *patch.StagePlan != "安排误会消解" || patch.Summary != "已更新" {
		t.Fatalf("unexpected patch: %#v", patch)
	}
}

func waitForDirectorRunStatus(t *testing.T, store *interactive.Store, storyID, branchID, status string) interactive.Snapshot {
	t.Helper()
	deadline := time.Now().Add(500 * time.Millisecond)
	for {
		snapshot, err := store.Snapshot(storyID, branchID)
		if err != nil {
			t.Fatal(err)
		}
		if snapshot.DirectorState.LastDirectorRun != nil && snapshot.DirectorState.LastDirectorRun.Status == status {
			return snapshot
		}
		if time.Now().After(deadline) {
			t.Fatalf("director run did not reach status %q: %#v", status, snapshot.DirectorState.LastDirectorRun)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func waitForDirectorRunSummary(t *testing.T, store *interactive.Store, storyID, branchID, summary string) interactive.Snapshot {
	t.Helper()
	deadline := time.Now().Add(500 * time.Millisecond)
	for {
		snapshot, err := store.Snapshot(storyID, branchID)
		if err != nil {
			t.Fatal(err)
		}
		if snapshot.DirectorState.LastDirectorRun != nil && snapshot.DirectorState.LastDirectorRun.Summary == summary {
			return snapshot
		}
		if time.Now().After(deadline) {
			t.Fatalf("director run did not reach summary %q: %#v", summary, snapshot.DirectorState.LastDirectorRun)
		}
		time.Sleep(5 * time.Millisecond)
	}
}
