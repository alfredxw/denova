package interactive

import "testing"

func TestClaimInteractiveMemoryRunIsIdempotentAndIndependentFromState(t *testing.T) {
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{Title: "记忆认领", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	turn, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID:   "main",
		User:       "继续",
		Narrative:  "剧情继续。",
		TurnResult: &TurnResult{Contract: TurnContract{PlayerIntent: "继续"}, Choices: []string{"继续向前", "观察四周"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := store.ClaimInteractiveMemoryRun(story.ID, "main", turn.ID, false)
	if err != nil || !claimed {
		t.Fatalf("first claim = %v, err=%v", claimed, err)
	}
	claimed, err = store.ClaimInteractiveMemoryRun(story.ID, "main", turn.ID, false)
	if err != nil || claimed {
		t.Fatalf("duplicate claim = %v, err=%v", claimed, err)
	}
	if err := store.MarkInteractiveMemoryFailed(story.ID, MarkStateFailedRequest{ParentID: turn.ID, BranchID: "main", Error: "recorder failed"}); err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.CurrentTurn == nil || snapshot.CurrentTurn.MemoryStatus != "failed" || snapshot.CurrentTurn.StateStatus != "ready" {
		t.Fatalf("memory failure should not downgrade atomic state: %#v", snapshot.CurrentTurn)
	}
	claimed, err = store.ClaimInteractiveMemoryRun(story.ID, "main", turn.ID, true)
	if err != nil || !claimed {
		t.Fatalf("forced retry = %v, err=%v", claimed, err)
	}
}

func TestCompletedNoOpMemoryRunResetsAutoInterval(t *testing.T) {
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{Title: "空记忆运行", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	interval := 2
	if _, err := store.UpdateStoryMemorySettings(story.ID, StoryMemorySettingsUpdateRequest{AutoIntervalTurns: &interval}); err != nil {
		t.Fatal(err)
	}
	var second TurnEvent
	for i := 0; i < interval; i++ {
		turn, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
			BranchID:   "main",
			User:       "继续",
			Narrative:  "剧情继续。",
			TurnResult: &TurnResult{Contract: TurnContract{PlayerIntent: "继续"}, Choices: []string{"继续向前", "观察四周"}},
		})
		if err != nil {
			t.Fatal(err)
		}
		second = turn
	}
	should, _, err := store.ShouldGenerateStoryMemory(story.ID, "main")
	if err != nil || !should {
		t.Fatalf("memory should be due after interval, should=%v err=%v", should, err)
	}
	if err := store.MarkInteractiveMemoryRunReady(story.ID, "main", second.ID); err != nil {
		t.Fatal(err)
	}
	should, next, err := store.ShouldGenerateStoryMemory(story.ID, "main")
	if err != nil || should || next != interval {
		t.Fatalf("completed no-op run should reset interval, should=%v next=%d err=%v", should, next, err)
	}
	snapshot, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.CurrentTurn == nil || snapshot.CurrentTurn.MemoryEntryID != interactiveMemoryRunMarkerPrefix+second.ID {
		t.Fatalf("memory run marker missing: %#v", snapshot.CurrentTurn)
	}
}

func TestAppendStoryMemoryPatchIsIdempotentPerTurn(t *testing.T) {
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{Title: "追加记忆幂等", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.SaveStoryMemoryStructure(story.ID, StoryMemoryStructureRequest{
		ID:   "turn_log",
		Name: "回合日志",
		Mode: "append",
		Fields: []StoryMemoryField{{
			ID:       "event",
			Name:     "事件",
			Required: true,
		}},
	}); err != nil {
		t.Fatal(err)
	}
	turn, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID:   "main",
		User:       "检查路标",
		Narrative:  "路标背面刻着警告。",
		TurnResult: &TurnResult{Contract: TurnContract{PlayerIntent: "检查路标"}, Choices: []string{"沿路标前进", "检查周围痕迹"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	patch := StoryMemoryPatch{Op: "append", StructureID: "turn_log", Values: map[string]string{"event": "路标背面刻着警告"}}
	for i := 0; i < 2; i++ {
		if _, err := store.ApplyStoryMemoryPatches(story.ID, "main", turn.ID, []StoryMemoryPatch{patch}); err != nil {
			t.Fatal(err)
		}
	}
	memory, err := store.StoryMemory(story.ID, "main", true)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, record := range memory.Records {
		if record.StructureID == "turn_log" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("same append patch retried for one turn should produce one record, got %d: %#v", count, memory.Records)
	}
}
