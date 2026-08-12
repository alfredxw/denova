package interactive

import (
	"fmt"
	"testing"
)

func TestReadModelHistoryProjectsExactRangeBeyondDisplayPage(t *testing.T) {
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{Title: "长篇故事", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	requests := make([]AppendTurnRequest, 0, 110)
	for index := 0; index < 110; index++ {
		requests = append(requests, AppendTurnRequest{
			User:      fmt.Sprintf("行动-%03d", index),
			Narrative: fmt.Sprintf("剧情-%03d", index),
			Thinking:  "不应进入模型历史投影",
			DisplayEvents: []DisplayEvent{{
				Role: "thinking", Content: "仅用于恢复界面",
			}},
			ModelContextMessages: []ModelContextMessage{{
				Role: "tool", Content: fmt.Sprintf("证据-%03d", index), ToolCallID: fmt.Sprintf("call-%03d", index), ToolName: "inspect_story",
			}},
		})
	}
	appendStoryTurns(t, store, story.ID, "main", requests)

	snapshot, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Turns) != defaultStoryHistoryPageTurns {
		t.Fatalf("display snapshot turns = %d, want %d", len(snapshot.Turns), defaultStoryHistoryPageTurns)
	}

	history, err := store.ReadModelHistory(story.ID, StoryModelHistoryQuery{
		BranchID: "main", StartTurn: 7, EndTurn: 107,
	})
	if err != nil {
		t.Fatal(err)
	}
	if history.TotalTurns != 110 || history.StartTurn != 7 || history.EndTurn != 107 || len(history.Turns) != 100 {
		t.Fatalf("unexpected model history range: %#v", history)
	}
	if history.Turns[0].User != "行动-007" || history.Turns[len(history.Turns)-1].Narrative != "剧情-106" {
		t.Fatalf("model history returned wrong logical range: first=%#v last=%#v", history.Turns[0], history.Turns[len(history.Turns)-1])
	}
	if messages := history.Turns[0].ModelContextMessages; len(messages) != 1 || messages[0].Content != "证据-007" {
		t.Fatalf("model-visible tool evidence was not preserved: %#v", messages)
	}
}

func TestReadModelHistoryRejectsRangePastCanonicalHead(t *testing.T) {
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{Title: "范围校验", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ReadModelHistory(story.ID, StoryModelHistoryQuery{BranchID: "main", StartTurn: 0, EndTurn: 1}); err == nil {
		t.Fatal("expected range past canonical head to fail")
	}
}
