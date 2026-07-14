package interactive

import (
	"strings"
	"testing"
)

func TestStageDirectorPlanRunUpdateValidatesOneCompleteSubmission(t *testing.T) {
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{Title: "结构化导演提交"})
	if err != nil {
		t.Fatal(err)
	}
	turn, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{BranchID: "main", User: "前进", Narrative: "抵达门前。"})
	if err != nil {
		t.Fatal(err)
	}
	token, err := store.DirectorPlanRunToken(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.MarkDirectorPlanRunStarted(story.ID, "main", token, turn.ID); err != nil {
		t.Fatal(err)
	}
	plan, err := store.DirectorPlan(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	updatedDocs := plan.Docs
	updatedDocs.Plan = strings.Replace(updatedDocs.Plan, "围绕主角当前最想解决的问题", "围绕本轮已经确认的门后危机", 1)
	receipt, err := store.StageDirectorPlanRunUpdate(story.ID, "main", token, turn.ID, DirectorPlanUpdateSubmission{
		Decision: PlanDecision{Mode: PlanDecisionPatch, Reason: "场景已推进"},
		Docs:     &updatedDocs,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !receipt.Accepted || !receipt.DocsUpdated || receipt.Decision.Mode != PlanDecisionPatch {
		t.Fatalf("unexpected receipt: %#v", receipt)
	}
	staged, err := store.DirectorPlan(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if staged.Docs.Plan != updatedDocs.Plan || staged.Metadata.LastRun == nil || staged.Metadata.LastRun.Status != DirectorPlanStatusRunning {
		t.Fatalf("documents should be staged without prematurely completing metadata: %#v", staged)
	}
	completed, err := store.CompleteDirectorPlanRun(story.ID, "main", token, turn.ID, `{"mode":"patch","reason":"场景已推进"}`)
	if err != nil {
		t.Fatal(err)
	}
	if completed.Metadata.LastRun == nil || completed.Metadata.LastRun.Status != DirectorPlanStatusReady {
		t.Fatalf("structured update did not complete: %#v", completed.Metadata.LastRun)
	}
}

func TestStageDirectorPlanRunUpdateKeepsModulesAtomic(t *testing.T) {
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{Title: "导演原子校验"})
	if err != nil {
		t.Fatal(err)
	}
	turn, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{BranchID: "main", User: "等待", Narrative: "局势未变。"})
	if err != nil {
		t.Fatal(err)
	}
	token, _ := store.DirectorPlanRunToken(story.ID, "main")
	if err := store.MarkDirectorPlanRunStarted(story.ID, "main", token, turn.ID); err != nil {
		t.Fatal(err)
	}
	before, _ := store.DirectorPlan(story.ID, "main")
	bad := before.Docs
	bad.LoreContext = "缺少固定标题"
	if _, err := store.StageDirectorPlanRunUpdate(story.ID, "main", token, turn.ID, DirectorPlanUpdateSubmission{Decision: PlanDecision{Mode: PlanDecisionPatch}, Docs: &bad}); err == nil {
		t.Fatal("invalid lore-context should reject the complete submission")
	}
	after, _ := store.DirectorPlan(story.ID, "main")
	if after.Docs != before.Docs {
		t.Fatalf("a rejected submission changed one of the documents: before=%#v after=%#v", before.Docs, after.Docs)
	}
	if _, err := store.StageDirectorPlanRunUpdate(story.ID, "main", token, turn.ID, DirectorPlanUpdateSubmission{Decision: PlanDecision{Mode: PlanDecisionKeep}, Docs: &before.Docs}); err == nil {
		t.Fatal("keep with docs must be rejected")
	}
	if _, err := store.StageDirectorPlanRunUpdate(story.ID, "main", token, turn.ID, DirectorPlanUpdateSubmission{Decision: PlanDecision{Mode: ""}}); err == nil {
		t.Fatal("an omitted mode must be rejected instead of defaulting to keep")
	}
	if _, err := store.StageDirectorPlanRunUpdate(story.ID, "main", token, turn.ID, DirectorPlanUpdateSubmission{Decision: PlanDecision{Mode: "rewrite"}}); err == nil {
		t.Fatal("an unknown mode must be rejected instead of defaulting to keep")
	}
	receipt, err := store.StageDirectorPlanRunUpdate(story.ID, "main", token, turn.ID, DirectorPlanUpdateSubmission{Decision: PlanDecision{Mode: PlanDecisionKeep, Reason: "计划仍有效"}})
	if err != nil || !receipt.Accepted || receipt.DocsUpdated {
		t.Fatalf("valid keep rejected: receipt=%#v err=%v", receipt, err)
	}
}
