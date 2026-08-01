package interactive

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	agentcontext "denova/internal/agents/context"
)

func TestStoryHotAppendsPreserveCanonicalPrefixWithoutFullRewrite(t *testing.T) {
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{Title: "append journal", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	store.rewriteJSONL = func(string, []any) error {
		t.Fatal("hot story append unexpectedly rewrote the complete JSONL file")
		return nil
	}
	path := store.storyPath(story.ID)
	previous, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	appendCalls := 0
	store.appendStoryRecord = func(path string, record []byte) error {
		appendCalls++
		before, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if !bytes.Equal(before, previous) {
			t.Fatalf("canonical prefix changed before append %d", appendCalls)
		}
		if bytes.Count(record, []byte{'\n'}) != 0 {
			t.Fatalf("transaction %d is not one JSON record", appendCalls)
		}
		if err := appendStoryRecord(path, record); err != nil {
			return err
		}
		after, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if !bytes.HasPrefix(after, before) {
			t.Fatalf("append %d replaced prior story bytes", appendCalls)
		}
		previous = after
		return nil
	}

	for index := 0; index < 8; index++ {
		if _, err := store.AppendTurn(story.ID, AppendTurnRequest{
			BranchID: "main", User: "继续", Narrative: "追加回合 " + testStoryDecimal(index),
		}); err != nil {
			t.Fatal(err)
		}
	}
	if appendCalls != 8 {
		t.Fatalf("append calls = %d, want 8", appendCalls)
	}
	physicalLines := strings.Split(strings.TrimSpace(string(previous)), "\n")
	if len(physicalLines) != 9 {
		t.Fatalf("physical records = %d, want meta + 8 transactions", len(physicalLines))
	}
	for index, line := range physicalLines[1:] {
		if _, events, transaction, err := decodeStoryAppendTransaction([]byte(line)); err != nil || !transaction || len(events) != 1 {
			t.Fatalf("transaction %d decode: transaction=%v events=%d err=%v", index+1, transaction, len(events), err)
		}
	}
	snapshot, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Turns) != 8 {
		t.Fatalf("logical turns = %d, want 8", len(snapshot.Turns))
	}
	stats := store.LastStoryJournalReplayStats(story.ID)
	if stats.BytesRead != int64(len(previous)) || stats.RecordsRead != 9 || stats.TransactionsRead != 8 || stats.EventsRead != 8 {
		t.Fatalf("story replay stats = %#v, file bytes=%d", stats, len(previous))
	}
}

