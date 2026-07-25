//go:build conversationjournal_benchmark

package interactive

import (
	"bufio"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"testing"

	"denova/internal/conversationjournal"
)

const storyJournalBenchmarkTurns = 100_000

// TestStoryJournal100KOffline exercises the actual game reducer and bounded
// snapshot path. It is intentionally excluded from ordinary CI. Run it with:
//
//	go test -tags conversationjournal_benchmark ./internal/interactive -run TestStoryJournal100KOffline -count=1 -v
func TestStoryJournal100KOffline(t *testing.T) {
	root := t.TempDir()
	seed := NewStore(root)
	story, err := seed.CreateStory(CreateStoryRequest{Title: "十万回合故事", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	meta, _, err := seed.readStoryJournalLocked(story.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := seed.Close(); err != nil {
		t.Fatal(err)
	}
	path := seed.storyPath(story.ID)
	writeStoryJournalBenchmarkFixture(t, path, meta, storyJournalBenchmarkTurns)
	if err := os.Remove(conversationjournal.SidecarPath(path)); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}

	rebuilt := NewStore(root)
	snapshot, err := rebuilt.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.TurnCount != storyJournalBenchmarkTurns || len(snapshot.Turns) != defaultStoryHistoryPageTurns || !snapshot.HasEarlierTurns {
		t.Fatalf("rebuilt snapshot turns=%d resident=%d has_more=%t", snapshot.TurnCount, len(snapshot.Turns), snapshot.HasEarlierTurns)
	}
	handle := rebuilt.storyJournals[story.ID]
	if handle == nil || !handle.journal.ReplayStats().IndexRebuilt {
		t.Fatalf("missing streaming index rebuild: %#v", handle)
	}
	if err := rebuilt.Close(); err != nil {
		t.Fatal(err)
	}

	logInfo, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	indexData, err := os.ReadFile(conversationjournal.SidecarPath(path))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(indexData), "UNIQUE_STORY_100K_NARRATIVE_SENTINEL") {
		t.Fatal("story index contains historical narrative")
	}
	if int64(len(indexData))*100 >= logInfo.Size() {
		t.Fatalf("story index is not below one percent: index=%d journal=%d", len(indexData), logInfo.Size())
	}
	prefixHash := hashFilePrefix(t, path, logInfo.Size())

	reopened := NewStore(root)
	recent, err := reopened.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	handle = reopened.storyJournals[story.ID]
	if handle == nil {
		t.Fatal("indexed story handle was not opened")
	}
	stats := handle.journal.ReplayStats()
	if !stats.IndexLoaded || stats.IndexRebuilt || stats.BytesRead != 0 || stats.TailBytesRead != 0 {
		t.Fatalf("indexed reopen read the canonical prefix: %#v", stats)
	}
	if len(recent.Turns) != defaultStoryHistoryPageTurns {
		t.Fatalf("recent page turns=%d", len(recent.Turns))
	}
	recentBytes := reopened.LastStoryJournalReplayStats(story.ID).BytesRead
	if recentBytes <= 0 || recentBytes*100 >= logInfo.Size() {
		t.Fatalf("recent story page scales with total journal: page=%d journal=%d", recentBytes, logInfo.Size())
	}
	continued, err := reopened.AppendTurn(story.ID, AppendTurnRequest{
		BranchID: "main", User: "继续第十万零一回合", Narrative: "索引重开后故事继续。",
	})
	if err != nil {
		t.Fatal(err)
	}
	if hot := reopened.LastStoryJournalReplayStats(story.ID); hot.BytesRead != 0 || hot.RecordsRead != 0 {
		t.Fatalf("hot story append read old history: %#v", hot)
	}
	if got := hashFilePrefix(t, path, logInfo.Size()); got != prefixHash {
		t.Fatal("continued story changed the canonical JSONL prefix")
	}
	if err := reopened.Close(); err != nil {
		t.Fatal(err)
	}

	continuedStore := NewStore(root)
	continuedSnapshot, err := continuedStore.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if continuedSnapshot.TurnCount != storyJournalBenchmarkTurns+1 || continuedSnapshot.CurrentTurn == nil || continuedSnapshot.CurrentTurn.ID != continued.ID {
		t.Fatalf("continued story was not recovered: turns=%d current=%#v", continuedSnapshot.TurnCount, continuedSnapshot.CurrentTurn)
	}
	if err := continuedStore.Close(); err != nil {
		t.Fatal(err)
	}

	if err := os.Remove(conversationjournal.SidecarPath(path)); err != nil {
		t.Fatal(err)
	}
	runtime.GC()
	var beforeMemory runtime.MemStats
	runtime.ReadMemStats(&beforeMemory)
	streamRebuilt := NewStore(root)
	if _, err := streamRebuilt.Snapshot(story.ID, "main"); err != nil {
		t.Fatal(err)
	}
	runtime.GC()
	var afterMemory runtime.MemStats
	runtime.ReadMemStats(&afterMemory)
	if delta := int64(afterMemory.Alloc) - int64(beforeMemory.Alloc); delta > 64*1024*1024 {
		t.Fatalf("streaming story rebuild retained too much heap: delta=%d", delta)
	}
	if err := streamRebuilt.Close(); err != nil {
		t.Fatal(err)
	}
	t.Logf("100k story journal=%d bytes index=%d bytes recent_read=%d bytes retained_heap_delta=%d bytes", logInfo.Size(), len(indexData), recentBytes, int64(afterMemory.Alloc)-int64(beforeMemory.Alloc))
}

func writeStoryJournalBenchmarkFixture(t *testing.T, path string, meta StoryMeta, turns int) {
	t.Helper()
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	writer := bufio.NewWriterSize(file, 1024*1024)
	finalID := benchmarkStoryTurnID(turns)
	branch := meta.Branches["main"]
	branch.Head = finalID
	meta.Branches["main"] = branch
	header, err := json.Marshal(meta)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write(append(header, '\n')); err != nil {
		t.Fatal(err)
	}
	narrative := "UNIQUE_STORY_100K_NARRATIVE_SENTINEL " + strings.Repeat("叙", 2048)
	for sequence := 1; sequence <= turns; sequence++ {
		parent := any(nil)
		if sequence > 1 {
			parent = benchmarkStoryTurnID(sequence - 1)
		}
		turn := TurnEvent{
			V: schemaVersion, Type: StoryEventTypeTurn, ID: benchmarkStoryTurnID(sequence), ParentID: parent,
			BranchID: "main", Ts: "2026-01-01T00:00:00Z", User: "继续故事", Narrative: narrative,
		}
		line, marshalErr := json.Marshal(turn)
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		if _, writeErr := writer.Write(append(line, '\n')); writeErr != nil {
			t.Fatal(writeErr)
		}
	}
	if err := writer.Flush(); err != nil {
		t.Fatal(err)
	}
	if err := file.Sync(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}

func benchmarkStoryTurnID(sequence int) string {
	return fmt.Sprintf("turn-%06d", sequence)
}

func hashFilePrefix(t *testing.T, path string, size int64) [sha256.Size]byte {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	digest := sha256.New()
	if _, err := io.CopyN(digest, file, size); err != nil {
		t.Fatal(err)
	}
	var result [sha256.Size]byte
	copy(result[:], digest.Sum(nil))
	return result
}
