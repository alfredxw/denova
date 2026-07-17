package interactive

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDirectorEventCadenceIntervals(t *testing.T) {
	turns := make([]TurnEvent, 6)
	for i := range turns {
		turns[i].ID = "turn-" + string(rune('1'+i))
	}
	for _, test := range []struct {
		frequency string
		dueAt     int
	}{
		{frequency: EventFrequencySparse, dueAt: 6},
		{frequency: EventFrequencyBalanced, dueAt: 4},
		{frequency: EventFrequencyFrequent, dueAt: 2},
	} {
		before := directorEventOpportunity(DirectorEventRuntime{}, turns[:test.dueAt-1], test.frequency, true, false)
		due := directorEventOpportunity(DirectorEventRuntime{}, turns[:test.dueAt], test.frequency, true, false)
		if before.Due || !due.Due || due.Kind != "new" {
			t.Fatalf("unexpected cadence for %s: before=%#v due=%#v", test.frequency, before, due)
		}
	}
	if opportunity := directorEventOpportunity(DirectorEventRuntime{}, turns, EventFrequencyOff, true, false); opportunity.Due {
		t.Fatalf("off frequency should not create opportunities: %#v", opportunity)
	}
}

func TestDirectorEventDecisionLifecycleValidation(t *testing.T) {
	turns := []TurnEvent{{ID: "turn-1"}, {ID: "turn-2"}, {ID: "turn-3"}, {ID: "turn-4"}}
	catalog := []DirectorEvent{{ID: "pack/card", Name: "事件卡", Enabled: true}}
	newOpportunity := EventOpportunity{Due: true, Kind: "new"}
	runtime, err := applyDirectorEventDecision(DirectorEventRuntime{}, &EventDecision{Mode: EventDecisionNone}, newOpportunity, "turn-4", turns, catalog)
	if err != nil || runtime.Active != nil || runtime.LastOpportunityTurnID != "turn-4" {
		t.Fatalf("none should consume the opportunity without activating an event: runtime=%#v err=%v", runtime, err)
	}
	runtime, err = applyDirectorEventDecision(DirectorEventRuntime{}, &EventDecision{Mode: EventDecisionSeed, EventRef: "pack/card", Summary: "开始铺垫"}, newOpportunity, "turn-4", turns, catalog)
	if err != nil || runtime.Active == nil || runtime.Active.Stage != EventDecisionSeed {
		t.Fatalf("seed should activate an explicitly selected card: runtime=%#v err=%v", runtime, err)
	}
	activeOpportunity := EventOpportunity{Due: true, Kind: "active", ActiveEventRef: "pack/card"}
	if _, err := applyDirectorEventDecision(runtime, &EventDecision{Mode: EventDecisionSeed, EventRef: "pack/card"}, activeOpportunity, "turn-4", turns, catalog); err == nil {
		t.Fatal("an active event must prevent seeding a second event")
	}
	if _, err := applyDirectorEventDecision(runtime, &EventDecision{Mode: EventDecisionAdvance, EventRef: "pack/card"}, activeOpportunity, "turn-4", turns, catalog); err == nil {
		t.Fatal("advance must require evidence from the active turn path")
	}
	advanced, err := applyDirectorEventDecision(runtime, &EventDecision{Mode: EventDecisionAdvance, EventRef: "pack/card", Summary: "冲突升级", EvidenceTurnIDs: []string{"turn-4"}}, activeOpportunity, "turn-4b", append(turns, TurnEvent{ID: "turn-4b"}), catalog)
	if err != nil || advanced.Active == nil || advanced.Active.Stage != EventDecisionAdvance {
		t.Fatalf("advance with valid evidence should update the active event: runtime=%#v err=%v", advanced, err)
	}
	resolved, err := applyDirectorEventDecision(advanced, &EventDecision{Mode: EventDecisionResolve, EventRef: "pack/card", EvidenceTurnIDs: []string{"turn-4b"}}, activeOpportunity, "turn-5", append(turns, TurnEvent{ID: "turn-4b"}, TurnEvent{ID: "turn-5"}), catalog)
	if err != nil || resolved.Active != nil {
		t.Fatalf("resolve should close the active event: runtime=%#v err=%v", resolved, err)
	}
}