func TestStoryReaderAcceptsLegacyFlatEventsBeforeAppendTransactions(t *testing.T) {
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{Title: "legacy mixed", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	first, err := store.AppendTurn(story.ID, AppendTurnRequest{BranchID: "main", User: "旧", Narrative: "旧扁平事件"})
	if err != nil {
		t.Fatal(err)
	}
	meta, events, err := store.readStoryJournalLocked(story.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeJSONL(store.storyPath(story.ID), []any{meta, events[0].Raw}); err != nil {
		t.Fatal(err)
	}
	second, err := store.AppendTurn(story.ID, AppendTurnRequest{BranchID: "main", User: "新", Narrative: "新事务事件"})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Turns) != 2 || snapshot.Turns[0].ID != first.ID || snapshot.Turns[1].ID != second.ID {
		t.Fatalf("mixed story turns = %#v", snapshot.Turns)
	}
	stats := store.LastStoryJournalReplayStats(story.ID)
	if stats.TransactionsRead != 1 || stats.EventsRead != 2 {
		t.Fatalf("mixed replay stats = %#v", stats)
	}
}

func TestStoryReaderRepairsOnlySyntacticallyTornFinalTransaction(t *testing.T) {
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{Title: "torn append", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	turn, err := store.AppendTurn(story.ID, AppendTurnRequest{BranchID: "main", User: "继续", Narrative: "已提交"})
	if err != nil {
		t.Fatal(err)
	}
	path := store.storyPath(story.ID)
	committed, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	torn := append(append([]byte(nil), committed...), []byte(`{"journal":"denova.story.append","version":1`)...)
	if err := os.WriteFile(path, torn, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.readStoryJournalLocked(story.ID); err == nil {
		t.Fatal("query-only journal read repaired a torn tail")
	}
	unchanged, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(unchanged, torn) {
		t.Fatalf("query-only read modified torn bytes: err=%v", err)
	}
	snapshot, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Turns) != 1 || snapshot.Turns[0].ID != turn.ID {
		t.Fatalf("repaired story = %#v", snapshot.Turns)
	}
	repaired, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(repaired, committed) {
		t.Fatalf("repaired bytes differ from committed prefix")
	}
	backups, err := filepath.Glob(path + ".recovery.*.bak")
	if err != nil || len(backups) != 1 {
		t.Fatalf("recovery backups = %v err=%v", backups, err)
	}
	backup, err := os.ReadFile(backups[0])
	if err != nil || !bytes.Equal(backup, torn) {
		t.Fatalf("recovery backup did not preserve torn bytes: err=%v", err)
	}
}

func TestStoryReaderRejectsCompleteChecksumCorruptionWithoutRepair(t *testing.T) {
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{Title: "corrupt append", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.AppendTurn(story.ID, AppendTurnRequest{BranchID: "main", User: "继续", Narrative: "已提交"}); err != nil {
		t.Fatal(err)
	}
	path := store.storyPath(story.ID)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	var transaction storyAppendTransaction
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &transaction); err != nil {
		t.Fatal(err)
	}
	transaction.Checksum = strings.Repeat("0", 64)
	corrupt, err := json.Marshal(transaction)
	if err != nil {
		t.Fatal(err)
	}
	lines[len(lines)-1] = string(corrupt)
	corruptFile := []byte(strings.Join(lines, "\n") + "\n")
	if err := os.WriteFile(path, corruptFile, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Snapshot(story.ID, "main"); err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("snapshot error = %v, want checksum mismatch", err)
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(after, corruptFile) {
		t.Fatalf("corrupt complete transaction was modified: err=%v", err)
	}
	backups, _ := filepath.Glob(path + ".recovery.*.bak")
	if len(backups) != 0 {
		t.Fatalf("complete checksum corruption created recovery backup: %v", backups)
	}
}

func TestStoryAppendReadbackDoesNotMaskFileSyncFailure(t *testing.T) {
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{Title: "sync failure", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	intent, err := NewPlayerInputIntent(DomainCommitIdentity{
		CommandID: "sync-command", OperationID: "sync-operation", Cycle: 1,
	}, "main", "继续")
	if err != nil {
		t.Fatal(err)
	}
	syncErr := errors.New("simulated story fsync failure")
	store.appendStoryRecord = func(path string, record []byte) error {
		if err := appendStoryRecord(path, record); err != nil {
			return err
		}
		return &storyAppendRecordError{syncErr: syncErr}
	}
	if _, err := store.CommitPlayerInput(story.ID, intent); !errors.Is(err, syncErr) {
		t.Fatalf("commit error = %v, want sync failure", err)
	}
	store.appendStoryRecord = nil
	receipt, err := store.CommitPlayerInput(story.ID, intent)
	if err != nil {
		t.Fatalf("exact retry after conservative sync failure: %v", err)
	}
	if receipt.Identity != intent.Identity || receipt.Revision == "" {
		t.Fatalf("replayed receipt = %#v", receipt)
	}
}

func TestStoryMutationLeaseLinearizesConcurrentCASAcrossStoreInstances(t *testing.T) {
	root := t.TempDir()
	firstStore := NewStore(root)
	story, err := firstStore.CreateStory(CreateStoryRequest{Title: "shared lease", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	turn, err := firstStore.AppendTurn(story.ID, AppendTurnRequest{BranchID: "main", User: "开始", Narrative: "共同父节点"})
	if err != nil {
		t.Fatal(err)
	}
	secondStore := NewStore(root)
	hookReached := make(chan struct{}, 2)
	allowAppend := make(chan struct{})
	hook := func(path string, record []byte) error {
		hookReached <- struct{}{}
		<-allowAppend
		return appendStoryRecord(path, record)
	}
	firstStore.appendStoryRecord = hook
	secondStore.appendStoryRecord = hook

	expected := turn.ID
	results := make(chan error, 2)
	start := make(chan struct{})
	appendCompaction := func(store *Store, id string) {
		defer func() {
			if recovered := recover(); recovered != nil {
				results <- fmt.Errorf("append compaction panic: %v", recovered)
			}
		}()
		<-start
		_, err := store.AppendContextCompaction(story.ID, "main", ContextCompactionEvent{
			ID: id, CompactionCheckpoint: agentcontext.CompactionCheckpoint{Summary: id, RetainedTurns: 1},
			SourceTurnCount: 1, ExpectedParentID: &expected,
		})
		results <- err
	}
	go appendCompaction(firstStore, "compaction-a")
	go appendCompaction(secondStore, "compaction-b")
	close(start)
	<-hookReached
	select {
	case <-hookReached:
		close(allowAppend)
		t.Fatal("two Store instances entered append before the first CAS transaction released its story lease")
	case <-time.After(25 * time.Millisecond):
		close(allowAppend)
	}

	firstErr, secondErr := <-results, <-results
	successes := 0
	conflicts := 0
	for _, err := range []error{firstErr, secondErr} {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, ErrStoryContextRevisionConflict):
			conflicts++
		default:
			t.Fatalf("unexpected concurrent CAS error: %v", err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("concurrent CAS results: success=%d conflict=%d errors=[%v, %v]", successes, conflicts, firstErr, secondErr)
	}
}

func testStoryDecimal(value int) string {
	return string(rune('0' + value))
}
