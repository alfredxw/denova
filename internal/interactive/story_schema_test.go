package interactive

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAppendTurnWithStatePersistsStateOpSchemaVersion(t *testing.T) {
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{Title: "schema", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	turn, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID:  "main",
		User:      "检查门",
		Narrative: "门上刻着新符号。",
		Ops:       []StateOp{{Op: "set", Path: "scene.symbol", Value: "月亮"}},
	})
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(store.Root(), "interactive", "story", "story-"+story.ID+".jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("jsonl line count = %d, want 2\n%s", len(lines), string(data))
	}
	var raw map[string]any
	if err := json.Unmarshal([]byte(lines[1]), &raw); err != nil {
		t.Fatal(err)
	}
	stateDelta, ok := raw["state_delta"].(map[string]any)
	if !ok {
		t.Fatalf("turn %s should carry state_delta: %#v", turn.ID, raw)
	}
	if stateDelta["schema_version"] != float64(stateOpSchemaVersion) {
		t.Fatalf("schema_version = %#v, want %d", stateDelta["schema_version"], stateOpSchemaVersion)
	}
}

func TestAppendStateDeltaRejectsInvalidStateOp(t *testing.T) {
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{Title: "bad op", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	turn, err := store.AppendTurn(story.ID, AppendTurnRequest{BranchID: "main", User: "继续", Narrative: "前方出现岔路。"})
	if err != nil {
		t.Fatal(err)
	}

	_, err = store.AppendStateDelta(story.ID, AppendStateDeltaRequest{
		ParentID: turn.ID,
		BranchID: "main",
		Ops:      []StateOp{{Op: "teleport", Path: "scene.place", Value: "塔顶"}},
	})
	if err == nil {
		t.Fatal("expected invalid state op to be rejected")
	}
	if !strings.Contains(err.Error(), "未知状态操作") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSnapshotRejectsCorruptStoryEventEnvelope(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	meta := StoryMeta{
		V:             schemaVersion,
		Type:          StoryEventTypeMeta,
		StoryID:       "st_corrupt",
		Title:         "corrupt",
		StoryTellerID: "classic",
		CurrentBranch: "main",
		Branches:      map[string]BranchMeta{"main": {Head: "ev_bad", CreatedAt: now}},
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := writeJSONL(store.storyPath("st_corrupt"), []any{
		meta,
		map[string]any{
			"v":         schemaVersion,
			"type":      "unknown_event",
			"id":        "ev_bad",
			"branch_id": "main",
			"ts":        now,
		},
	}); err != nil {
		t.Fatal(err)
	}

	_, err := store.Snapshot("st_corrupt", "main")
	if err == nil {
		t.Fatal("expected corrupt story event to be rejected")
	}
	if !strings.Contains(err.Error(), "未知故事事件类型") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAppendContextCompactionRemovalIsAcceptedByStorySchema(t *testing.T) {
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{Title: "compaction", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	turn, err := store.AppendTurn(story.ID, AppendTurnRequest{BranchID: "main", User: "继续", Narrative: "剧情继续。"})
	if err != nil {
		t.Fatal(err)
	}
	compaction, err := store.AppendContextCompaction(story.ID, "main", ContextCompactionEvent{
		AgentKind:       "interactive",
		Summary:         "较早剧情摘要",
		SourceTurnCount: 1,
		RetainedTurns:   1,
		TokensBefore:    1200,
		TokensAfter:     200,
	})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.ContextCompaction == nil || snapshot.ContextCompaction.ID != compaction.ID {
		t.Fatalf("snapshot should expose active compaction: %#v", snapshot.ContextCompaction)
	}

	removal, err := store.AppendContextCompactionRemoval(story.ID, "main", ContextCompactionRemovalEvent{
		AgentKind:       "interactive",
		CompactionID:    compaction.ID,
		SourceTurnCount: len(snapshot.Turns),
		Reason:          "user_removed",
	})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err = store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.ContextCompaction != nil {
		t.Fatalf("snapshot should clear active compaction after removal: %#v", snapshot.ContextCompaction)
	}
	if snapshot.ContextCompactionRemoval == nil || snapshot.ContextCompactionRemoval.ID != removal.ID {
		t.Fatalf("snapshot should expose compaction removal: %#v", snapshot.ContextCompactionRemoval)
	}
	if snapshot.CurrentTurn == nil || snapshot.CurrentTurn.ID != turn.ID {
		t.Fatalf("removal should keep latest turn as current turn: %#v", snapshot.CurrentTurn)
	}
}
