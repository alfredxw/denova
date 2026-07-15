package interactive

import (
	"strings"
	"testing"
	"time"
)

func TestAppendTurnWithStatePersistsTurnResultAndActorStateAtomically(t *testing.T) {
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{
		Title:         "青冥试炼",
		Origin:        "林风进入外门",
		StoryTellerID: "classic",
	})
	if err != nil {
		t.Fatal(err)
	}

	turn, delta, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID:  "main",
		User:      "我接受苏灿灿的帮助",
		Narrative: "苏灿灿替林风处理了掌心灼伤，并答应继续调查青冥灵根。",
		TurnResult: &TurnResult{
			StateUpdates: []StateUpdate{{
				Op:   TurnStateUpdateCreate,
				Path: "/protagonist",
				Value: map[string]any{
					"template_id": "protagonist",
					"name":        "林风",
					"state":       map[string]any{"当前身体状态": "掌心灼伤缓解，体力恢复"},
				},
			}},
			Choices: testTurnChoices(),
			DirectorUpdate: &DirectorUpdateHint{
				Needed: true,
				Reason: "主角接受关键角色帮助，公开关系与阶段调查方向发生变化",
			},
		},
	})
	if err != nil {
		t.Fatalf("AppendTurnWithState failed: %v", err)
	}
	if turn.TurnResult == nil || len(turn.TurnResult.StateUpdates) != 1 || turn.TurnResult.StateUpdates[0].Path != "/protagonist" || turn.TurnResult.DirectorUpdate == nil || !turn.TurnResult.DirectorUpdate.Needed {
		t.Fatalf("turn result not persisted: %#v", turn.TurnResult)
	}
	if delta == nil || turn.StateDelta == nil || len(turn.StateDelta.ActorOps) == 0 {
		t.Fatalf("expected atomic state delta: turn=%#v delta=%#v", turn.StateDelta, delta)
	}
	foundBodyStatus := false
	for _, op := range turn.StateDelta.ActorOps {
		if op.SourceKind != StateOpSourceTurnResult || op.SourceID != turn.ID || op.SourceTurnID != turn.ID {
			t.Fatalf("turn result state op source mismatch: %#v", op)
		}
		if op.ActorID == "protagonist" && op.FieldID == "当前身体状态" {
			foundBodyStatus = true
		}
	}
	if !foundBodyStatus {
		t.Fatalf("body status op missing: %#v", turn.StateDelta.ActorOps)
	}
	if turn.StateStatus != "ready" {
		t.Fatalf("turn state status mismatch: %q", turn.StateStatus)
	}
	if turn.HotState != nil || len(turn.TurnResult.Choices) != DefaultStoryChoiceCount {
		t.Fatalf("new turn choices should exist only in turn result: turn_result=%#v hot_state=%#v", turn.TurnResult, turn.HotState)
	}

	snapshot, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if got := actorStateFieldValue(snapshot.State, "protagonist", "当前身体状态"); got != "掌心灼伤缓解，体力恢复" {
		t.Fatalf("body status = %#v", got)
	}
}

func TestValidateTurnResultRequiresConfiguredChoices(t *testing.T) {
	base := TurnResult{StateUpdates: []StateUpdate{}}
	if err := ValidateTurnResult(base); err == nil {
		t.Fatal("a non-terminal turn may not omit choices")
	}
	if err := validateTerminalTurnResult(base, DefaultStoryChoiceCount); err != nil {
		t.Fatalf("a declared terminal turn may use empty choices: %v", err)
	}
	base.Choices = []string{"只有一个"}
	if err := ValidateTurnResult(base); err == nil {
		t.Fatal("one choice should fail the default count")
	}
	base.Choices = []string{"推开门", "检查窗户"}
	if err := ValidateTurnResult(base); err == nil {
		t.Fatal("two choices should fail the default count")
	}
	base.Choices = testTurnChoices()
	if err := ValidateTurnResult(base); err != nil {
		t.Fatalf("five choices should pass: %v", err)
	}
	if err := validateTerminalTurnResult(base, DefaultStoryChoiceCount); err == nil {
		t.Fatal("a declared terminal turn must not expose follow-up choices")
	}
	base.Choices = []string{"左", "中", "右"}
	if err := ValidateTurnResult(base, 3); err != nil {
		t.Fatalf("configured choice count should pass: %v", err)
	}
}

func TestNormalizeTurnResultKeepsDistinctChoices(t *testing.T) {
	result := NormalizeTurnResult(TurnResult{Choices: []string{" Ａ ", "a", "B", "C", "D", "E"}})
	if got := result.Choices; len(got) != 5 || strings.Join(got, ",") != "Ａ,B,C,D,E" {
		t.Fatalf("normalized choices = %#v", got)
	}
}

