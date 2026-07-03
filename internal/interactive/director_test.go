package interactive

import (
	"os"
	"strings"
	"testing"
)

func TestCreateStorySeedsDirectorPlanDocs(t *testing.T) {
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{
		Title:         "学院逆袭",
		Origin:        "主角被同门轻视，却握有残卷线索。",
		StoryTellerID: "classic",
		DirectorPlanSeed: &DirectorPlanSeed{
			Templates:           DefaultStoryDirectorPlanningTemplates(),
			BranchPlanningTurns: 5,
			Source:              "test_seed",
			OpeningSummary:      "隐脉觉醒。",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := store.DirectorPlan(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if plan.Metadata.BranchID != "main" || plan.Metadata.BranchPlanningTurns != 5 || plan.Metadata.LastRun == nil {
		t.Fatalf("director metadata mismatch: %#v", plan.Metadata)
	}
	for name, doc := range map[string]string{
		"mainline":      plan.Docs.Mainline,
		"current-event": plan.Docs.CurrentEvent,
		"next-branches": plan.Docs.NextBranches,
	} {
		if err := validateDirectorPlanDoc(name, doc); err != nil {
			t.Fatalf("%s should be valid: %v\n%s", name, err, doc)
		}
		if !strings.Contains(doc, "隐脉觉醒") {
			t.Fatalf("%s should include opening summary:\n%s", name, doc)
		}
	}
	if strings.Contains(plan.VisibleDocs.Mainline, "后台导演私密") {
		t.Fatalf("visible docs must exclude private section:\n%s", plan.VisibleDocs.Mainline)
	}
	for _, path := range store.DirectorPlanAllowedPaths(story.ID, "main") {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("director plan path should exist %s: %v", path, err)
		}
	}
}

func TestCreateBranchSeedsBranchDirectorPlan(t *testing.T) {
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{Title: "分支故事", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	turn, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID:  "main",
		User:      "我走上擂台",
		Narrative: "钟声落下，人群看向擂台。",
	})
	if err != nil {
		t.Fatal(err)
	}
	branch, err := store.CreateBranch(story.ID, CreateBranchRequest{ParentEventID: turn.ID, Title: "暗中修炼"})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := store.DirectorPlan(story.ID, branch.ID)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Metadata.BranchID != branch.ID || plan.Metadata.Source != "branch_seed" {
		t.Fatalf("branch director metadata mismatch: %#v", plan.Metadata)
	}
	if !strings.Contains(plan.Docs.Mainline, "分支说明") || !strings.Contains(plan.Docs.Mainline, branch.ID) {
		t.Fatalf("branch plan should carry branch note:\n%s", plan.Docs.Mainline)
	}
}

func TestUpdateDirectorPlanValidatesRequiredHeadingsAndRevision(t *testing.T) {
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{Title: "手动规划", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := store.DirectorPlan(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	nextDocs := plan.Docs
	nextDocs.NextBranches += "\n\n可见安排：给玩家观察、对话、调查三个方向。"
	updated, err := store.UpdateDirectorPlan(story.ID, UpdateDirectorPlanRequest{
		BranchID:     "main",
		Docs:         nextDocs,
		BaseRevision: plan.Metadata.Revision,
		Source:       "manual",
		Summary:      "测试保存。",
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Metadata.LastRun == nil || updated.Metadata.LastRun.Status != "ready" || updated.Metadata.Revision == plan.Metadata.Revision {
		t.Fatalf("manual save should refresh metadata: %#v", updated.Metadata)
	}
	badDocs := updated.Docs
	badDocs.Mainline = "# 缺标题"
	if _, err := store.UpdateDirectorPlan(story.ID, UpdateDirectorPlanRequest{BranchID: "main", Docs: badDocs}); err == nil || !strings.Contains(err.Error(), "缺少必填标题") {
		t.Fatalf("invalid plan should be rejected, err=%v", err)
	}
	if _, err := store.UpdateDirectorPlan(story.ID, UpdateDirectorPlanRequest{BranchID: "main", Docs: updated.Docs, BaseRevision: plan.Metadata.Revision}); err == nil || !strings.Contains(err.Error(), "重新加载") {
		t.Fatalf("stale revision should be rejected, err=%v", err)
	}
}

func TestCompleteDirectorPlanRunDetectsManualConflict(t *testing.T) {
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{Title: "冲突规划", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	token, err := store.DirectorPlanRunToken(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	plan, err := store.DirectorPlan(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	docs := plan.Docs
	docs.CurrentEvent += "\n\n手动保存：用户调整当前事件。"
	if _, err := store.UpdateDirectorPlan(story.ID, UpdateDirectorPlanRequest{BranchID: "main", Docs: docs, BaseRevision: plan.Metadata.Revision}); err != nil {
		t.Fatal(err)
	}
	completed, err := store.CompleteDirectorPlanRun(story.ID, "main", token, "turn_1", "后台导演尝试完成。")
	if err != nil {
		t.Fatal(err)
	}
	if completed.Metadata.LastRun == nil || completed.Metadata.LastRun.Status != "conflict" {
		t.Fatalf("manual save during run should mark conflict: %#v", completed.Metadata.LastRun)
	}
}

func TestDirectorPlanVisibleContextExcludesPrivateSections(t *testing.T) {
	docs := DefaultStoryDirectorPlanningTemplates()
	plan := DirectorPlan{VisibleDocs: DirectorPlanVisibleDocs{
		Mainline:     ExtractDirectorPlanVisibleSection(docs.Mainline),
		CurrentEvent: ExtractDirectorPlanVisibleSection(docs.CurrentEvent),
		NextBranches: ExtractDirectorPlanVisibleSection(docs.NextBranches),
	}}
	context := DirectorPlanVisibleContext(plan, 4096)
	if !strings.Contains(context, "正文Agent可读") || !strings.Contains(context, "大方向规划") {
		t.Fatalf("visible context should include public sections:\n%s", context)
	}
	if strings.Contains(context, "后台导演私密") || strings.Contains(context, "隐藏状态") {
		t.Fatalf("visible context should exclude private sections:\n%s", context)
	}
}

func TestTerminalBranchRejectsFurtherTurnsUntilNewBranch(t *testing.T) {
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{Title: "终局分支", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	brief := TurnBrief{
		UserAction: "强闯禁制",
		Intent:     "冒险",
		TurnGoal:   "错误选择产生终局",
		RuleChecks: []RuleCheck{{
			ID:                "gate",
			Difficulty:        99,
			TerminalOnFailure: true,
			TerminalType:      "mainline_failed",
			TerminalReason:    "主线入口崩塌。",
			Seed:              1,
		}},
	}
	resolution, err := ResolveTurnRules(story.ID, "main", initialStoryState(), brief)
	if err != nil {
		t.Fatal(err)
	}
	terminal := terminalOutcomeFromRuleResolution(resolution, "turn", "禁制炸裂，入口坍塌。")
	turn, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID:        "main",
		User:            brief.UserAction,
		Narrative:       "禁制炸裂，入口坍塌。",
		TurnBrief:       &brief,
		RuleResolution:  &resolution,
		TerminalOutcome: terminal,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID:  "main",
		User:      "继续前进",
		Narrative: "我继续向前。",
	}); err == nil || !strings.Contains(err.Error(), "终局") {
		t.Fatalf("terminal branch should reject normal append, err=%v", err)
	}
	branch, err := store.CreateBranch(story.ID, CreateBranchRequest{ParentEventID: turn.ID, Title: "终局节点"})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID:  branch.ID,
		User:      "仍要继续",
		Narrative: "我仍要继续。",
	}); err == nil {
		t.Fatalf("branch created from terminal node should also reject continuation")
	}
}

func TestRerollRuleResolutionUpdatesTurnAudit(t *testing.T) {
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{
		Title:           "规则重抽",
		StoryTellerID:   "classic",
		InitialStateOps: []StateOp{{Op: "set", Path: "resources.stamina", Value: float64(5)}},
	})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	brief := TurnBrief{
		UserAction: "冲刺",
		Intent:     "冒险",
		TurnGoal:   "结算冲刺",
		RuleChecks: []RuleCheck{{ID: "dash", AttributePath: "resources.stamina", Dice: "1d20", Difficulty: 10, Seed: 2}},
	}
	resolution, err := ResolveTurnRules(story.ID, "main", snapshot.State, brief)
	if err != nil {
		t.Fatal(err)
	}
	turn, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID:       "main",
		User:           brief.UserAction,
		Narrative:      "他冲了出去。",
		TurnBrief:      &brief,
		RuleResolution: &resolution,
	})
	if err != nil {
		t.Fatal(err)
	}
	reroll, err := store.RerollRuleResolution(story.ID, resolution.ID, RuleResolutionRerollRequest{TurnID: turn.ID})
	if err != nil {
		t.Fatal(err)
	}
	if reroll.ID == resolution.ID {
		t.Fatalf("reroll should create a new resolution id")
	}
	if reroll.RuleResults[0].Seed == resolution.RuleResults[0].Seed {
		t.Fatalf("reroll should use a new seed: old=%d new=%d", resolution.RuleResults[0].Seed, reroll.RuleResults[0].Seed)
	}
	snapshot, err = store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.CurrentTurn == nil || snapshot.CurrentTurn.RuleResolution == nil || snapshot.CurrentTurn.RuleResolution.ID != reroll.ID {
		t.Fatalf("rerolled resolution should be persisted: %#v", snapshot.CurrentTurn)
	}
}
