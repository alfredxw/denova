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
	if branchState.BranchID == "main" || len(branchState.Entries) != 0 {
		t.Fatalf("branch memory should be isolated: %#v", branchState)
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
	if updated.Title != updatedTitle || updated.Importance != updatedImportance || len(updated.Tags) != 1 || updated.Tags[0] != "钥匙" {
		t.Fatalf("updated memory mismatch: %#v", updated)
	}
	if _, err := store.SetInteractiveMemoryHidden(story.ID, generated.ID, true); err != nil {
		t.Fatal(err)
	}
	state, err = store.InteractiveMemory(story.ID, "main", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Entries) != 0 {
		t.Fatalf("hidden memory should be excluded: %#v", state.Entries)
	}
	state, err = store.InteractiveMemory(story.ID, "main", true)
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
