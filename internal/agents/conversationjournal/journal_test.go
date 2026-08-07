package conversationjournal

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type countingProjection struct {
	Count int `json:"count"`
	Sum   int `json:"sum"`
}

func (projection *countingProjection) Reset() error {
	*projection = countingProjection{}
	return nil
}

func (projection *countingProjection) Restore(data json.RawMessage) error {
	return json.Unmarshal(data, projection)
}

func (projection *countingProjection) Apply(record Record) error {
	var payload struct {
		N int `json:"n"`
	}
	if err := json.Unmarshal(record.Payload, &payload); err != nil {
		return err
	}
	if payload.N != 0 {
		projection.Count++
		projection.Sum += payload.N
	}
	return nil
}

func (projection *countingProjection) Checkpoint() (json.RawMessage, error) {
	return json.Marshal(projection)
}

func writeLegacyJournal(t *testing.T, path string, values ...int) []byte {
	t.Helper()
	data := []byte(`{"type":"session","id":"test"}` + "\n")
	for _, value := range values {
		line, err := json.Marshal(map[string]int{"n": value})
		if err != nil {
			t.Fatal(err)
		}
		data = append(data, line...)
		data = append(data, '\n')
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return data
}

func rawValue(t *testing.T, value int) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(map[string]int{"n": value})
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestJournalAppendsWithoutChangingCanonicalPrefixAndRestoresIndex(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.jsonl")
	prefix := writeLegacyJournal(t, path, 1, 2)
	identity := Identity{ID: "session-test", Generation: "generation-1"}
	projection := &countingProjection{}
	journal, err := Open(context.Background(), path, identity, projection, Options{FlushEvery: 1})
	if err != nil {
		t.Fatal(err)
	}
	if projection.Count != 2 || projection.Sum != 3 {
		t.Fatalf("legacy projection = %#v", projection)
	}
	commit, err := journal.Append(context.Background(), Guard{Cursor: journal.Head().Cursor}, rawValue(t, 3), rawValue(t, 4))
	if err != nil {
		t.Fatal(err)
	}
	if commit.Head.Cursor != 4 || projection.Count != 4 || projection.Sum != 10 {
		t.Fatalf("commit=%#v projection=%#v", commit, projection)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(after), string(prefix)) {
		t.Fatal("append changed the canonical legacy prefix")
	}
	if err := journal.Close(); err != nil {
		t.Fatal(err)
	}

	restored := &countingProjection{}
	reopened, err := Open(context.Background(), path, identity, restored, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if restored.Count != 4 || restored.Sum != 10 {
		t.Fatalf("restored projection = %#v", restored)
	}
	stats := reopened.ReplayStats()
	if !stats.IndexLoaded || stats.IndexRebuilt || stats.TailRecordsRead != 0 {
		t.Fatalf("reopen stats = %#v", stats)
	}
	index, err := os.ReadFile(SidecarPath(path))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(index), `"n":4`) || strings.Contains(string(index), `"n":3`) {
		t.Fatalf("derived index leaked canonical payload: %s", index)
	}
}

func TestJournalRefreshesIndependentHandleBeforeCAS(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.jsonl")
	writeLegacyJournal(t, path)
	identity := Identity{ID: "session-test", Generation: "generation-1"}
	firstProjection := &countingProjection{}
	first, err := Open(context.Background(), path, identity, firstProjection, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	secondProjection := &countingProjection{}
	second, err := Open(context.Background(), path, identity, secondProjection, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	stale := second.Head()
	if _, err := first.Append(context.Background(), Guard{Cursor: first.Head().Cursor}, rawValue(t, 7)); err != nil {
		t.Fatal(err)
	}
	_, err = second.Append(context.Background(), Guard{Cursor: stale.Cursor}, rawValue(t, 8))
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("stale append error = %v", err)
	}
	if secondProjection.Count != 1 || secondProjection.Sum != 7 {
		t.Fatalf("stale handle did not replay canonical tail: %#v", secondProjection)
	}
}

func TestJournalRepairsOnlyAnIncompleteFinalRecord(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "session.jsonl")
	writeLegacyJournal(t, path, 1)
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString(`{"n":`); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	identity := Identity{ID: "session-test", Generation: "generation-1"}
	projection := &countingProjection{}
	journal, err := Open(context.Background(), path, identity, projection, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := journal.Append(context.Background(), Guard{Cursor: journal.Head().Cursor}, rawValue(t, 2)); err != nil {
		t.Fatal(err)
	}
	if err := journal.Close(); err != nil {
		t.Fatal(err)
	}
	backups, err := filepath.Glob(path + ".incomplete-*")
	if err != nil || len(backups) != 1 {
		t.Fatalf("tail backups=%v error=%v", backups, err)
	}
	restored := &countingProjection{}
	reopened, err := Open(context.Background(), path, identity, restored, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if restored.Count != 2 || restored.Sum != 3 {
		t.Fatalf("repaired projection = %#v", restored)
	}
}

func TestJournalRebuildsCorruptIndexAndReadsSparseRange(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.jsonl")
	writeLegacyJournal(t, path)
	identity := Identity{ID: "session-test", Generation: "generation-1"}
	projection := &countingProjection{}
	journal, err := Open(context.Background(), path, identity, projection, Options{FlushEvery: 1, SparseEvery: 2})
	if err != nil {
		t.Fatal(err)
	}
	for value := 1; value <= 8; value++ {
		if _, err := journal.Append(context.Background(), Guard{Cursor: journal.Head().Cursor}, rawValue(t, value)); err != nil {
			t.Fatal(err)
		}
	}
	records, err := journal.ReadRange(context.Background(), Range{After: 5, Through: 8, Limit: 3})
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 3 {
		t.Fatalf("range records = %d", len(records))
	}
	if err := journal.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(SidecarPath(path), []byte("not-json\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	rebuiltProjection := &countingProjection{}
	rebuilt, err := Open(context.Background(), path, identity, rebuiltProjection, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer rebuilt.Close()
	if !rebuilt.ReplayStats().IndexRebuilt || rebuiltProjection.Count != 8 || rebuiltProjection.Sum != 36 {
		t.Fatalf("rebuild stats=%#v projection=%#v", rebuilt.ReplayStats(), rebuiltProjection)
	}
}

func TestJournalReplaysAndImmediatelyCheckpointsOnlyCompleteUnindexedTail(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.jsonl")
	writeLegacyJournal(t, path, 1)
	identity := Identity{ID: "session-test", Generation: "generation-1"}
	journal, err := Open(context.Background(), path, identity, &countingProjection{}, Options{FlushEvery: 1})
	if err != nil {
		t.Fatal(err)
	}
	if err := journal.Close(); err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString("{\"n\":2}\n"); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	projection := &countingProjection{}
	reopened, err := Open(context.Background(), path, identity, projection, Options{})
	if err != nil {
		t.Fatal(err)
	}
	stats := reopened.ReplayStats()
	if !stats.IndexLoaded || stats.IndexRebuilt || stats.TailRecordsRead != 1 || projection.Count != 2 || projection.Sum != 3 {
		t.Fatalf("stale index tail replay stats=%#v projection=%#v", stats, projection)
	}

	checkpointedProjection := &countingProjection{}
	checkpointed, err := Open(context.Background(), path, identity, checkpointedProjection, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer checkpointed.Close()
	checkpointedStats := checkpointed.ReplayStats()
	if !checkpointedStats.IndexLoaded || checkpointedStats.IndexRebuilt || checkpointedStats.TailRecordsRead != 0 {
		t.Fatalf("replayed tail was not checkpointed on close: %#v", checkpointedStats)
	}
	if checkpointedProjection.Count != 2 || checkpointedProjection.Sum != 3 {
		t.Fatalf("checkpointed tail projection = %#v", checkpointedProjection)
	}
	if err := reopened.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestJournalCleanCloseDoesNotRewriteIndex(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.jsonl")
	writeLegacyJournal(t, path, 1)
	identity := Identity{ID: "session-test", Generation: "generation-1"}
	journal, err := Open(context.Background(), path, identity, &countingProjection{}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if err := journal.Close(); err != nil {
		t.Fatal(err)
	}

	indexPath := SidecarPath(path)
	fixedTime := time.Unix(1_700_000_000, 0)
	if err := os.Chtimes(indexPath, fixedTime, fixedTime); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(context.Background(), path, identity, &countingProjection{}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if err := reopened.Close(); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(indexPath)
	if err != nil {
		t.Fatal(err)
	}
	if !info.ModTime().Equal(fixedTime) {
		t.Fatalf("clean close rewrote index: got=%s want=%s", info.ModTime(), fixedTime)
	}
}

func TestJournalRejectsCanonicalChecksumCorruption(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.jsonl")
	writeLegacyJournal(t, path)
	identity := Identity{ID: "session-test", Generation: "generation-1"}
	journal, err := Open(context.Background(), path, identity, &countingProjection{}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := journal.Append(context.Background(), Guard{Cursor: journal.Head().Cursor}, rawValue(t, 7)); err != nil {
		t.Fatal(err)
	}
	if err := journal.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	corrupt := bytes.Replace(data, []byte(`"n":7`), []byte(`"n":9`), 1)
	if bytes.Equal(corrupt, data) {
		t.Fatal("fixture did not contain transaction payload")
	}
	if err := os.WriteFile(path, corrupt, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(context.Background(), path, identity, &countingProjection{}, Options{}); err == nil || !strings.Contains(err.Error(), "checksum") {
		t.Fatalf("canonical checksum corruption error=%v", err)
	}
}

func TestJournalRejectsOldHandleAfterFileRecreation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.jsonl")
	writeLegacyJournal(t, path, 1)
	identity := Identity{ID: "session-test", Generation: "generation-1"}
	journal, err := Open(context.Background(), path, identity, &countingProjection{}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	replacement := []byte("{\"type\":\"session\",\"id\":\"replacement\"}\n")
	if err := os.WriteFile(path, replacement, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := journal.Append(context.Background(), Guard{Cursor: journal.Head().Cursor}, rawValue(t, 2)); err == nil || !strings.Contains(err.Error(), "incarnation changed") {
		t.Fatalf("old handle recreation error=%v", err)
	}
}
