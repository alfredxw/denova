package interactive

import (
	"testing"
)

func TestSearchStoryHistoryUsesCurrentBranchTurnsAsSource(t *testing.T) {
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{Title: "历史检索", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	first, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID:  "main",
		User:      "前往银月港",
		Narrative: "林舟在银月港见到了穿红衣的岚。",
		Ops:       []StateOp{{Op: "set", Path: "actors.story.id", Value: DefaultStoryContextActorID}},
	})
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID:  "main",
		User:      "询问黑帆船",
		Narrative: "岚承认黑帆船会在午夜靠岸。",
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := store.SearchStoryHistory(story.ID, "main", StoryHistorySearchRequest{Keywords: []string{"银月港", "岚"}, Match: "all"})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Hits) != 1 || result.Hits[0].TurnID != first.ID {
		t.Fatalf("unexpected hits: %#v", result.Hits)
	}
	if len(result.Hits[0].StateChanges) != 1 {
		t.Fatalf("state source was not included: %#v", result.Hits[0])
	}

	recent, err := store.SearchStoryHistory(story.ID, "main", StoryHistorySearchRequest{BeforeTurnID: second.ID, Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(recent.Hits) != 1 || recent.Hits[0].TurnID != first.ID {
		t.Fatalf("unexpected bounded history: %#v", recent.Hits)
	}
}
