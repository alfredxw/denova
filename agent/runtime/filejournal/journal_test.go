package filejournal

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	runstate "github.com/alfredxw/denova/agent/runtime"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFileJournalAppendUsesLoadedCursorWithoutRescanning(t *testing.T) {
	t.Parallel()

	journal := newTestFileJournal(t)
	openReplayFile := journal.openReplayFile
	reads := 0
	journal.openReplayFile = func(path string) (*os.File, error) {
		reads++
		return openReplayFile(path)
	}
	if _, err := journal.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	for cursor := runstate.Cursor(0); cursor < 8; cursor++ {
		appendTestJournalEvent(t, journal, cursor, runstate.CommandID(string(rune('a'+cursor))))
	}
	if reads != 1 {
		t.Fatalf("journal file reads = %d, want one initial scan", reads)
	}
}

func TestFileJournalReconcilesCommittedAppendAfterAmbiguousCloseError(t *testing.T) {
	t.Parallel()

	journal := newTestFileJournal(t)
	if _, err := journal.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	ambiguous := errors.New("close result was lost")
	closeFile := journal.closeFile
	journal.closeFile = func(file *os.File) error {
		if err := closeFile(file); err != nil {
			return err
		}
		return ambiguous
	}
	committed, err := journal.Append(context.Background(), 0, []runstate.EventPayload{runstate.CommandAcceptedEvent{
		CommandID: "ambiguous", CommandKind: "test", OperationID: "operation", Fingerprint: "ambiguous",
	}})
	if err != nil {
		t.Fatalf("append should reconcile the complete transaction: %v", err)
	}
	if len(committed) != 1 || committed[0].Cursor != 1 {
		t.Fatalf("committed events = %#v", committed)
	}

	journal.closeFile = closeFile
	appendTestJournalEvent(t, journal, 1, "after-ambiguous")
	events, err := journal.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 {
		t.Fatalf("replayed events = %d, want 2", len(events))
	}
}

func TestFileJournalDoesNotMaskFileSyncFailureWithReadBack(t *testing.T) {
	t.Parallel()

	journal := newTestFileJournal(t)
	if _, err := journal.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	syncErr := errors.New("file sync failed")
	journal.syncFile = func(*os.File) error { return syncErr }
	if _, err := journal.Append(context.Background(), 0, []runstate.EventPayload{runstate.CommandAcceptedEvent{
		CommandID: "sync-error", CommandKind: "test", OperationID: "operation", Fingerprint: "sync-error",
	}}); !errors.Is(err, syncErr) {
		t.Fatalf("append error = %v, want file sync failure", err)
	}
}

func TestFileJournalReturnsDirectorySyncFailure(t *testing.T) {
	t.Parallel()

	journal := newTestFileJournal(t)
	if _, err := journal.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	directoryErr := errors.New("directory sync failed")
	syncDirectory := journal.syncDirectory
	journal.syncDirectory = func(string) error { return directoryErr }
	if _, err := journal.Append(context.Background(), 0, []runstate.EventPayload{runstate.CommandAcceptedEvent{
		CommandID: "directory-error", CommandKind: "test", OperationID: "operation", Fingerprint: "directory-error",
	}}); !errors.Is(err, directoryErr) {
		t.Fatalf("append error = %v, want directory sync failure", err)
	}

	journal.syncDirectory = syncDirectory
	events, err := journal.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Cursor != 1 {
		t.Fatalf("directory-sync-uncertain replay = %#v", events)
	}
}

func TestFileJournalSyncsDirectoryOnlyWhenTailIsCreated(t *testing.T) {
	t.Parallel()

	journal := newTestFileJournal(t)
	if _, err := journal.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	syncDirectory := journal.syncDirectory
	directorySyncs := 0
	journal.syncDirectory = func(path string) error {
		directorySyncs++
		return syncDirectory(path)
	}
	appendTestJournalEvent(t, journal, 0, "first")
	appendTestJournalEvent(t, journal, 1, "second")
	if directorySyncs != 1 {
		t.Fatalf("directory syncs = %d, want one namespace sync for the created tail", directorySyncs)
	}
}

