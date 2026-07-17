package interactive

import (
	"strings"
	"testing"
)

func TestUpdateTurnNarrativePreservesStoryPathAndInvalidatesCoveredCompaction(t *testing.T) {
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{Title: "正文修正", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	first, err := store.AppendTurn(story.ID, AppendTurnRequest{
		BranchID:      "main",
		User:          "去找朋友",
		Narrative:     "朋友住在 3 楼 403 室。",
		Thinking:      "保留的思考记录",
		DisplayEvents: []DisplayEvent{{ID: "image-1", Role: "tool_result", Content: "保留的图像记录"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.AppendTurn(story.ID, AppendTurnRequest{BranchID: "main", User: "敲门", Narrative: "门内传来脚步声。"})
	if err != nil {
		t.Fatal(err)
	}
	compaction, err := store.AppendContextCompaction(story.ID, "main", ContextCompactionEvent{
		AgentKind:       "interactive_story",
		Summary:         "朋友住在旧地址。",
		SourceTurnCount: 2,
		RetainedTurns:   1,
		TokensBefore:    1200,
		TokensAfter:     200,
	})
	if err != nil {
		t.Fatal(err)
	}

	expected := first.Narrative
	result, err := store.UpdateTurnNarrative(story.ID, UpdateTurnNarrativeRequest{
		BranchID:          "main",
		TurnID:            first.ID,
		Narrative:         "  朋友住在 4 楼 403 室。\r\n  门牌上贴着姓名。  ",
		ExpectedNarrative: &expected,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.ContextCompactionInvalidated {
		t.Fatal("editing compacted prose must invalidate the stale checkpoint")
	}
	if result.Turn.ID != first.ID || result.Turn.Narrative != "朋友住在 4 楼 403 室。\n  门牌上贴着姓名。" {
		t.Fatalf("unexpected updated turn: %#v", result.Turn)
	}
	if result.Turn.Thinking != first.Thinking || len(result.Turn.DisplayEvents) != 1 || result.Turn.DisplayEvents[0].ID != "image-1" {
		t.Fatalf("non-narrative turn data changed: %#v", result.Turn)
	}

	snapshot, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Turns) != 2 || snapshot.Turns[0].ID != first.ID || snapshot.Turns[1].ID != second.ID {
		t.Fatalf("story path changed after prose-only edit: %#v", snapshot.Turns)
	}
	if snapshot.Turns[0].Narrative != result.Turn.Narrative || snapshot.Turns[1].Narrative != second.Narrative {
		t.Fatalf("unexpected persisted narratives: %#v", snapshot.Turns)
	}
	if snapshot.ContextCompaction != nil {
		t.Fatalf("stale compaction remains active: %#v", snapshot.ContextCompaction)
	}
	if snapshot.ContextCompactionRemoval == nil || snapshot.ContextCompactionRemoval.CompactionID != compaction.ID || snapshot.ContextCompactionRemoval.Reason != "turn_narrative_edited" {
		t.Fatalf("missing edit-driven compaction removal: %#v", snapshot.ContextCompactionRemoval)
	}
	if snapshot.CurrentTurn == nil || snapshot.CurrentTurn.ID != second.ID {
		t.Fatalf("later current turn changed: %#v", snapshot.CurrentTurn)
	}
}

func TestUpdateTurnNarrativeRejectsStaleOrInvalidEdits(t *testing.T) {
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{Title: "冲突保护", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	turn, err := store.AppendTurn(story.ID, AppendTurnRequest{BranchID: "main", User: "观察", Narrative: "原始正文"})
	if err != nil {
		t.Fatal(err)
	}

	stale := "过期正文"
	_, err = store.UpdateTurnNarrative(story.ID, UpdateTurnNarrativeRequest{
		BranchID:          "main",
		TurnID:            turn.ID,
		Narrative:         "不应保存",
		ExpectedNarrative: &stale,
	})
	if err == nil || !strings.Contains(err.Error(), "reload") {
		t.Fatalf("stale update should fail with reload guidance: %v", err)
	}
	if _, err := store.UpdateTurnNarrative(story.ID, UpdateTurnNarrativeRequest{BranchID: "main", TurnID: turn.ID, Narrative: "  "}); err == nil {
		t.Fatal("empty narrative should fail")
	}
	if _, err := store.UpdateTurnNarrative(story.ID, UpdateTurnNarrativeRequest{BranchID: "main", TurnID: "missing", Narrative: "正文"}); err == nil {
		t.Fatal("turn outside the active path should fail")
	}

	snapshot, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Turns) != 1 || snapshot.Turns[0].Narrative != turn.Narrative {
		t.Fatalf("rejected edits changed persisted prose: %#v", snapshot.Turns)
	}
}
