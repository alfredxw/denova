package interactive

import (
	"os"
	"testing"

	"denova/internal/agents/conversationjournal"
)

func TestCanonicalHistoryHeadTracksAppendsEditsAndIndexRebuild(t *testing.T) {
	ctx := t.Context()
	workspace := t.TempDir()
	store := NewStore(workspace)
	story, err := store.CreateStory(CreateStoryRequest{Title: "History head", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	initial, err := store.CanonicalHistoryHead(ctx, story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	turn, err := store.AppendTurn(story.ID, AppendTurnRequest{BranchID: "main", User: "request", Narrative: "original narrative"})
	if err != nil {
		t.Fatal(err)
	}
	appended, err := store.CanonicalHistoryHead(ctx, story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if appended.Identity != initial.Identity || appended.Revision != turn.ID {
		t.Fatalf("append changed lane identity or missed receipt: initial=%+v appended=%+v", initial, appended)
	}
	fork, err := store.CreateBranch(story.ID, CreateBranchRequest{ParentEventID: turn.ID, Title: "shared prefix"})
	if err != nil {
		t.Fatal(err)
	}
	forkBefore, err := store.CanonicalHistoryHead(ctx, story.ID, fork.ID)
	if err != nil {
		t.Fatal(err)
	}
	if forkBefore.Identity == appended.Identity {
		t.Fatal("branches share a history identity")
	}
	if err := store.AppendTurnDisplayEvent(story.ID, "main", turn.ID, DisplayEvent{Role: "thinking", Content: "display annotation"}); err != nil {
		t.Fatal(err)
	}
	display, err := store.CanonicalHistoryHead(ctx, story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if display != appended {
		t.Fatalf("display append changed model history: %+v", display)
	}
	if _, err := store.UpdateTurnNarrative(story.ID, UpdateTurnNarrativeRequest{BranchID: "main", TurnID: turn.ID, Narrative: "edited narrative"}); err != nil {
		t.Fatal(err)
	}
	edited, err := store.CanonicalHistoryHead(ctx, story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	forkAfter, err := store.CanonicalHistoryHead(ctx, story.ID, fork.ID)
	if err != nil {
		t.Fatal(err)
	}
	if edited.Identity == appended.Identity || forkBefore != forkAfter {
		t.Fatal("edit did not isolate the changed lane from its frozen fork")
	}
	path := store.storyPath(story.ID)
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(conversationjournal.SidecarPath(path)); err != nil {
		t.Fatal(err)
	}
	reopened := NewStore(workspace)
	defer reopened.Close()
	restored, err := reopened.CanonicalHistoryHead(ctx, story.ID, "main")
	if err != nil || restored != edited {
		t.Fatalf("rebuilt head=%+v want=%+v error=%v", restored, edited, err)
	}
	inherited, err := reopened.ReadModelHistory(story.ID, StoryModelHistoryQuery{BranchID: fork.ID, EndTurn: 1})
	if err != nil || len(inherited.Turns) != 1 || inherited.Turns[0].Narrative != "original narrative" {
		t.Fatalf("shared projection=%+v %v", inherited, err)
	}
}
