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
	if err := validateDirectorPlanDoc("plan", plan.Docs.Plan); err != nil {
		t.Fatalf("director plan should be valid: %v\n%s", err, plan.Docs.Plan)
	}
	if !strings.Contains(plan.Docs.Plan, "隐脉觉醒") {
		t.Fatalf("director plan should include opening summary:\n%s", plan.Docs.Plan)
	}
	if strings.Contains(plan.VisibleDocs.Plan, "后台导演私密") {
		t.Fatalf("visible docs must exclude private section:\n%s", plan.VisibleDocs.Plan)
	}
	paths := store.DirectorPlanAllowedPaths(story.ID, "main")
	if len(paths) != 1 || !strings.HasSuffix(paths[0], "director.md") {
		t.Fatalf("director should expose only director.md path: %#v", paths)
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
	if !strings.Contains(plan.Docs.Plan, "分支说明") || !strings.Contains(plan.Docs.Plan, branch.ID) {
		t.Fatalf("branch plan should carry branch note:\n%s", plan.Docs.Plan)
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
	nextDocs.Plan += "\n\n可见安排：给玩家观察、对话、调查三个方向。"
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
	badDocs.Plan = "# 缺标题"
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
	docs.Plan += "\n\n手动保存：用户调整当前事件。"
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
		Plan: ExtractDirectorPlanVisibleSection(docs.Plan),
	}}
	context := DirectorPlanVisibleContext(plan, 4096)
	if !strings.Contains(context, "正文Agent可读") || !strings.Contains(context, "导演规划") {
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
	request := sampleTurnCheckRequest()
	request.Action = "强闯禁制"
	request.Intent = "冒险"
	request.Challenge = "穿过即将崩塌的主线入口"
	request.Cost = "失败会导致主线入口崩塌"
	request.State = "禁制已经濒临失控。"
	seed := seedForTurnCheckOutcome(t, "normal", "normal", 0, "critical_failure")
	resolution, err := resolveTurnRulesWithSeed(story.ID, "main", initialStoryState(), request, seed)
	if err != nil {
		t.Fatal(err)
	}
	terminal := &TerminalOutcome{
		Terminal:         true,
		Type:             "mainline_failed",
		Reason:           "主线入口崩塌。",
		RuleResolutionID: resolution.ID,
	}
	turn, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID:        "main",
		User:            request.Action,
		Narrative:       "禁制炸裂，入口坍塌。",
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
	request := sampleTurnCheckRequest()
	request.Action = "冲刺"
	request.Intent = "冒险"
	request.Challenge = "冲过即将关闭的门"
	request.Cost = "失败会浪费体力"
	request.State = "体力仍可支撑一次短距离冲刺。"
	resolution, err := ResolveTurnRules(story.ID, "main", snapshot.State, request)
	if err != nil {
		t.Fatal(err)
	}
	turn, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID:       "main",
		User:           request.Action,
		Narrative:      "他冲了出去。",
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
	if reroll.Seed == resolution.Seed {
		t.Fatalf("reroll should use a new seed: old=%d new=%d", resolution.Seed, reroll.Seed)
	}
	snapshot, err = store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.CurrentTurn == nil || snapshot.CurrentTurn.RuleResolution == nil || snapshot.CurrentTurn.RuleResolution.ID != reroll.ID {
		t.Fatalf("rerolled resolution should be persisted: %#v", snapshot.CurrentTurn)
	}
}
