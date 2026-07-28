package interactive

import (
	"fmt"
	"reflect"
	"strings"
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

func TestSearchStoryHistoryMatchAllUsesEveryKeyword(t *testing.T) {
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{Title: "all keywords", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	keywords := make([]string, 10)
	for index := range keywords {
		keywords[index] = fmt.Sprintf("线索%d", index+1)
	}
	complete, err := store.AppendTurn(story.ID, AppendTurnRequest{
		BranchID: "main", User: "追踪全部线索", Narrative: strings.Join(keywords, " "),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.AppendTurn(story.ID, AppendTurnRequest{
		BranchID: "main", User: "只看到部分", Narrative: strings.Join(keywords[:8], " "),
	}); err != nil {
		t.Fatal(err)
	}
	result, err := store.SearchStoryHistory(story.ID, "main", StoryHistorySearchRequest{Keywords: keywords, Match: "all"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(result.Keywords, keywords) || len(result.Hits) != 1 || result.Hits[0].TurnID != complete.ID {
		t.Fatalf("all-keyword result = %#v", result)
	}
}

func TestSearchStoryHistoryCursorFreezesHistoryHead(t *testing.T) {
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{Title: "cursor", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	for index := range 5 {
		if _, err := store.AppendTurn(story.ID, AppendTurnRequest{
			BranchID: "main", User: fmt.Sprintf("action %d", index), Narrative: "shared clue",
		}); err != nil {
			t.Fatal(err)
		}
	}
	request := StoryHistorySearchRequest{Keywords: []string{"shared", "clue"}, Match: "all", Limit: 2, MaxBytes: 64 * 1024}
	first, err := store.SearchStoryHistory(story.ID, "main", request)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Hits) != 2 || !first.Truncated || first.NextCursor == "" {
		t.Fatalf("first history page = %#v", first)
	}
	newTurn, err := store.AppendTurn(story.ID, AppendTurnRequest{BranchID: "main", User: "new action", Narrative: "shared clue"})
	if err != nil {
		t.Fatal(err)
	}
	request.Cursor = first.NextCursor
	second, err := store.SearchStoryHistory(story.ID, "main", request)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Hits) != 2 || second.Hits[0].TurnID == newTurn.ID || second.Hits[1].TurnID == newTurn.ID {
		t.Fatalf("second history page crossed frozen head = %#v", second)
	}
	for _, firstHit := range first.Hits {
		for _, secondHit := range second.Hits {
			if firstHit.TurnID == secondHit.TurnID {
				t.Fatalf("history cursor repeated turn %s", firstHit.TurnID)
			}
		}
	}
	request.Keywords = []string{"different"}
	if _, err := store.SearchStoryHistory(story.ID, "main", request); err == nil || !strings.Contains(err.Error(), "does not belong") {
		t.Fatalf("mismatched history cursor error = %v", err)
	}
}