func TestNormalizeTurnResultOmitsRoutineDirectorHintAndValidatesMaterialReason(t *testing.T) {
	routine := NormalizeTurnResult(TurnResult{DirectorUpdate: &DirectorUpdateHint{Needed: false, Reason: "普通承接"}})
	if routine.DirectorUpdate != nil {
		t.Fatalf("needed=false should normalize to omission: %#v", routine.DirectorUpdate)
	}
	material := TurnResult{Choices: testTurnChoices(), DirectorUpdate: &DirectorUpdateHint{Needed: true}}
	if err := ValidateTurnResult(material); err == nil || !strings.Contains(err.Error(), "reason") {
		t.Fatalf("material Director hint without evidence reason should fail: %v", err)
	}
	material.DirectorUpdate.Reason = "阶段目标已经完成"
	if err := ValidateTurnResult(material); err != nil {
		t.Fatalf("bounded material Director hint should pass: %v", err)
	}
}

func TestAppendTurnWithStateUsesStoryChoiceCount(t *testing.T) {
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{Title: "七个选项", ChoiceCount: 7})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		User: "前进", Narrative: "前方出现岔路。",
		TurnResult: &TurnResult{StateUpdates: []StateUpdate{}, Choices: testTurnChoices()},
	}); err == nil {
		t.Fatal("default five choices should fail a story configured for seven")
	}
	choices := append(testTurnChoices(), "返回营地", "独自探路")
	turn, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		User: "前进", Narrative: "前方出现岔路。",
		TurnResult: &TurnResult{StateUpdates: []StateUpdate{}, Choices: choices},
	})
	if err != nil {
		t.Fatal(err)
	}
	if turn.TurnResult == nil || len(turn.TurnResult.Choices) != 7 {
		t.Fatalf("custom choices were not persisted: %#v", turn.TurnResult)
	}
}

func TestSnapshotRestoresLegacyHotChoicesAsReadOnlyFallback(t *testing.T) {
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{Title: "旧快捷选项", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	turn, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID:  "main",
		User:      "我推开门",
		Narrative: "门外传来脚步声。",
	})
	if err != nil {
		t.Fatal(err)
	}

	store.mu.Lock()
	meta, lines, err := store.readStoryLocked(story.ID)
	if err == nil {
		now := time.Now().UTC().Format(time.RFC3339Nano)
		err = store.rewriteStoryLocked(story.ID, meta, lines, HotChoicesEvent{
			V:        schemaVersion,
			Type:     StoryEventTypeHotChoices,
			ID:       "legacy-hot-choices",
			ParentID: turn.ID,
			BranchID: "main",
			Ts:       now,
			Choices:  []string{"沿墙观察", "询问守夜人"},
		})
	}
	store.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}

	snapshot, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.CurrentTurn == nil || snapshot.CurrentTurn.TurnResult != nil || snapshot.CurrentTurn.HotState == nil || len(snapshot.CurrentTurn.HotState.Choices) != 2 {
		t.Fatalf("legacy choices should remain readable without creating a TurnResult: %#v", snapshot.CurrentTurn)
	}
}

func TestAppendTurnWithStateRejectsStaleExpectedParent(t *testing.T) {
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{Title: "分支并发", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	base := ""
	first, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID:         "main",
		ExpectedParentID: &base,
		User:             "先行动",
		Narrative:        "第一回合完成。",
		TurnResult:       &TurnResult{StateUpdates: []StateUpdate{}, Choices: testTurnChoices()},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID:         "main",
		ExpectedParentID: &base,
		User:             "迟到行动",
		Narrative:        "不应写入。",
		TurnResult:       &TurnResult{StateUpdates: []StateUpdate{}, Choices: testTurnChoices()},
	})
	if err == nil || !strings.Contains(err.Error(), "分支已前进") {
		t.Fatalf("expected stale parent rejection after %s, got %v", first.ID, err)
	}
}

func TestAppendStateDeltaRejectsNonHeadTurn(t *testing.T) {
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{Title: "迟到状态", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	first, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID:   "main",
		User:       "第一步",
		Narrative:  "第一回合。",
		TurnResult: &TurnResult{StateUpdates: []StateUpdate{}, Choices: testTurnChoices()},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID:   "main",
		User:       "第二步",
		Narrative:  "第二回合。",
		TurnResult: &TurnResult{StateUpdates: []StateUpdate{}, Choices: testTurnChoices()},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.AppendStateDelta(story.ID, AppendStateDeltaRequest{
		ParentID: first.ID,
		BranchID: "main",
		Ops:      []StateOp{{Op: "set", Path: "scene.late", Value: true}},
	})
	if err == nil || !strings.Contains(err.Error(), "不是当前分支头") {
		t.Fatalf("expected non-head state rejection, got %v", err)
	}
}

func testTurnChoices() []string {
	return []string{"继续行动", "观察环境", "询问同伴", "检查状态", "暂时等待"}
}
