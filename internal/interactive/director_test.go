package interactive

import (
	"os"
	"strings"
	"testing"
)

func TestDirectorEventActionsPersistAuditWithoutMovingBranchHead(t *testing.T) {
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{Title: "学院逆袭", StoryTellerID: "classic"})
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

	state, err := store.ForceDirectorEvent(story.ID, "face_slap", DirectorEventActionRequest{
		Reason: "安排公开打脸",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !stringInList("face_slap", state.ForcedEvents) || stringInList("face_slap", state.DisabledEvents) {
		t.Fatalf("force event lists mismatch: %#v", state)
	}
	if len(state.EventQueue) == 0 || state.EventQueue[0].ID != "face_slap" || state.EventQueue[0].Status != "forced" {
		t.Fatalf("forced event should be queued: %#v", state.EventQueue)
	}
	snapshot, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.CurrentTurn == nil || snapshot.CurrentTurn.ID != turn.ID {
		t.Fatalf("director patch must not move branch head: %#v", snapshot.CurrentTurn)
	}

	state, err = store.DisableDirectorEvent(story.ID, "face_slap", DirectorEventActionRequest{
		Reason: "用户暂时不想打脸",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !stringInList("face_slap", state.DisabledEvents) || stringInList("face_slap", state.ForcedEvents) {
		t.Fatalf("disable event lists mismatch: %#v", state)
	}
	if len(state.EventQueue) == 0 || state.EventQueue[0].Status != "disabled" || state.EventQueue[0].Enabled {
		t.Fatalf("disabled event should remain audited as disabled: %#v", state.EventQueue)
	}
	data, err := os.ReadFile(store.storyPath(story.ID))
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(string(data), `"type":"director_patch"`); got != 2 {
		t.Fatalf("expected two director patch audit rows, got %d\n%s", got, string(data))
	}
}

func TestCreateStoryUsesProvidedDirectorState(t *testing.T) {
	store := NewStore(t.TempDir())
	seed := DefaultDirectorState()
	seed.MainArc = "复仇主线"
	seed.EventQueue = []DirectorEvent{directorEventForAction("revenge", nil)}
	story, err := store.CreateStory(CreateStoryRequest{
		Title:         "复仇故事",
		StoryTellerID: "classic",
		DirectorState: &seed,
	})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.DirectorState.MainArc != "复仇主线" || !directorEventQueued(snapshot.DirectorState.EventQueue, "revenge") {
		t.Fatalf("story should use provided director state: %#v", snapshot.DirectorState)
	}
}

func TestDirectorStateContextSummaryIncludesEventCardMarkdown(t *testing.T) {
	state := DefaultDirectorState()
	state.EventQueue = []DirectorEvent{{
		ID:            "academy_trial",
		Name:          "外门考核打脸",
		Category:      "学院",
		Status:        "available",
		Enabled:       true,
		Summary:       "外门考核压力",
		PublicSummary: "外门考核压力",
		Template: "## 触发场景\n主角在外门考核被同门质疑。\n\n" +
			"## 背景融合方式\n绑定外门名额、执事偏见和残卷线索。\n\n" +
			"## 事件回收 / 后果\n考核结束后以榜单和戒律回收。",
	}}
	summary := DirectorStateContextSummary(state, "main", 2048)
	for _, want := range []string{"外门考核打脸", "事件卡", "触发场景", "背景融合方式", "残卷线索", "事件回收"} {
		if !strings.Contains(summary, want) {
			t.Fatalf("director summary should include %q:\n%s", want, summary)
		}
	}
	if len([]byte(summary)) > 2048 {
		t.Fatalf("director summary should stay bounded, got %d bytes", len([]byte(summary)))
	}
}

func TestDirectorStateInteractiveContextSummaryOmitsEventCardQueue(t *testing.T) {
	state := DefaultDirectorState()
	state.MainArc = "外门逆袭"
	state.StagePlan = "下一阶段制造公开压力，并让玩家行动决定是否正面迎击。"
	state.BeatQueue = []DirectorBeat{{
		ID:       "beat_trial_pressure",
		Summary:  "公开压力升高",
		Pressure: "同门质疑",
		Payoff:   "玩家可以选择反证、迂回或调查。",
		Status:   "planned",
	}}
	state.Foreshadowing = []DirectorThread{{
		ID:      "thread_scroll",
		Title:   "残卷线索",
		Status:  "open",
		Summary: "残卷来源尚未公开。",
	}}
	state.EventQueue = []DirectorEvent{{
		ID:            "academy_trial",
		Name:          "外门考核打脸",
		Category:      "学院",
		Status:        "available",
		Enabled:       true,
		Summary:       "外门考核压力",
		PublicSummary: "外门考核压力",
		Template: "## 触发场景\n主角在外门考核被同门质疑。\n\n" +
			"## 背景融合方式\n绑定外门名额、执事偏见和残卷线索。\n\n" +
			"## 事件回收 / 后果\n考核结束后以榜单和戒律回收。",
	}}

	summary := DirectorStateInteractiveContextSummary(state, "main", 2048)
	for _, want := range []string{"外门逆袭", "公开压力", "同门质疑", "残卷线索"} {
		if !strings.Contains(summary, want) {
			t.Fatalf("interactive director summary should include translated plan %q:\n%s", want, summary)
		}
	}
	for _, forbidden := range []string{"事件 1", "外门考核打脸", "事件卡", "触发场景", "背景融合方式", "事件回收"} {
		if strings.Contains(summary, forbidden) {
			t.Fatalf("interactive director summary should not expose raw event cards %q:\n%s", forbidden, summary)
		}
	}
	if len([]byte(summary)) > 2048 {
		t.Fatalf("interactive director summary should stay bounded, got %d bytes", len([]byte(summary)))
	}
}

func TestAppendTurnWithBriefAutoUpdatesDirectorState(t *testing.T) {
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{
		Title:         "自动导演",
		Origin:        "主角被同门轻视",
		StoryTellerID: "classic",
	})
	if err != nil {
		t.Fatal(err)
	}
	brief := TurnBrief{
		UserAction:       "我报名参加学院比拼",
		Intent:           "冒险",
		TurnGoal:         "建立学院比拼压力",
		Pressure:         "同门当众质疑主角资格",
		EventIntents:     []string{"打脸", "比拼"},
		StateExpectation: "下一回合进入公开比试前的准备",
		ContinuityNotes:  "同门已经公开质疑主角。",
	}
	resolution, err := ResolveTurnRules(story.ID, "main", initialStoryState(), brief)
	if err != nil {
		t.Fatal(err)
	}
	turn, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID:       "main",
		User:           brief.UserAction,
		Narrative:      "报名册上落下主角的名字，周围瞬间安静。",
		TurnBrief:      &brief,
		RuleResolution: &resolution,
	})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.CurrentTurn == nil || snapshot.CurrentTurn.ID != turn.ID {
		t.Fatalf("director patch must not move branch head: %#v", snapshot.CurrentTurn)
	}
	if snapshot.DirectorState.LastDirectorRun == nil || snapshot.DirectorState.LastDirectorRun.Status != "ready" {
		t.Fatalf("director run should be ready: %#v", snapshot.DirectorState.LastDirectorRun)
	}
	if !strings.Contains(snapshot.DirectorState.StagePlan, "公开比试") {
		t.Fatalf("stage plan should follow brief expectation: %s", snapshot.DirectorState.StagePlan)
	}
	if len(snapshot.DirectorState.BeatQueue) == 0 || !strings.Contains(snapshot.DirectorState.BeatQueue[0].Pressure, "质疑") {
		t.Fatalf("beat queue should be based on brief pressure: %#v", snapshot.DirectorState.BeatQueue)
	}
	if !directorEventQueued(snapshot.DirectorState.EventQueue, "face_slap") || !directorEventQueued(snapshot.DirectorState.EventQueue, "contest") {
		t.Fatalf("event intents should update director event queue: %#v", snapshot.DirectorState.EventQueue)
	}
	faceSlap := directorEventByID(snapshot.DirectorState.EventQueue, "face_slap")
	if faceSlap.LastTriggeredTurnID != turn.ID || faceSlap.NextEligibleAfterTurns != faceSlap.CooldownTurns {
		t.Fatalf("event cooldown audit mismatch: %#v", faceSlap)
	}
	data, err := os.ReadFile(store.storyPath(story.ID))
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(string(data), `"type":"director_patch"`); got != 1 {
		t.Fatalf("expected one automatic director patch, got %d\n%s", got, string(data))
	}
}

