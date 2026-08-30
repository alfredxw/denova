package interactive

import (
	"errors"
	"testing"
)

func TestCommitDomainTurnRegenerateAtomicallyReplacesBranchHead(t *testing.T) {
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{Title: "原子再生成", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	first, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{BranchID: "main", User: "进入", Narrative: "进入门厅。"})
	if err != nil {
		t.Fatal(err)
	}
	oldHead, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{BranchID: "main", User: "开门", Narrative: "门后是旧走廊。"})
	if err != nil {
		t.Fatal(err)
	}
	contextAtParent, err := store.StoryContextAtTurnParent(story.ID, "main", oldHead.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(contextAtParent.Snapshot.Turns) != 1 || contextAtParent.Snapshot.Turns[0].ID != first.ID {
		t.Fatalf("regeneration context = %+v, want only target parent history", contextAtParent.Snapshot.Turns)
	}
	expectedHead := oldHead.ID
	intent, err := NewDomainCommitIntent(AppendTurnWithStateRequest{
		BranchID: "main", ExpectedParentID: &expectedHead, ReplaceTurnID: oldHead.ID,
		User: "开门", Narrative: "门后变成了星光庭院。",
		AgentCommandID: "regenerate-command", AgentOperationID: "regenerate-operation", AgentCycle: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	commitPlayerInputForTurnTest(t, store, story.ID, intent.Request)
	receipt, err := store.CommitDomainTurn(story.ID, intent)
	if err != nil {
		t.Fatal(err)
	}
	if parentIDString(receipt.Turn.ParentID) != first.ID || receipt.Revision != receipt.Turn.ID {
		t.Fatalf("replacement receipt = %+v, want parent %s", receipt, first.ID)
	}
	retry, err := store.CommitDomainTurn(story.ID, intent)
	if err != nil {
		t.Fatal(err)
	}
	if retry.Revision != receipt.Revision || retry.Turn.ID != receipt.Turn.ID {
		t.Fatalf("exact retry created another replacement: first=%+v retry=%+v", receipt, retry)
	}
	snapshot, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Turns) != 2 || snapshot.Turns[0].ID != first.ID || snapshot.Turns[1].ID != receipt.Turn.ID {
		t.Fatalf("canonical branch path after replacement = %+v", snapshot.Turns)
	}
}

func TestCommitDomainTurnRegenerateRejectsHeadDriftWithoutRewind(t *testing.T) {
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{Title: "再生成 CAS", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	target, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{BranchID: "main", User: "前进", Narrative: "旧版本。"})
	if err != nil {
		t.Fatal(err)
	}
	expectedHead := target.ID
	intent, err := NewDomainCommitIntent(AppendTurnWithStateRequest{
		BranchID: "main", ExpectedParentID: &expectedHead, ReplaceTurnID: target.ID,
		User: "前进", Narrative: "准备替换。",
		AgentCommandID: "regenerate-command", AgentOperationID: "regenerate-operation", AgentCycle: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	commitPlayerInputForTurnTest(t, store, story.ID, intent.Request)
	newHead, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{BranchID: "main", User: "并发前进", Narrative: "分支已经前进。"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CommitDomainTurn(story.ID, intent); !errors.Is(err, ErrStoryContextRevisionConflict) {
		t.Fatalf("stale regenerate error = %v, want %v", err, ErrStoryContextRevisionConflict)
	}
	snapshot, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Turns) != 2 || snapshot.Turns[1].ID != newHead.ID {
		t.Fatalf("failed regenerate changed canonical head: %+v", snapshot.Turns)
	}
}

func TestStoryContextAtTurnParentRestoresHistoricalBranchPlan(t *testing.T) {
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{Title: "分支规划上下文隔离", StoryTellerID: "classic", PlanningMode: StoryPlanningModeEnabled})
	if err != nil {
		t.Fatal(err)
	}
	baselinePlan := "Keep the sealed door unresolved."
	_, _, err = store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID: "main", User: "来到门前", Narrative: "门仍然封闭。",
		TurnResult: &TurnResult{Choices: testTurnChoices(), PlanUpdate: &baselinePlan},
	})
	if err != nil {
		t.Fatal(err)
	}
	targetPlan := "Investigate what is behind the door."
	target, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID: "main", User: "打开门", Narrative: "门后显出一道走廊。",
		TurnResult: &TurnResult{Choices: testTurnChoices(), PlanUpdate: &targetPlan},
	})
	if err != nil {
		t.Fatal(err)
	}
	projection, err := store.StoryContextAtTurnParent(story.ID, "main", target.ID)
	if err != nil {
		t.Fatal(err)
	}
	if projection.Snapshot.BranchPlan == nil || projection.Snapshot.BranchPlan.Markdown != baselinePlan {
		t.Fatalf("historical regeneration projection has the wrong plan: %+v", projection.Snapshot.BranchPlan)
	}
	if len(projection.Snapshot.TokenUsageEvents) != 0 {
		t.Fatalf("historical regeneration projection exposed branch-wide telemetry: %+v", projection.Snapshot.TokenUsageEvents)
	}
}

func TestRegeneratePreservesCurrentBranchPlanWhenUpdateIsOmitted(t *testing.T) {
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{Title: "重生成沿用规划", PlanningMode: StoryPlanningModeEnabled})
	if err != nil {
		t.Fatal(err)
	}
	baselinePlan := "Keep the station entrance available."
	_, _, err = store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID: "main", User: "抵达车站", Narrative: "入口仍然敞开。",
		TurnResult: &TurnResult{Choices: testTurnChoices(), PlanUpdate: &baselinePlan},
	})
	if err != nil {
		t.Fatal(err)
	}
	currentPlan := "Follow the bell signal while preserving an exit route."
	target, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID: "main", User: "聆听钟声", Narrative: "钟声来自北侧月台。",
		TurnResult: &TurnResult{Choices: testTurnChoices(), PlanUpdate: &currentPlan},
	})
	if err != nil {
		t.Fatal(err)
	}
	expectedHead := target.ID
	replacement, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID: "main", ExpectedParentID: &expectedHead, ReplaceTurnID: target.ID,
		User: "聆听钟声", Narrative: "钟声改从封闭的钟楼传来。",
		TurnResult: &TurnResult{Choices: testTurnChoices()},
	})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.BranchPlan == nil || snapshot.BranchPlan.Markdown != currentPlan || snapshot.BranchPlan.UpdatedTurnID != replacement.ID {
		t.Fatalf("replacement did not preserve the current plan: %#v", snapshot.BranchPlan)
	}
}
