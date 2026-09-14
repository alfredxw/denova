package interactive

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"denova/internal/agents/conversationjournal"
)

func TestStoryExtensionRecordsRecoverAndRejectStaleSources(t *testing.T) {
	ctx := context.Background()
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{Title: "Extension test"})
	if err != nil {
		t.Fatal(err)
	}
	turn, err := store.AppendTurn(story.ID, AppendTurnRequest{BranchID: "main", User: "Enter", Narrative: "Original prose"})
	if err != nil {
		t.Fatal(err)
	}
	before, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	record := ExtensionRecord{Owner: "game/test.scene", Key: "scene", TurnID: turn.ID, SourceRevision: turn.ID, SchemaVersion: 1, Value: json.RawMessage(`{"lines":["Original prose"]}`)}
	revision, err := store.SetExtensionRecord(ctx, story.ID, "main", 0, record)
	if err != nil || revision == 0 {
		t.Fatalf("write = %d, %v", revision, err)
	}
	after, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if after.ContextRevision != before.ContextRevision || after.TurnCount != before.TurnCount || after.CurrentTurn.ID != before.CurrentTurn.ID {
		t.Fatal("extension data changed Story progression or model context")
	}
	if _, err := os.Stat(store.storyPath(story.ID) + ".pre-extensions-v1.bak"); err != nil {
		t.Fatal("original Story was not backed up", err)
	}
	if _, err := store.SetExtensionRecord(ctx, story.ID, "main", 0, record); !errors.Is(err, conversationjournal.ErrConflict) {
		t.Fatalf("stale CAS accepted: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	indices, err := filepath.Glob(filepath.Join(store.Root(), "interactive", "story", "*.idx.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range indices {
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
	}
	store = NewStore(store.Root())
	defer store.Close()
	loaded, loadedRevision, err := store.ExtensionRecord(ctx, story.ID, "main", record)
	if err != nil || loadedRevision != revision || string(loaded.Value) != string(record.Value) {
		t.Fatalf("recovered record = %+v, %d, %v", loaded, loadedRevision, err)
	}
	if _, err := store.UpdateTurnNarrative(story.ID, UpdateTurnNarrativeRequest{BranchID: "main", TurnID: turn.ID, Narrative: "Edited prose"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SetExtensionRecord(ctx, story.ID, "main", revision, record); !errors.Is(err, conversationjournal.ErrConflict) {
		t.Fatalf("stale source accepted: %v", err)
	}
	if _, err := store.UpdateTurnNarrative(story.ID, UpdateTurnNarrativeRequest{BranchID: "main", TurnID: turn.ID, Narrative: "Original prose"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SetExtensionRecord(ctx, story.ID, "main", revision, record); !errors.Is(err, conversationjournal.ErrConflict) {
		t.Fatalf("old source became valid after text was restored: %v", err)
	}
}

func TestStoryExtensionRecordsRespectOwnerBranchAndExactTurn(t *testing.T) {
	ctx := context.Background()
	store := NewStore(t.TempDir())
	defer store.Close()
	story, err := store.CreateStory(CreateStoryRequest{Title: "Branches"})
	if err != nil {
		t.Fatal(err)
	}
	first, err := store.AppendTurn(story.ID, AppendTurnRequest{BranchID: "main", Narrative: "First"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.AppendTurn(story.ID, AppendTurnRequest{BranchID: "main", Narrative: "Second"})
	if err != nil {
		t.Fatal(err)
	}
	branch, err := store.CreateBranch(story.ID, CreateBranchRequest{ParentEventID: first.ID, Title: "Fork"})
	if err != nil {
		t.Fatal(err)
	}
	record := ExtensionRecord{Owner: "game/a", Key: "scene", TurnID: second.ID, SourceRevision: second.ID, SchemaVersion: 1, Value: json.RawMessage(`{"scene":1}`)}
	if _, err := store.SetExtensionRecord(ctx, story.ID, branch.ID, 0, record); !errors.Is(err, conversationjournal.ErrConflict) {
		t.Fatalf("accepted unreachable source: %v", err)
	}
	if _, err := store.SetExtensionRecord(ctx, story.ID, "main", 0, record); err != nil {
		t.Fatal(err)
	}
	other := record
	other.Owner = "game/b"
	if _, revision, err := store.ExtensionRecord(ctx, story.ID, "main", other); err != nil || revision != 0 {
		t.Fatalf("owner leaked: %d %v", revision, err)
	}
	if _, revision, err := store.ExtensionRecord(ctx, story.ID, branch.ID, record); err != nil || revision != 0 {
		t.Fatalf("branch leaked: %d %v", revision, err)
	}
	// Story-level preferences do not require a turn and survive branch changes.
	prefs := ExtensionRecord{Owner: "game/a", Key: "preferences", SchemaVersion: 2, Value: json.RawMessage(`{"speed":2}`)}
	if _, err := store.SetExtensionRecord(ctx, story.ID, "", 0, prefs); err != nil {
		t.Fatal(err)
	}
	addresses, err := store.ExtensionRecordAddresses(story.ID, "game/a")
	if err != nil || len(addresses) != 1 || addresses[0].Key != "preferences" {
		t.Fatalf("addresses = %+v %v", addresses, err)
	}
}

func TestStoryPreviewDoesNotChangeNormalSelection(t *testing.T) {
	store := NewStore(t.TempDir())
	defer store.Close()
	normal, err := store.CreateStory(CreateStoryRequest{Title: "Normal"})
	if err != nil {
		t.Fatal(err)
	}
	preview, err := store.CreateStory(CreateStoryRequest{Title: "Preview", Preview: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.AppendTurn(preview.ID, AppendTurnRequest{BranchID: "main", Narrative: "Test prose"}); err != nil {
		t.Fatal(err)
	}
	index, err := store.Index()
	if err != nil {
		t.Fatal(err)
	}
	if index.CurrentStoryID != normal.ID {
		t.Fatal("preview changed current story")
	}
	if err := store.SelectStory(preview.ID); err == nil {
		t.Fatal("preview selected through ordinary Story picker")
	}
	previewContext, err := store.StoryContext(preview.ID, "")
	if err != nil || !previewContext.Meta.Preview {
		t.Fatalf("preview marker lost: %+v %v", previewContext.Meta, err)
	}
}
