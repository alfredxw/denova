package interactive

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

const legacyStoryCompactionBackupDirForTest = "interactive-story-v0.3.3-compaction"

func TestOpenStoryMigratesReleasedCompactionEvents(t *testing.T) {
	root := t.TempDir()
	dataRoot := t.TempDir()
	seed := NewStore(root)
	story, err := seed.CreateStory(CreateStoryRequest{Title: "Released story"})
	if err != nil {
		t.Fatal(err)
	}
	first, err := seed.AppendTurn(story.ID, AppendTurnRequest{BranchID: "main", User: "first", Narrative: "First turn."})
	if err != nil {
		t.Fatal(err)
	}
	second, err := seed.AppendTurn(story.ID, AppendTurnRequest{BranchID: "main", User: "second", Narrative: "Second turn."})
	if err != nil {
		t.Fatal(err)
	}
	meta, events, err := seed.readStoryJournalLocked(story.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := seed.Close(); err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 {
		t.Fatalf("seed events = %d, want 2", len(events))
	}

	firstRaw := cloneRawMapForLegacyMigrationTest(t, events[0].Raw)
	secondRaw := cloneRawMapForLegacyMigrationTest(t, events[1].Raw)
	secondRaw["parent_id"] = "ccr-released"
	compaction := map[string]any{
		"v": 1, "type": "context_compaction", "id": "cc-released",
		"parent_id": first.ID, "branch_id": "main", "ts": second.Ts,
		"epoch": 1, "summary": "Released summary", "source_turn_count": 1,
		"retained_turns": 1, "tokens_before": 1200, "tokens_after": 300,
		"context_window_tokens": 128000, "threshold": 0.9,
	}
	removal := map[string]any{
		"v": 1, "type": "context_compaction_removed", "id": "ccr-released",
		"parent_id": "cc-released", "branch_id": "main", "ts": second.Ts,
		"compaction_id": "cc-released", "source_turn_count": 1, "reason": "turn_narrative_edited",
	}
	terminalCompaction := map[string]any{
		"v": 1, "type": "context_compaction", "id": "cc-terminal",
		"parent_id": second.ID, "branch_id": "main", "ts": second.Ts,
		"epoch": 2, "summary": "Latest released summary", "source_turn_count": 2,
		"retained_turns": 1, "tokens_before": 1800, "tokens_after": 350,
		"context_window_tokens": 128000, "threshold": 0.9,
	}
	branch := meta.Branches["main"]
	branch.Head = "cc-terminal"
	branch.FromEvent = "cc-released"
	meta.Branches["main"] = branch
	legacyRows := []any{meta, firstRaw, compaction, removal, secondRaw, terminalCompaction}
	if err := writeJSONL(seed.storyPath(story.ID), legacyRows); err != nil {
		t.Fatal(err)
	}
	indexBytes, err := os.ReadFile(seed.indexPath())
	if err != nil {
		t.Fatal(err)
	}
	var releasedIndex map[string]any
	if err := json.Unmarshal(indexBytes, &releasedIndex); err != nil {
		t.Fatal(err)
	}
	delete(releasedIndex, "version")
	indexBytes, err = json.Marshal(releasedIndex)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeAtomicBytes(seed.indexPath(), append(indexBytes, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	legacyBytes, err := os.ReadFile(seed.storyPath(story.ID))
	if err != nil {
		t.Fatal(err)
	}

	migratedStore := NewStoreWithNovaDir(root, dataRoot)
	migratedIndex, err := migratedStore.Index()
	if err != nil {
		t.Fatal(err)
	}
	if migratedIndex.Version != storyIndexSchemaVersion {
		t.Fatalf("migrated index version = %d, want %d", migratedIndex.Version, storyIndexSchemaVersion)
	}
	context, err := migratedStore.StoryContext(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(context.Snapshot.Turns) != 2 || context.Snapshot.Turns[0].ID != first.ID || context.Snapshot.Turns[1].ID != second.ID {
		t.Fatalf("migrated turns = %#v", context.Snapshot.Turns)
	}
	if got := context.Meta.Branches["main"].Head; got != second.ID {
		t.Fatalf("migrated branch head = %q, want %q", got, second.ID)
	}
	if got := context.Meta.Branches["main"].FromEvent; got != first.ID {
		t.Fatalf("migrated branch from_event = %q, want %q", got, first.ID)
	}
	migratedRaw, err := os.ReadFile(migratedStore.storyPath(story.ID))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(migratedRaw, []byte("context_compaction")) {
		t.Fatalf("migrated journal retained released compaction events:\n%s", migratedRaw)
	}
	if !bytes.Contains(migratedRaw, []byte(`"parent_id":"`+first.ID+`"`)) {
		t.Fatalf("migrated journal did not reconnect second turn to first:\n%s", migratedRaw)
	}

	backups, err := filepath.Glob(filepath.Join(dataRoot, "backups", legacyStoryCompactionBackupDirForTest, "*", filepath.Base(migratedStore.storyPath(story.ID))))
	if err != nil {
		t.Fatal(err)
	}
	if len(backups) != 1 {
		t.Fatalf("migration backups = %#v, want one", backups)
	}
	backupBytes, err := os.ReadFile(backups[0])
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(backupBytes, legacyBytes) {
		t.Fatal("migration backup does not match the released journal")
	}

	if err := migratedStore.Close(); err != nil {
		t.Fatal(err)
	}
	beforeSecondOpen := append([]byte(nil), migratedRaw...)
	reopened := NewStoreWithNovaDir(root, dataRoot)
	if _, err := reopened.StoryContext(story.ID, "main"); err != nil {
		t.Fatal(err)
	}
	if err := reopened.Close(); err != nil {
		t.Fatal(err)
	}
	afterSecondOpen, err := os.ReadFile(seed.storyPath(story.ID))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(afterSecondOpen, beforeSecondOpen) {
		t.Fatal("opening an already migrated story rewrote the canonical journal")
	}
	backups, err = filepath.Glob(filepath.Join(dataRoot, "backups", legacyStoryCompactionBackupDirForTest, "*", filepath.Base(seed.storyPath(story.ID))))
	if err != nil {
		t.Fatal(err)
	}
	if len(backups) != 1 {
		t.Fatalf("idempotent migration backups = %#v, want one", backups)
	}
}

func cloneRawMapForLegacyMigrationTest(t *testing.T, value map[string]any) map[string]any {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var cloned map[string]any
	if err := json.Unmarshal(data, &cloned); err != nil {
		t.Fatal(err)
	}
	return cloned
}