func TestRebuildDirectorStateCreatesPlanAndBranchPatch(t *testing.T) {
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{
		Title:         "废柴逆袭",
		Origin:        "主角被同门轻视，却握有残卷线索。",
		StoryTellerID: "classic",
	})
	if err != nil {
		t.Fatal(err)
	}
	turn, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID:  "main",
		User:      "我决定参加外门比试",
		Narrative: "报名钟前，执事冷眼扫来。",
	})
	if err != nil {
		t.Fatal(err)
	}
	branch, err := store.CreateBranch(story.ID, CreateBranchRequest{
		ParentEventID: turn.ID,
		Title:         "暗中修炼",
	})
	if err != nil {
		t.Fatal(err)
	}

	state, err := store.RebuildDirectorState(story.ID, RebuildDirectorStateRequest{BranchID: branch.ID})
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(state.MainArc) == "" || strings.TrimSpace(state.StagePlan) == "" {
		t.Fatalf("rebuilt state should contain main arc and stage plan: %#v", state)
	}
	if len(state.BeatQueue) != 3 {
		t.Fatalf("rebuilt state should contain three beats: %#v", state.BeatQueue)
	}
	if len(state.EventQueue) == 0 {
		t.Fatalf("rebuilt state should seed event queue")
	}
	if !strings.Contains(state.StagePlan, "我决定参加外门比试") {
		t.Fatalf("stage plan should be based on latest turn: %s", state.StagePlan)
	}
	if strings.TrimSpace(state.BranchPatches[branch.ID]) == "" {
		t.Fatalf("branch patch should be recorded: %#v", state.BranchPatches)
	}
	if state.LastDirectorRun == nil || state.LastDirectorRun.Status != "ready" {
		t.Fatalf("director run status mismatch: %#v", state.LastDirectorRun)
	}
	snapshot, err := store.Snapshot(story.ID, branch.ID)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.CurrentTurn == nil || snapshot.CurrentTurn.ID != turn.ID {
		t.Fatalf("rebuild patch must not move branch head: %#v", snapshot.CurrentTurn)
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

func directorEventQueued(events []DirectorEvent, id string) bool {
	return directorEventByID(events, id).ID != ""
}

func directorEventByID(events []DirectorEvent, id string) DirectorEvent {
	for _, event := range events {
		if event.ID == id {
			return event
		}
	}
	return DirectorEvent{}
}
