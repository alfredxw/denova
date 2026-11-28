package session

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"denova/internal/agents/conversationjournal"

	agentschema "github.com/alfredxw/denova/agent/schema"
)

func TestHistoryPageReadsIndexedRangesWithoutOmissions(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "long-session.jsonl")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	writer := bufio.NewWriterSize(file, 1024*1024)
	body := strings.Repeat("正文", 2048)
	for index := 0; index < 1_000; index++ {
		line, marshalErr := json.Marshal(agentschema.UserMessage(fmt.Sprintf("message-%04d-%s", index, body)))
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
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	store, err := NewStore(directory)
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.Get("long-session")
	if err != nil {
		t.Fatal(err)
	}
	if len(sess.messages) > sessionRecentTransactionLimit {
		t.Fatalf("resident messages=%d want<=%d", len(sess.messages), sessionRecentTransactionLimit)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	reopenedStore, err := NewStore(directory)
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := reopenedStore.Get("long-session")
	if err != nil {
		t.Fatal(err)
	}
	stats := reopened.JournalReplayStats()
	if !stats.IndexLoaded || stats.IndexRebuilt || stats.BytesRead != 0 {
		t.Fatalf("indexed reopen stats=%#v", stats)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if stats.LastRangeBytesRead >= info.Size()/2 {
		t.Fatalf("recent materialization read=%d journal=%d", stats.LastRangeBytesRead, info.Size())
	}

	before := -1
	seen := make(map[string]bool, 1_000)
	for {
		page, pageErr := reopened.ReadHistoryPage(context.Background(), before, 73)
		if pageErr != nil {
			t.Fatal(pageErr)
		}
		for _, entry := range page.Entries {
			prefix := strings.SplitN(entry.Content, "-", 3)
			if len(prefix) < 2 || seen[prefix[1]] {
				t.Fatalf("duplicate or malformed paged entry: %q", entry.Content)
			}
			seen[prefix[1]] = true
		}
		if !page.HasMore {
			break
		}
		before = page.NextBefore
	}
	if len(seen) != 1_000 {
		t.Fatalf("paged entries=%d want=1000", len(seen))
	}

	prefix, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	readsBeforeAppend := reopened.JournalReplayStats().BytesRead
	if err := reopened.Append(agentschema.UserMessage("message-1000")); err != nil {
		t.Fatal(err)
	}
	if reopened.JournalReplayStats().BytesRead != readsBeforeAppend {
		t.Fatal("hot append read the canonical history prefix")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(after), string(prefix)) {
		t.Fatal("append changed the canonical JSONL prefix")
	}
	index, err := os.ReadFile(conversationjournal.SidecarPath(path))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(index), "message-0999") || int64(len(index))*100 >= info.Size() {
		t.Fatalf("index leaked content or exceeded one percent: index=%d journal=%d", len(index), info.Size())
	}
	if err := reopenedStore.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestHistoryPageDoesNotSplitSingleLongRun(t *testing.T) {
	directory := t.TempDir()
	store, err := NewStore(directory)
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.GetOrCreate("single-long-run")
	if err != nil {
		t.Fatal(err)
	}
	var contextRevision uint64
	appendSegmentedHistoryRun(t, sess, "run-long", 138, &contextRevision)

	page, err := sess.ReadHistoryPage(context.Background(), -1, 100)
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 139 || len(page.Entries) != 139 {
		t.Fatalf("long turn page rows=%d total=%d want=139", len(page.Entries), page.Total)
	}
	if page.Entries[0].Role != "user" || page.Entries[0].Content != "user-run-long" {
		t.Fatalf("long turn lost its user boundary: %#v", page.Entries[0])
	}
	if page.NextBefore != 0 || page.HasMore {
		t.Fatalf("single complete turn exposed false earlier history: before=%d has_more=%t", page.NextBefore, page.HasMore)
	}
}

func TestHistoryPageKeepsSegmentedRunsWholeAcrossSparseAnchors(t *testing.T) {
	directory := t.TempDir()
	store, err := NewStore(directory)
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.GetOrCreate("segmented-history")
	if err != nil {
		t.Fatal(err)
	}
	const (
		runs           = 13
		displaysPerRun = 19
		visiblePerRun  = displaysPerRun + 1
	) // The 260 visible rows cross the 256-row anchor interval.
	var contextRevision uint64
	for index := 0; index < runs; index++ {
		appendSegmentedHistoryRun(t, sess, fmt.Sprintf("run-%03d", index), displaysPerRun, &contextRevision)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	reopenedStore, err := NewStore(directory)
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := reopenedStore.Get("segmented-history")
	if err != nil {
		t.Fatal(err)
	}
	before := -1
	all := make([]HistoryEntry, 0, runs*visiblePerRun)
	for {
		page, pageErr := reopened.ReadHistoryPage(context.Background(), before, 37)
		if pageErr != nil {
			t.Fatal(pageErr)
		}
		if len(page.Entries) > 0 && page.Entries[0].Role != "user" {
			t.Fatalf("history page split a turn at role %q", page.Entries[0].Role)
		}
		all = append(append([]HistoryEntry(nil), page.Entries...), all...)
		if !page.HasMore {
			if page.Total != runs*visiblePerRun {
				t.Fatalf("visible total=%d want=%d", page.Total, runs*visiblePerRun)
			}
			break
		}
		before = page.NextBefore
	}
	if len(all) != runs*visiblePerRun {
		t.Fatalf("paged visible rows=%d want=%d", len(all), runs*visiblePerRun)
	}
	for index := 0; index < runs; index++ {
		runID := fmt.Sprintf("run-%03d", index)
		group := all[index*visiblePerRun : index*visiblePerRun+visiblePerRun]
		if group[0].Role != "user" || group[0].Content != "user-"+runID {
			t.Fatalf("run %s user row changed: %#v", runID, group[0])
		}
		for displayIndex, entry := range group[1 : len(group)-1] {
			wantRole := "thinking"
			if displayIndex%2 == 1 {
				wantRole = "assistant"
			}
			if entry.Role != wantRole || entry.DisplaySegmentID == "" {
				t.Fatalf("run %s display %d changed: %#v", runID, displayIndex, entry)
			}
		}
		finalEntry := group[len(group)-1]
		if finalEntry.Role != "assistant" || finalEntry.DisplayPhase != DisplayPhaseFinal {
			t.Fatalf("run %s canonical final changed: %#v", runID, finalEntry)
		}
	}
}

// appendSegmentedHistoryRun keeps each logical run in one physical journal
// transaction. Under v3 single-write semantics, intermediate thinking and
// progress assistant segments are persisted as immutable displays and the final
// reply is persisted once as the canonical assistant message.
func appendSegmentedHistoryRun(t *testing.T, sess *Session, runID string, displaysPerRun int, contextRevision *uint64) {
	t.Helper()
	now := time.Now().UTC()
	*contextRevision++
	user := agentschema.UserMessage("user-" + runID)
	intermediateCount := displaysPerRun - 1
	if intermediateCount < 0 {
		intermediateCount = 0
	}
	records := make([]any, 0, intermediateCount+2)
	records = append(records, messageRecord{
		Type: historyTypeMessage, CreatedAt: now, Message: *user,
		MessageMetadata: MessageMetadata{RunID: runID, ContextRevision: *contextRevision},
	})
	for displayIndex := 0; displayIndex < intermediateCount; displayIndex++ {
		role := "thinking"
		content := fmt.Sprintf("think-%02d-%s", displayIndex, runID)
		phase := ""
		if displayIndex%2 == 1 {
			role = "assistant"
			content = fmt.Sprintf("answer-%02d-%s", displayIndex, runID)
			phase = DisplayPhaseProgress
		}
		records = append(records, displayRecord{
			Type: historyTypeDisplay, RecordID: newDisplayRecordID(),
			DisplayEvent: DisplayEvent{
				ID: fmt.Sprintf("%s-display-%02d", runID, displayIndex), Role: role,
				DisplayPhase: phase, Content: content, RunID: runID, CreatedAt: now,
			},
		})
	}
	*contextRevision++
	assistant := agentschema.AssistantMessage(fmt.Sprintf("final-answer-%s", runID), nil)
	assistant.ReasoningContent = fmt.Sprintf("final-think-%s", runID)
	records = append(records, messageRecord{
		Type: historyTypeMessage, CreatedAt: now, Message: *assistant,
		MessageMetadata: MessageMetadata{RunID: runID, DisplayPhase: DisplayPhaseFinal, ContextRevision: *contextRevision},
	})
	if err := sess.withCanonicalMutation(context.Background(), "append segmented history fixture", func() error {
		_, err := sess.appendJournalRecordsLocked(records...)
		return err
	}); err != nil {
		t.Fatal(err)
	}
}
