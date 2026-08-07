package interactive

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestOptimizeBloatedStoryStorageBacksUpAndSquashesDisplayRevisions(t *testing.T) {
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{Title: "旧流式日志", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	turn, err := store.AppendTurn(story.ID, AppendTurnRequest{BranchID: "main", User: "继续", Narrative: "正文"})
	if err != nil {
		t.Fatal(err)
	}

	store.mu.Lock()
	release, err := store.acquireStoryMutationLeaseLocked(story.ID)
	if err != nil {
		store.mu.Unlock()
		t.Fatal(err)
	}
	meta, _, err := store.readStoryRecentLocked(story.ID, "main")
	if err != nil {
		release()
		store.mu.Unlock()
		t.Fatal(err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	revisions := make([]any, 140)
	for index := range revisions {
		revisions[index] = TurnDisplayAppendedEvent{
			V: schemaVersion, Type: StoryEventTypeTurnDisplayAppended, ID: fmt.Sprintf("display-revision-%03d", index),
			ParentID: turn.ID, BranchID: "main", Ts: now, TurnID: turn.ID,
			Display: DisplayEvent{ID: "streamed-tool", Role: "tool_call", Name: "read", Status: "running", Args: strings.Repeat("x", 32*1024) + fmt.Sprint(index)},
		}
	}
	meta.UpdatedAt = now
	if err := store.appendStoryTransactionLocked(story.ID, meta, revisions...); err != nil {
		release()
		store.mu.Unlock()
		t.Fatal(err)
	}
	if err := store.syncStorySummaryLocked(story.ID); err != nil {
		release()
		store.mu.Unlock()
		t.Fatal(err)
	}
	release()
	store.mu.Unlock()

	path := store.storyPath(story.ID)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(before) < storyPresentationCompactionMinBytes {
		t.Fatalf("fixture is not large enough: %d", len(before))
	}
	beforeSnapshot, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(beforeSnapshot.Turns) != 1 || len(beforeSnapshot.Turns[0].DisplayEvents) != 1 {
		t.Fatalf("fixture display projection = %#v", beforeSnapshot.Turns)
	}
	wantDisplay := beforeSnapshot.Turns[0].DisplayEvents[0]

	optimized, err := store.OptimizeBloatedStoryStorage()
	if err != nil {
		t.Fatal(err)
	}
	if optimized != 1 {
		t.Fatalf("optimized stories = %d, want 1", optimized)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(after)*20 >= len(before) {
		t.Fatalf("compacted story remains too large: before=%d after=%d", len(before), len(after))
	}
	backups, err := filepath.Glob(path + ".presentation-v1.*.bak")
	if err != nil || len(backups) != 1 {
		t.Fatalf("presentation backups=%v err=%v", backups, err)
	}
	backup, err := os.ReadFile(backups[0])
	if err != nil || !bytes.Equal(backup, before) {
		t.Fatalf("backup does not preserve original journal: err=%v", err)
	}
	afterSnapshot, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(afterSnapshot.Turns) != 1 || len(afterSnapshot.Turns[0].DisplayEvents) != 1 || !reflect.DeepEqual(afterSnapshot.Turns[0].DisplayEvents[0], wantDisplay) {
		t.Fatalf("display projection changed after compaction: got=%#v want=%#v", afterSnapshot.Turns, wantDisplay)
	}
	store.mu.Lock()
	_, compactedEvents, err := store.readStoryJournalLocked(story.ID)
	store.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range compactedEvents {
		if event.Envelope.Type == StoryEventTypeTurnDisplayAppended {
			t.Fatalf("superseded display revision remains after compaction: %s", event.Envelope.ID)
		}
	}
}