func TestFileJournalRepairsOnlySyntacticallyTornFinalRecord(t *testing.T) {
	t.Parallel()

	journal := newTestFileJournal(t)
	appendTestJournalEvent(t, journal, 0, "first")
	committed, err := os.ReadFile(journal.path)
	if err != nil {
		t.Fatalf("read committed journal: %v", err)
	}
	torn := append(append([]byte(nil), committed...), []byte(`{"version":1,"start":2`)...)
	if err := os.WriteFile(journal.path, torn, 0o600); err != nil {
		t.Fatalf("write torn journal: %v", err)
	}

	events, err := journal.Load(context.Background())
	if err != nil {
		t.Fatalf("load torn journal: %v", err)
	}
	if len(events) != 1 || events[0].Cursor != 1 {
		t.Fatalf("replayed events = %#v, want cursor 1", events)
	}
	repaired, err := os.ReadFile(journal.path)
	if err != nil {
		t.Fatalf("read repaired journal: %v", err)
	}
	if !bytes.Equal(repaired, committed) {
		t.Fatalf("repaired journal differs from committed prefix\n got: %q\nwant: %q", repaired, committed)
	}
	backups, err := filepath.Glob(journal.path + ".recovery.*.bak")
	if err != nil {
		t.Fatalf("glob recovery backups: %v", err)
	}
	if len(backups) != 1 {
		t.Fatalf("recovery backups = %v, want exactly one", backups)
	}
	backup, err := os.ReadFile(backups[0])
	if err != nil {
		t.Fatalf("read recovery backup: %v", err)
	}
	if !bytes.Equal(backup, torn) {
		t.Fatalf("backup does not contain the original journal\n got: %q\nwant: %q", backup, torn)
	}
}

func TestFileJournalRejectsCompleteCorruptFinalRecordWithoutNewline(t *testing.T) {
	t.Parallel()

	journal := newTestFileJournal(t)
	appendTestJournalEvent(t, journal, 0, "first")
	content, err := os.ReadFile(journal.path)
	if err != nil {
		t.Fatalf("read journal: %v", err)
	}
	var transaction journalTransaction
	if err := json.Unmarshal(bytes.TrimSuffix(content, []byte{'\n'}), &transaction); err != nil {
		t.Fatalf("decode committed transaction: %v", err)
	}
	transaction.Checksum = strings.Repeat("0", sha256HexLength)
	corrupt, err := json.Marshal(transaction)
	if err != nil {
		t.Fatalf("encode corrupt transaction: %v", err)
	}
	if err := os.WriteFile(journal.path, corrupt, 0o600); err != nil {
		t.Fatalf("write corrupt journal: %v", err)
	}

	if _, err := journal.Load(context.Background()); err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("load error = %v, want checksum mismatch", err)
	}
	after, err := os.ReadFile(journal.path)
	if err != nil {
		t.Fatalf("read rejected journal: %v", err)
	}
	if !bytes.Equal(after, corrupt) {
		t.Fatalf("complete corrupt record was modified: got %q, want %q", after, corrupt)
	}
	assertNoRecoveryBackup(t, journal.path)
}

func TestFileJournalRejectsCorruptMiddleRecord(t *testing.T) {
	t.Parallel()

	journal := newTestFileJournal(t)
	appendTestJournalEvent(t, journal, 0, "first")
	appendTestJournalEvent(t, journal, 1, "second")
	content, err := os.ReadFile(journal.path)
	if err != nil {
		t.Fatalf("read journal: %v", err)
	}
	lines := bytes.Split(content, []byte{'\n'})
	lines[0] = []byte(`{"version":`)
	corrupt := bytes.Join(lines, []byte{'\n'})
	if err := os.WriteFile(journal.path, corrupt, 0o600); err != nil {
		t.Fatalf("write corrupt journal: %v", err)
	}

	if _, err := journal.Load(context.Background()); err == nil || !strings.Contains(err.Error(), "line 1") {
		t.Fatalf("load error = %v, want strict middle-record error", err)
	}
	after, err := os.ReadFile(journal.path)
	if err != nil {
		t.Fatalf("read rejected journal: %v", err)
	}
	if !bytes.Equal(after, corrupt) {
		t.Fatalf("middle corruption was modified: got %q, want %q", after, corrupt)
	}
	assertNoRecoveryBackup(t, journal.path)
}

const sha256HexLength = 64

func newTestFileJournal(t *testing.T) *journal {
	t.Helper()
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("new file journal store: %v", err)
	}
	opened, err := store.OpenJournal(context.Background(), t.Name())
	if err != nil {
		t.Fatalf("open file journal: %v", err)
	}
	journal, ok := opened.(*journal)
	if !ok {
		t.Fatalf("journal type = %T, want *journal", opened)
	}
	t.Cleanup(func() { _ = journal.Close() })
	return journal
}

func appendTestJournalEvent(t *testing.T, journal *journal, expected runstate.Cursor, id runstate.CommandID) {
	t.Helper()
	_, err := journal.Append(context.Background(), expected, []runstate.EventPayload{runstate.CommandAcceptedEvent{
		CommandID: id, CommandKind: "test", OperationID: runstate.OperationID(id), Fingerprint: string(id),
	}})
	if err != nil {
		t.Fatalf("append event at %d: %v", expected, err)
	}
}

func assertNoRecoveryBackup(t *testing.T, path string) {
	t.Helper()
	backups, err := filepath.Glob(path + ".recovery.*.bak")
	if err != nil {
		t.Fatalf("glob recovery backups: %v", err)
	}
	if len(backups) != 0 {
		t.Fatalf("unexpected recovery backups: %v", backups)
	}
}
