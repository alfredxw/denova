package filejournal

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	runstate "github.com/alfredxw/denova/agent/runtime"
	"os"
	"strconv"
	"testing"
)

func TestFileJournalStreamingReplayReportsIOAndLetsReducerStayBounded(t *testing.T) {
	t.Parallel()

	journal := newTestFileJournal(t)
	const records = 512
	encoded := make([]byte, 0, records*256)
	for index := 0; index < records; index++ {
		cursor := runstate.Cursor(index + 1)
		line := encodeTestJournalTransaction(t, cursor, runstate.CommandID("command-"+testDecimal(index)))
		encoded = append(encoded, line...)
		encoded = append(encoded, '\n')
	}
	if err := os.WriteFile(journal.path, encoded, 0o600); err != nil {
		t.Fatal(err)
	}

	retained := make([]runstate.Event, 0, 3)
	maxRetained := 0
	stats, err := journal.Replay(context.Background(), func(event runstate.Event) error {
		retained = append(retained, event)
		if len(retained) > 3 {
			copy(retained, retained[len(retained)-3:])
			retained = retained[:3]
		}
		if len(retained) > maxRetained {
			maxRetained = len(retained)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if stats.BytesRead != int64(len(encoded)) || stats.RecordsRead != records || stats.EventsRead != records {
		t.Fatalf("replay stats = %#v, bytes=%d records=%d", stats, len(encoded), records)
	}
	if maxRetained != 3 || len(retained) != 3 || retained[2].Cursor != records {
		t.Fatalf("bounded reducer retained=%d events=%#v", maxRetained, retained)
	}
}

func TestFileJournalColdCommandLookupUsesValidatedPersistentIndex(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store, err := NewStore(root)
	if err != nil {
		t.Fatal(err)
	}
	opened, err := store.OpenJournal(context.Background(), t.Name())
	if err != nil {
		t.Fatal(err)
	}
	firstJournal := opened.(*journal)
	for cursor := runstate.Cursor(0); cursor < 12; cursor++ {
		appendTestJournalEvent(t, firstJournal, cursor, runstate.CommandID("indexed-"+testDecimal(int(cursor))))
	}
	if err := firstJournal.Close(); err != nil {
		t.Fatal(err)
	}

	reopenedRaw, err := store.OpenJournal(context.Background(), t.Name())
	if err != nil {
		t.Fatal(err)
	}
	reopened := reopenedRaw.(*journal)
	t.Cleanup(func() { _ = reopened.Close() })
	replayOpens := 0
	openReplayFile := reopened.openReplayFile
	reopened.openReplayFile = func(path string) (*os.File, error) {
		replayOpens++
		return openReplayFile(path)
	}
	record, found, err := reopened.LookupCommand(context.Background(), "indexed-0")
	if err != nil {
		t.Fatal(err)
	}
	if !found || record.Receipt.Cursor != 1 || record.Receipt.OperationID != "indexed-0" {
		t.Fatalf("indexed command = %#v found=%v", record, found)
	}
	if replayOpens != 0 {
		t.Fatalf("cold lookup streamed the complete main journal %d times", replayOpens)
	}
}

func TestFileJournalCommandIndexUsesAppendOnlyBoundedDeltas(t *testing.T) {
	t.Parallel()

	journal := newTestFileJournal(t)
	for cursor := runstate.Cursor(0); cursor < 8; cursor++ {
		appendTestJournalEvent(t, journal, cursor, runstate.CommandID("delta-"+testDecimal(int(cursor))))
	}
	data, err := os.ReadFile(journal.commandIndexPath())
	if err != nil {
		t.Fatal(err)
	}
	lines := bytes.Split(bytes.TrimSpace(data), []byte{'\n'})
	if len(lines) != 8 {
		t.Fatalf("index records = %d, want one snapshot + seven deltas", len(lines))
	}
	for index, line := range lines {
		var record journalCommandIndex
		if err := json.Unmarshal(line, &record); err != nil {
			t.Fatal(err)
		}
		if index == 0 && !record.Snapshot {
			t.Fatal("command index does not start with a compact snapshot")
		}
		if index > 0 && (record.Snapshot || len(record.Commands) != 1) {
			t.Fatalf("index delta %d rewrote historical commands: %#v", index, record)
		}
	}
}

func TestFileJournalCorruptCommandIndexFallsBackToCanonicalStreamingReplay(t *testing.T) {
	t.Parallel()

	journal := newTestFileJournal(t)
	appendTestJournalEvent(t, journal, 0, "canonical")
	if err := os.WriteFile(journal.commandIndexPath(), []byte(`{"version":1,"checksum":"corrupt"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	journal.initialized = false
	journal.indexReady = false
	journal.commandIndex = nil
	replayOpens := 0
	openReplayFile := journal.openReplayFile
	journal.openReplayFile = func(path string) (*os.File, error) {
		replayOpens++
		return openReplayFile(path)
	}
	record, found, err := journal.LookupCommand(context.Background(), "canonical")
	if err != nil {
		t.Fatal(err)
	}
	if !found || record.Receipt.Cursor != 1 {
		t.Fatalf("rebuilt command = %#v found=%v", record, found)
	}
	if replayOpens != 1 {
		t.Fatalf("canonical rebuild replay opens = %d, want 1", replayOpens)
	}
}

func encodeTestJournalTransaction(t *testing.T, cursor runstate.Cursor, commandID runstate.CommandID) []byte {
	t.Helper()
	event, err := runstate.EncodeJournalEvent(runstate.Event{
		Cursor: cursor, Durability: runstate.EventDurable,
		Payload: runstate.CommandAcceptedEvent{
			CommandID: commandID, CommandKind: "test", OperationID: runstate.OperationID(commandID), Fingerprint: string(commandID),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	body := journalTransactionBody{Version: journalVersion, Start: cursor, End: cursor, Events: []runstate.JournalEvent{event}}
	bodyJSON, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(bodyJSON)
	line, err := json.Marshal(journalTransaction{
		Version: body.Version, Start: body.Start, End: body.End, Events: body.Events,
		Checksum: hex.EncodeToString(digest[:]),
	})
	if err != nil {
		t.Fatal(err)
	}
	return line
}

func testDecimal(value int) string {
	return strconv.Itoa(value)
}
