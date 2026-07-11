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
			Contract: TurnContract{
				PlayerIntent: "接受帮助并调查灵根",
				SceneGoal:    "确认伤势来源",
				ChoiceAxes:   []string{"追问典籍来源", "检查掌心变化"},
			},
			ActorStatePatches: []ActorStatePatch{{
				ActorID:    "protagonist",
				TemplateID: "protagonist",
				State:      map[string]any{"当前身体状态": "掌心灼伤缓解，体力恢复"},
				Reason:     "接受治疗后体力恢复",
			}},
			FactCandidates: []StoryFactCandidate{{
				Kind:       "relationship",
				Subject:    "苏灿灿",
				Fact:       "确认林风的伤势异常并决定继续帮助调查",
				Visibility: "player_known",
				Importance: "high",
			}},
			SceneResult: TurnSceneResult{Status: "continued", Summary: "丹堂调查继续"},
			PlanSignals: TurnPlanSignals{DeviationLevel: "none"},
			Choices:     []string{"追问典籍来源", "检查掌心变化"},
		},
	})
	if err != nil {
		t.Fatalf("AppendTurnWithState failed: %v", err)
	}
	if turn.TurnResult == nil || turn.TurnResult.Contract.SceneGoal != "确认伤势来源" {
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
	if turn.StateStatus != "ready" || turn.MemoryStatus != "pending" {
		t.Fatalf("turn phase status mismatch: state=%q memory=%q", turn.StateStatus, turn.MemoryStatus)
	}
	if turn.HotState != nil || len(turn.TurnResult.Choices) != 2 {
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

func TestValidateTurnResultRequiresChoicesExceptAtTerminal(t *testing.T) {
	base := TurnResult{Contract: TurnContract{PlayerIntent: "前进", SceneGoal: "离开大厅"}}
	if err := ValidateTurnResult(base); err == nil || !strings.Contains(err.Error(), "2 到 4") {
		t.Fatalf("non-terminal turn without choices should fail, err=%v", err)
	}
	base.Choices = []string{"只有一个"}
	if err := ValidateTurnResult(base); err == nil {
		t.Fatal("non-terminal turn with one choice should fail")
	}
	base.Choices = []string{"推开门", "检查窗户"}
	if err := ValidateTurnResult(base); err != nil {
		t.Fatalf("two choices should pass: %v", err)
	}
	base.SceneResult.Status = "terminal"
	base.Choices = nil
	if err := ValidateTurnResult(base); err != nil {
		t.Fatalf("terminal turn may omit choices: %v", err)
	}
}

func TestNormalizeTurnResultKeepsAtMostFourUniqueChoices(t *testing.T) {
	result := NormalizeTurnResult(TurnResult{Choices: []string{" A ", "A", "B", "C", "D", "E"}})
	if got := result.Choices; len(got) != 4 || strings.Join(got, ",") != "A,B,C,D" {
		t.Fatalf("normalized choices = %#v", got)
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
		TurnResult:       &TurnResult{Contract: TurnContract{PlayerIntent: "先行动"}, Choices: []string{"继续行动", "观察环境"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID:         "main",
		ExpectedParentID: &base,
		User:             "迟到行动",
		Narrative:        "不应写入。",
		TurnResult:       &TurnResult{Contract: TurnContract{PlayerIntent: "迟到行动"}, Choices: []string{"继续行动", "观察环境"}},
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
		TurnResult: &TurnResult{Contract: TurnContract{PlayerIntent: "第一步"}, Choices: []string{"继续行动", "观察环境"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID:   "main",
		User:       "第二步",
		Narrative:  "第二回合。",
		TurnResult: &TurnResult{Contract: TurnContract{PlayerIntent: "第二步"}, Choices: []string{"继续行动", "观察环境"}},
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