func TestDirectorEventRuntimeIsIdempotentBranchScopedAndRewindSafe(t *testing.T) {
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{Title: "事件运行态", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	turns := make([]TurnEvent, 0, 4)
	for i := 0; i < 4; i++ {
		turn, _, appendErr := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{BranchID: "main", User: "行动", Narrative: "结果"})
		if appendErr != nil {
			t.Fatal(appendErr)
		}
		turns = append(turns, turn)
	}
	token, err := store.DirectorPlanRunToken(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.MarkDirectorPlanRunStarted(story.ID, "main", token, turns[3].ID); err != nil {
		t.Fatal(err)
	}
	status, err := store.DirectorPlanStatus(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if !status.EventOpportunity.Due || status.EventOpportunity.Kind != "new" {
		t.Fatalf("fourth balanced turn should create a new opportunity: %#v", status.EventOpportunity)
	}
	catalog := DirectorEventCatalogFromStoryDirector(DefaultStoryDirector())
	if len(catalog) == 0 {
		t.Fatal("default director should explicitly select at least one event card")
	}
	decision := PlanDecision{Mode: PlanDecisionKeep, EventDecision: &EventDecision{Mode: EventDecisionSeed, EventRef: catalog[0].ID, Summary: "事件已埋设"}}
	output, _ := json.Marshal(decision)
	completed, err := store.CompleteDirectorPlanRun(story.ID, "main", token, turns[3].ID, string(output))
	if err != nil {
		t.Fatal(err)
	}
	if completed.Metadata.EventRuntime.Active == nil || len(completed.Metadata.EventRuntime.RecentDecisions) != 1 {
		t.Fatalf("seed should create one active event record: %#v", completed.Metadata.EventRuntime)
	}
	if _, err := store.CompleteDirectorPlanRun(story.ID, "main", token, turns[3].ID, string(output)); err != nil {
		t.Fatal(err)
	}
	retried, err := store.DirectorPlan(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(retried.Metadata.EventRuntime.RecentDecisions) != 1 {
		t.Fatalf("retry must be idempotent: %#v", retried.Metadata.EventRuntime)
	}
	branch, err := store.CreateBranch(story.ID, CreateBranchRequest{ParentEventID: turns[3].ID, Title: "事件分支"})
	if err != nil {
		t.Fatal(err)
	}
	branchPlan, err := store.DirectorPlan(story.ID, branch.ID)
	if err != nil {
		t.Fatal(err)
	}
	if branchPlan.Metadata.EventRuntime.Active == nil {
		t.Fatalf("branch should inherit event runtime valid at its fork point: %#v", branchPlan.Metadata.EventRuntime)
	}
	rebuilt, err := store.RebuildDirectorPlan(story.ID, RebuildDirectorPlanRequest{BranchID: branch.ID}, DirectorPlanSeed{Templates: DefaultStoryDirectorPlanningTemplates()})
	if err != nil {
		t.Fatal(err)
	}
	if rebuilt.Metadata.EventRuntime.Active == nil {
		t.Fatalf("rebuild should preserve valid event runtime by default: %#v", rebuilt.Metadata.EventRuntime)
	}
	reset, err := store.RebuildDirectorPlan(story.ID, RebuildDirectorPlanRequest{BranchID: branch.ID, ResetEvents: true}, DirectorPlanSeed{Templates: DefaultStoryDirectorPlanningTemplates()})
	if err != nil {
		t.Fatal(err)
	}
	if reset.Metadata.EventRuntime.Active != nil || len(reset.Metadata.EventRuntime.RecentDecisions) != 0 {
		t.Fatalf("explicit rebuild reset should clear event runtime: %#v", reset.Metadata.EventRuntime)
	}
	if err := store.RewindToTurnParent(story.ID, RewindTurnRequest{BranchID: "main", TurnID: turns[3].ID}); err != nil {
		t.Fatal(err)
	}
	rewound, err := store.DirectorPlan(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if rewound.Metadata.EventRuntime.Active != nil || len(rewound.Metadata.EventRuntime.RecentDecisions) != 0 {
		t.Fatalf("rewind should remove decisions whose evidence turn left the active path: %#v", rewound.Metadata.EventRuntime)
	}
	docs := rewound.Docs
	docs.Plan += "\n\n手动更新：回退后的路线。"
	updated, err := store.UpdateDirectorPlan(story.ID, UpdateDirectorPlanRequest{BranchID: "main", Docs: docs, BaseRevision: rewound.Metadata.Revision})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Metadata.EventRuntime.Active != nil || len(updated.Metadata.EventRuntime.RecentDecisions) != 0 {
		t.Fatalf("manual update must persist the rewind-reconciled runtime: %#v", updated.Metadata.EventRuntime)
	}
}

func TestDirectorPlanIgnoresOutOfOrderCompletion(t *testing.T) {
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{Title: "并发导演", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	first, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{BranchID: "main", User: "第一步", Narrative: "第一步结果"})
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{BranchID: "main", User: "第二步", Narrative: "第二步结果"})
	if err != nil {
		t.Fatal(err)
	}
	token, err := store.DirectorPlanRunToken(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.MarkDirectorPlanRunStarted(story.ID, "main", token, first.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.MarkDirectorPlanRunStarted(story.ID, "main", token, second.ID); err != nil {
		t.Fatal(err)
	}
	output, _ := json.Marshal(PlanDecision{Mode: PlanDecisionKeep})
	completed, err := store.CompleteDirectorPlanRun(story.ID, "main", token, first.ID, string(output))
	if err != nil {
		t.Fatal(err)
	}
	if completed.Metadata.LastRun == nil || completed.Metadata.LastRun.Status != DirectorPlanStatusRunning || completed.Metadata.LastRun.SourceTurnID != second.ID {
		t.Fatalf("older completion must not replace the newer run: %#v", completed.Metadata.LastRun)
	}
}

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
	if err := validateDirectorPlanDoc(DirectorPlanDocAgentBrief, plan.Docs.AgentBrief); err != nil {
		t.Fatalf("agent brief should be valid: %v\n%s", err, plan.Docs.AgentBrief)
	}
	if !strings.Contains(plan.Docs.Plan, "隐脉觉醒") {
		t.Fatalf("director plan should include opening summary:\n%s", plan.Docs.Plan)
	}
	if plan.VisibleDocs.AgentBrief != strings.TrimSpace(plan.Docs.AgentBrief) || strings.Contains(plan.VisibleDocs.AgentBrief, "阶段目标与隐藏钩子") {
		t.Fatalf("visible docs must expose only agent-brief.md:\n%s", plan.VisibleDocs.AgentBrief)
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
	completed, err := store.CompleteDirectorPlanRun(story.ID, "main", token, "turn_1", `{"mode":"patch","reason":"后台导演尝试完成"}`)
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
		AgentBrief: docs.AgentBrief + "\n" + strings.Repeat("可见线索", 2000),
	}}
	context := DirectorPlanVisibleContext(plan, 4096)
	if len(context) > 4096 {
		t.Fatalf("visible context exceeded caller budget: bytes=%d", len(context))
	}
	if !strings.Contains(context, "正文 Agent 简报") || !strings.Contains(context, "当前目标与可见钩子") {
		t.Fatalf("visible context should include public sections:\n%s", context)
	}
	if strings.Contains(context, "阶段目标与隐藏钩子") || strings.Contains(context, "隐藏状态不可泄露") {
		t.Fatalf("visible context should exclude private sections:\n%s", context)
	}
}

func TestDirectorPlanAcceptsLargeVisibleSections(t *testing.T) {
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{Title: "大纲规划", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := store.DirectorPlan(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	visibleMarker := strings.Repeat("可见线索", 3600)
	privateMarker := "隐藏真相不可泄露"
	docs := plan.Docs
	docs.AgentBrief = strings.Replace(docs.AgentBrief, "说明主角当前最想解决的问题、可见收益、未解谜团和这一回合应延续的推进动力。", visibleMarker, 1)
	docs.Plan = strings.Replace(docs.Plan, "维护当前阶段的隐藏真相、阶段高潮、反转条件与阅读钩子投放顺序，保证每个可玩回合都能产生有效推进。", privateMarker, 1)
	if len([]byte(docs.AgentBrief)) <= 32*1024 || len([]byte(docs.AgentBrief)) >= 64*1024 {
		t.Fatalf("test agent brief size should sit between old and new limits, got %d", len([]byte(docs.AgentBrief)))
	}
	updated, err := store.UpdateDirectorPlan(story.ID, UpdateDirectorPlanRequest{
		BranchID:     "main",
		Docs:         docs,
		BaseRevision: plan.Metadata.Revision,
		Source:       "test_large_plan",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(updated.VisibleDocs.AgentBrief, visibleMarker) {
		t.Fatalf("visible agent brief should keep large public section, visible bytes=%d", len([]byte(updated.VisibleDocs.AgentBrief)))
	}
	if strings.Contains(updated.VisibleDocs.AgentBrief, privateMarker) {
		t.Fatalf("visible agent brief should exclude private plan")
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
	seed := seedForTurnCheckOutcome(t, "1d20", "normal", "normal", 0, 0, "critical_failure")
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
