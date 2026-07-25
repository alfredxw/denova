//go:build conversationjournal_benchmark

package conversationjournal

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

const conversationJournalBenchmarkTurns = 100_000

type boundedBenchmarkProjection struct {
	Count        int    `json:"count"`
	LatestCursor Cursor `json:"latest_cursor"`
}

func (projection *boundedBenchmarkProjection) Reset() error {
	*projection = boundedBenchmarkProjection{}
	return nil
}

func (projection *boundedBenchmarkProjection) Restore(data json.RawMessage) error {
	return json.Unmarshal(data, projection)
}

func (projection *boundedBenchmarkProjection) Apply(record Record) error {
	var payload struct {
		Sequence int `json:"sequence"`
	}
	if err := json.Unmarshal(record.Payload, &payload); err != nil {
		return err
	}
	if payload.Sequence > 0 {
		projection.Count++
		projection.LatestCursor = record.Location.Cursor
	}
	return nil
}

func (projection *boundedBenchmarkProjection) Checkpoint() (json.RawMessage, error) {
	return json.Marshal(projection)
}

// TestConversationJournal100KOffline is intentionally excluded from ordinary
// CI. Run it with:
//
//	go test -tags conversationjournal_benchmark ./internal/conversationjournal -run TestConversationJournal100KOffline -count=1 -v
//
// The fixture uses the real checksummed transaction codec but syncs the bulk
// seed once; the tested continuation itself goes through Journal.Append/fsync.
func TestConversationJournal100KOffline(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "conversation-100k.jsonl")
	identity := Identity{ID: "conversation-100k", Generation: "generation-100k"}
	writeConversationJournalBenchmarkFixture(t, path, identity, conversationJournalBenchmarkTurns)

	projection := &boundedBenchmarkProjection{}
	journal, err := Open(context.Background(), path, identity, projection, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if !journal.ReplayStats().IndexRebuilt || projection.Count != conversationJournalBenchmarkTurns {
		t.Fatalf("initial streaming rebuild stats=%#v projection=%#v", journal.ReplayStats(), projection)
	}
	if err := journal.Close(); err != nil {
		t.Fatal(err)
	}

	logInfo, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	indexData, err := os.ReadFile(SidecarPath(path))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(indexData, []byte("UNIQUE_100K_NARRATIVE_SENTINEL")) {
		t.Fatal("derived index contains historical narrative")
	}
	if int64(len(indexData))*100 >= logInfo.Size() {
		t.Fatalf("index is not below one percent of the journal: index=%d journal=%d", len(indexData), logInfo.Size())
	}

	reopenedProjection := &boundedBenchmarkProjection{}
	reopened, err := Open(context.Background(), path, identity, reopenedProjection, Options{})
	if err != nil {
		t.Fatal(err)
	}
	stats := reopened.ReplayStats()
	if !stats.IndexLoaded || stats.IndexRebuilt || stats.BytesRead != 0 || stats.TailBytesRead != 0 {
		t.Fatalf("indexed reopen read canonical prefix: %#v", stats)
	}
	beforeAppendReads := reopened.ReplayStats().BytesRead
	payload, _ := json.Marshal(map[string]any{"sequence": conversationJournalBenchmarkTurns + 1, "user": "continue", "narrative": "continued after indexed reopen"})
	head := reopened.Head()
	if _, err := reopened.Append(context.Background(), Guard{Cursor: head.Cursor, RecordSHA256: head.RecordSHA256}, payload); err != nil {
		t.Fatal(err)
	}
	if reopened.ReplayStats().BytesRead != beforeAppendReads {
		t.Fatalf("hot append read old canonical history: before=%d after=%d", beforeAppendReads, reopened.ReplayStats().BytesRead)
	}
	head = reopened.Head()
	recent, err := reopened.ReadRange(context.Background(), Range{After: head.Cursor - 100, Through: head.Cursor, Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	if len(recent) != 100 {
		t.Fatalf("recent page records=%d want=100", len(recent))
	}
	recentBytesRead := reopened.ReplayStats().LastRangeBytesRead
	if recentBytesRead <= 0 || recentBytesRead*100 >= logInfo.Size() {
		t.Fatalf("recent page read scales with total journal: page_bytes=%d journal_bytes=%d", recentBytesRead, logInfo.Size())
	}
	if err := reopened.Close(); err != nil {
		t.Fatal(err)
	}

	if err := os.Remove(SidecarPath(path)); err != nil {
		t.Fatal(err)
	}
	runtime.GC()
	var beforeMemory runtime.MemStats
	runtime.ReadMemStats(&beforeMemory)
	rebuiltProjection := &boundedBenchmarkProjection{}
	rebuilt, err := Open(context.Background(), path, identity, rebuiltProjection, Options{})
	if err != nil {
		t.Fatal(err)
	}
	runtime.GC()
	var afterMemory runtime.MemStats
	runtime.ReadMemStats(&afterMemory)
	if !rebuilt.ReplayStats().IndexRebuilt || rebuiltProjection.Count != conversationJournalBenchmarkTurns+1 {
		t.Fatalf("index deletion did not stream-rebuild: stats=%#v projection=%#v", rebuilt.ReplayStats(), rebuiltProjection)
	}
	if afterMemory.Alloc > beforeMemory.Alloc+64*1024*1024 {
		t.Fatalf("streaming rebuild retained too much heap: before=%d after=%d", beforeMemory.Alloc, afterMemory.Alloc)
	}
	if err := rebuilt.Close(); err != nil {
		t.Fatal(err)
	}
	t.Logf("100k journal=%d bytes index=%d bytes recent_read=%d bytes retained_heap_delta=%d bytes", logInfo.Size(), len(indexData), recentBytesRead, int64(afterMemory.Alloc)-int64(beforeMemory.Alloc))
}

func writeConversationJournalBenchmarkFixture(t *testing.T, path string, identity Identity, turns int) {
	t.Helper()
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	writer := bufio.NewWriterSize(file, 1024*1024)
	header := []byte(`{"type":"benchmark_header"}`)
	if _, err := writer.Write(append(header, '\n')); err != nil {
		t.Fatal(err)
	}
	previousSHA := recordSHA256(header)
	narrative := "UNIQUE_100K_NARRATIVE_SENTINEL " + string(bytes.Repeat([]byte("n"), 6*1024))
	for sequence := 1; sequence <= turns; sequence++ {
		payload, marshalErr := json.Marshal(map[string]any{
			"sequence":  sequence,
			"user":      "continue the story",
			"narrative": narrative,
		})
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		line, encodeErr := encodeTransaction(identity, Cursor(sequence+1), previousSHA, []json.RawMessage{payload})
		if encodeErr != nil {
			t.Fatal(encodeErr)
		}
		if _, writeErr := writer.Write(append(line, '\n')); writeErr != nil {
			t.Fatal(writeErr)
		}
		previousSHA = recordSHA256(line)
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
