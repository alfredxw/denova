package interactive

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"testing"

	"denova/internal/conversationjournal"
)

func TestStoryHistoryPagesUseBoundedIndexedRangesWithoutDuplicates(t *testing.T) {
	root := t.TempDir()
	seed := NewStore(root)
	story, err := seed.CreateStory(CreateStoryRequest{Title: "千回合分页", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	meta, _, err := seed.readStoryJournalLocked(story.ID)
	if err != nil {
		t.Fatal(err)
	}
	lines := make([]any, 0, 1001)
	parentID := ""
	turnIDs := make([]string, 0, 1000)
	for index := 0; index < 1000; index++ {
		id := fmt.Sprintf("turn-%04d", index)
		turnIDs = append(turnIDs, id)
		parent := any(nil)
		if parentID != "" {
			parent = parentID
		}
		lines = append(lines, TurnEvent{
			V: schemaVersion, Type: StoryEventTypeTurn, ID: id, ParentID: parent,
			BranchID: "main", Ts: fmt.Sprintf("2026-01-01T00:%02d:%02dZ", (index/60)%60, index%60),
			User: fmt.Sprintf("行动 %04d", index), Narrative: fmt.Sprintf("正文 %04d %s", index, strings.Repeat("叙", 2500)),
		})
		parentID = id
	}
	branch := meta.Branches["main"]
	branch.Head = parentID
	meta.Branches["main"] = branch
	canonical := append([]any{meta}, lines...)
	if err := writeJSONL(seed.storyPath(story.ID), canonical); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(conversationjournal.SidecarPath(seed.storyPath(story.ID))); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}

	indexed := NewStore(root)
	initial, err := indexed.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(initial.Turns) != defaultStoryHistoryPageTurns || initial.TurnCount != 1000 || !initial.HasEarlierTurns || initial.HistoryBeforeCursor == "" {
		t.Fatalf("initial bounded snapshot = turns:%d total:%d more:%t cursor:%q", len(initial.Turns), initial.TurnCount, initial.HasEarlierTurns, initial.HistoryBeforeCursor)
	}

	reopened := NewStore(root)
	recent, err := reopened.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	fileInfo, err := os.Stat(reopened.storyPath(story.ID))
	if err != nil {
		t.Fatal(err)
	}
	stats := reopened.LastStoryJournalReplayStats(story.ID)
	if stats.BytesRead <= 0 || stats.BytesRead >= fileInfo.Size()/2 {
		t.Fatalf("indexed recent page read %d of %d bytes", stats.BytesRead, fileInfo.Size())
	}

	all := append([]TurnEvent(nil), recent.Turns...)
	cursor := recent.HistoryBeforeCursor
	for recent.HasEarlierTurns {
		page, pageErr := reopened.ReadHistoryPage(story.ID, "main", cursor, 100)
		if pageErr != nil {
			t.Fatal(pageErr)
		}
		all = append(append([]TurnEvent(nil), page.Turns...), all...)
		cursor = page.BeforeCursor
		recent.HasEarlierTurns = page.HasMore
	}
	if len(all) != len(turnIDs) {
		t.Fatalf("paged turns = %d, want %d", len(all), len(turnIDs))
	}
	seen := make(map[string]bool, len(all))
	for index, turn := range all {
		if turn.ID != turnIDs[index] || seen[turn.ID] {
			t.Fatalf("turn %d = %q duplicate=%t, want %q", index, turn.ID, seen[turn.ID], turnIDs[index])
		}
		seen[turn.ID] = true
	}
	indexData, err := os.ReadFile(conversationjournal.SidecarPath(reopened.storyPath(story.ID)))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(indexData), "正文 0999") {
		t.Fatal("story index leaked historical narrative")
	}
	if int64(len(indexData))*100 >= fileInfo.Size() {
		t.Fatalf("story index is not below 1%% of representative log: index=%d log=%d", len(indexData), fileInfo.Size())
	}
	prefix, err := os.ReadFile(reopened.storyPath(story.ID))
	if err != nil {
		t.Fatal(err)
	}
	continued, err := reopened.AppendTurn(story.ID, AppendTurnRequest{
		BranchID: "main", User: "继续前进", Narrative: "第 1001 轮从缓存的最新投影继续。",
	})
	if err != nil {
		t.Fatal(err)
	}
	if stats := reopened.LastStoryJournalReplayStats(story.ID); stats.BytesRead != 0 || stats.RecordsRead != 0 {
		t.Fatalf("hot append reread old story history: %#v", stats)
	}
	afterAppend, err := os.ReadFile(reopened.storyPath(story.ID))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(afterAppend, prefix) {
		t.Fatal("hot append rewrote the canonical story prefix")
	}
	if err := reopened.Close(); err != nil {
		t.Fatal(err)
	}
	continuedStore := NewStore(root)
	continuedSnapshot, err := continuedStore.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if continuedSnapshot.CurrentTurn == nil || continuedSnapshot.CurrentTurn.ID != continued.ID || continuedSnapshot.TurnCount != 1001 {
		t.Fatalf("indexed continuation was not recovered: current=%#v total=%d", continuedSnapshot.CurrentTurn, continuedSnapshot.TurnCount)
	}
}
