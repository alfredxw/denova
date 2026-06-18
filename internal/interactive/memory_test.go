package interactive

import "testing"

func TestInteractiveMemoryStoreFiltersUpdatesAndHidesByBranch(t *testing.T) {
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{Title: "记忆测试"})
	if err != nil {
		t.Fatal(err)
	}
	turn, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID:  "main",
		User:      "我拾起钥匙",
		Narrative: "钥匙刻着旧宅的徽记。",
	})
	if err != nil {
		t.Fatal(err)
	}
	generated, err := store.AppendInteractiveMemory(story.ID, "main", turn.ID, InteractiveMemoryCreateRequest{
		Title:      "旧宅钥匙",
		Summary:    "主角获得刻着旧宅徽记的钥匙。",
		Content:    "这把钥匙后续可以用于进入旧宅或证明主角接触过旧宅相关线索。",
		People:     []string{"主角"},
		Places:     []string{"旧宅"},
		Tags:       []string{"线索", "物品"},
		Importance: 4,
	})
	if err != nil {
		t.Fatal(err)
	}
	state, err := store.InteractiveMemory(story.ID, "main", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Entries) != 1 || state.Entries[0].ID != generated.ID || state.SyncStatus != "ready" {
		t.Fatalf("memory state mismatch: %#v", state)
	}
	if _, err := store.CreateBranch(story.ID, CreateBranchRequest{ParentEventID: turn.ID, Title: "支线"}); err != nil {
		t.Fatal(err)
	}
	branchState, err := store.InteractiveMemory(story.ID, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if branchState.BranchID == "main" || len(branchState.Entries) != 1 || branchState.Entries[0].ID != generated.ID {
		t.Fatalf("branch memory should inherit pre-fork records: %#v", branchState)
	}
	updatedTitle := "铜钥匙"
	updatedImportance := 5
	updated, err := store.UpdateInteractiveMemory(story.ID, generated.ID, InteractiveMemoryUpdateRequest{
		Title:      &updatedTitle,
		Importance: &updatedImportance,
		Tags:       []string{"钥匙"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Title != updatedTitle {
		t.Fatalf("updated memory mismatch: %#v", updated)
	}
	mainState, err := store.InteractiveMemory(story.ID, "main", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(mainState.Entries) != 1 || mainState.Entries[0].Title != "旧宅钥匙" {
		t.Fatalf("main branch should keep original inherited memory: %#v", mainState.Entries)
	}
	if _, err := store.SetInteractiveMemoryHidden(story.ID, updated.ID, true); err != nil {
		t.Fatal(err)
	}
	state, err = store.InteractiveMemory(story.ID, branchState.BranchID, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Entries) != 0 {
		t.Fatalf("hidden memory should be excluded: %#v", state.Entries)
	}
	state, err = store.InteractiveMemory(story.ID, branchState.BranchID, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Entries) != 1 || !state.Entries[0].Hidden {
		t.Fatalf("hidden memory should be restorable: %#v", state.Entries)
	}
}

func TestCreateInteractiveMemoryDefaultsToCurrentBranch(t *testing.T) {
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{Title: "手动记忆"})
	if err != nil {
		t.Fatal(err)
	}
	entry, err := store.CreateInteractiveMemory(story.ID, InteractiveMemoryCreateRequest{
		Title:   "手动线索",
		Summary: "用户手动补充的线索。",
	})
	if err != nil {
		t.Fatal(err)
	}
	if entry.BranchID != "main" || !entry.Manual {
		t.Fatalf("manual memory mismatch: %#v", entry)
	}
}

func TestStoryMemoryStructuresRecordsAndBranchCopyOnWrite(t *testing.T) {
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{Title: "故事记忆"})
	if err != nil {
		t.Fatal(err)
	}
	state, err := store.StoryMemory(story.ID, "main", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Structures) < 5 || state.Settings.AutoIntervalTurns != defaultStoryMemoryInterval || !state.Settings.Enabled {
		t.Fatalf("default story memory state mismatch: %#v", state)
	}
	structure, err := store.SaveStoryMemoryStructure(story.ID, StoryMemoryStructureRequest{
		ID:         "relationship_clock",
		Name:       "关系时钟",
		Mode:       "keyed",
		KeyFieldID: "name",
		Fields: []StoryMemoryField{
			{ID: "name", Name: "姓名", Required: true, Order: 10},
			{ID: "status", Name: "状态", Order: 20},
		},
		Order: 90,
	})
	if err != nil {
		t.Fatal(err)
	}
	if structure.ID != "relationship_clock" {
		t.Fatalf("structure mismatch: %#v", structure)
	}
	turn, err := store.AppendTurn(story.ID, AppendTurnRequest{BranchID: "main", User: "我叫住林川", Narrative: "林川停下脚步。"})
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.SaveStoryMemoryRecord(story.ID, StoryMemoryRecordRequest{
		BranchID:    "main",
		StructureID: structure.ID,
		Key:         "林川",
		Values:      map[string]string{"name": "林川", "status": "开始信任主角"},
	})
	if err != nil {
		t.Fatal(err)
	}
	branch, err := store.CreateBranch(story.ID, CreateBranchRequest{ParentEventID: turn.ID, Title: "另一种回应"})
	if err != nil {
		t.Fatal(err)
	}
	branchState, err := store.StoryMemory(story.ID, branch.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(branchState.Records) != 1 || branchState.Records[0].ID != record.ID {
		t.Fatalf("branch should inherit parent record: %#v", branchState.Records)
	}
	updated, err := store.SaveStoryMemoryRecord(story.ID, StoryMemoryRecordRequest{
		ID:          record.ID,
		BranchID:    branch.ID,
		StructureID: structure.ID,
		Key:         "林川",
		Values:      map[string]string{"name": "林川", "status": "怀疑主角"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.ID == record.ID || updated.InheritedFrom != record.ID {
		t.Fatalf("expected copy-on-write record, got %#v", updated)
	}
	mainState, err := store.StoryMemory(story.ID, "main", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(mainState.Records) != 1 || mainState.Records[0].Values["status"] != "开始信任主角" {
		t.Fatalf("main branch should keep original record: %#v", mainState.Records)
	}
}
