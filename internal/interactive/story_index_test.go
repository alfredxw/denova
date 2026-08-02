package interactive

import (
	"encoding/json"
	"os"
	"testing"
)

func TestStoryIndexTracksCurrentBranchTurnsInsteadOfJournalEvents(t *testing.T) {
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{Title: "分支回合统计", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	first, err := store.AppendTurn(story.ID, AppendTurnRequest{BranchID: "main", User: "第一步", Narrative: "抵达路口。"})
	if err != nil {
		t.Fatal(err)
	}
	for _, input := range []AppendTurnRequest{
		{BranchID: "main", User: "第二步", Narrative: "沿主路前进。"},
		{BranchID: "main", User: "第三步", Narrative: "抵达主路尽头。"},
	} {
		if _, err := store.AppendTurn(story.ID, input); err != nil {
			t.Fatal(err)
		}
	}

	branch, err := store.CreateBranch(story.ID, CreateBranchRequest{ParentEventID: first.ID, Title: "岔路"})
	if err != nil {
		t.Fatal(err)
	}
	index, err := store.Index()
	if err != nil {
		t.Fatal(err)
	}
	if got := index.Stories[0].TurnCount; got != 1 {
		t.Fatalf("new branch turn count = %d, want 1", got)
	}

	branchTurn, err := store.AppendTurn(story.ID, AppendTurnRequest{BranchID: branch.ID, User: "改走岔路", Narrative: "岔路通向山谷。"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AppendTurnDisplayEvent(story.ID, branch.ID, branchTurn.ID, DisplayEvent{
		ID: "image-branch", Role: "tool_result", Name: "generate_interactive_image", Status: "success",
	}); err != nil {
		t.Fatal(err)
	}
	index, err = store.Index()
	if err != nil {
		t.Fatal(err)
	}
	if got := index.Stories[0].TurnCount; got != 2 {
		t.Fatalf("branch turn count after side event = %d, want 2", got)
	}
	if index.Stories[0].Events <= index.Stories[0].TurnCount {
		t.Fatalf("test requires journal events to exceed logical turns: %#v", index.Stories[0])
	}

	if err := store.SwitchBranch(story.ID, "main"); err != nil {
		t.Fatal(err)
	}
	index, err = store.Index()
	if err != nil {
		t.Fatal(err)
	}
	if got := index.Stories[0].TurnCount; got != 3 {
		t.Fatalf("main branch turn count = %d, want 3", got)
	}
}

func TestStoryIndexMigratesTurnCountFromCanonicalJournal(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)
	story, err := store.CreateStory(CreateStoryRequest{Title: "旧索引", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	for _, input := range []AppendTurnRequest{
		{BranchID: "main", User: "第一步", Narrative: "第一轮。"},
		{BranchID: "main", User: "第二步", Narrative: "第二轮。"},
	} {
		if _, err := store.AppendTurn(story.ID, input); err != nil {
			t.Fatal(err)
		}
	}
	index, err := store.Index()
	if err != nil {
		t.Fatal(err)
	}
	index.Version = 0
	index.Stories[0].Events = 99
	index.Stories[0].TurnCount = 0
	legacy, err := json.Marshal(index)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.indexPath(), legacy, 0o644); err != nil {
		t.Fatal(err)
	}

	reopened := NewStore(root)
	migrated, err := reopened.Index()
	if err != nil {
		t.Fatal(err)
	}
	if migrated.Version != storyIndexSchemaVersion {
		t.Fatalf("index version = %d, want %d", migrated.Version, storyIndexSchemaVersion)
	}
	if got := migrated.Stories[0].TurnCount; got != 2 {
		t.Fatalf("migrated turn count = %d, want 2", got)
	}
	if got := migrated.Stories[0].Events; got != 2 {
		t.Fatalf("migrated event count = %d, want canonical 2", got)
	}
}
