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

	agent "github.com/alfredxw/denova/agent"

	"denova/internal/contextmaintenance"
	"denova/internal/conversationjournal"
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
		line, marshalErr := json.Marshal(agent.UserMessage(fmt.Sprintf("message-%04d-%s", index, body)))
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		if _, writeErr := writer.Write(append(line, '\n')); writeErr != nil {
			t.Fatal(writeErr)
		}
	}
	legacyCompaction, err := json.Marshal(ContextCompaction{
		Type: historyTypeCompaction,
		ID:   "legacy-long-compaction",
		CompactionCheckpoint: contextmaintenance.CompactionCheckpoint{
			AgentKind: "ide", Epoch: 1, Summary: "旧格式长会话摘要",
		},
		SourceStartIndex:   123,
		SourceEndIndex:     701,
		SourceMessageCount: 578,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write(append(legacyCompaction, '\n')); err != nil {
		t.Fatal(err)
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
	compaction, ok := sess.LatestContextCompaction("ide")
	if !ok || compaction.SourceStartCursor == 0 || compaction.SourceEndCursor == 0 {
		t.Fatalf("legacy compaction indexes were not migrated to stable cursors: %#v", compaction)
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
	if err := reopened.Append(agent.UserMessage("message-1000")); err != nil {
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
	for index := 0; index < runs; index++ {
		runID := fmt.Sprintf("run-%03d", index)
		if err := sess.Append(agent.UserMessage("user-" + runID)); err != nil {
			t.Fatal(err)
		}
		var canonical strings.Builder
		for displayIndex := 0; displayIndex < displaysPerRun; displayIndex++ {
			role := "thinking"
			content := fmt.Sprintf("think-%02d-%s", displayIndex, runID)
			if displayIndex%2 == 1 {
				role = "assistant"
				content = fmt.Sprintf("answer-%02d-%s", displayIndex, runID)
				canonical.WriteString(content)
			}
			event := DisplayEvent{ID: fmt.Sprintf("%s-display-%02d", runID, displayIndex), Role: role, Content: content, RunID: runID}
			if err := sess.AppendDisplayEvent(event); err != nil {
				t.Fatal(err)
			}
		}
		if err := sess.AppendWithMetadata(agent.AssistantMessage(canonical.String(), nil), MessageMetadata{RunID: runID}); err != nil {
			t.Fatal(err)
		}
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
		for displayIndex, entry := range group[1:] {
			wantRole := "thinking"
			if displayIndex%2 == 1 {
				wantRole = "assistant"
			}
			if entry.Role != wantRole || entry.DisplaySegmentID == "" {
				t.Fatalf("run %s display %d changed: %#v", runID, displayIndex, entry)
			}
		}
	}
}

func TestProjectionCheckpointResumesAssistantDigestWithoutLeakingContent(t *testing.T) {
	directory := t.TempDir()
	store, err := NewStore(directory)
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.GetOrCreate("digest-checkpoint")
	if err != nil {
		t.Fatal(err)
	}
	if err := sess.Append(agent.UserMessage("继续")); err != nil {
		t.Fatal(err)
	}
	if err := sess.AppendDisplayEvent(DisplayEvent{ID: "answer-1", Role: "assistant", Content: "第一段秘密正文", RunID: "run-checkpoint"}); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	indexBytes, err := os.ReadFile(conversationjournal.SidecarPath(filepath.Join(directory, "digest-checkpoint.jsonl")))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(indexBytes), "第一段秘密正文") {
		t.Fatal("assistant prose leaked into the projection checkpoint")
	}

	reopenedStore, err := NewStore(directory)
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := reopenedStore.Get("digest-checkpoint")
	if err != nil {
		t.Fatal(err)
	}
	if err := reopened.AppendDisplayEvent(DisplayEvent{ID: "answer-2", Role: "assistant", Content: "第二段秘密正文", RunID: "run-checkpoint"}); err != nil {
		t.Fatal(err)
	}
	if err := reopened.AppendWithMetadata(agent.AssistantMessage("第一段秘密正文第二段秘密正文", nil), MessageMetadata{RunID: "run-checkpoint"}); err != nil {
		t.Fatal(err)
	}
	page, err := reopened.ReadHistoryPage(context.Background(), -1, 10)
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 3 || len(page.Entries) != 3 {
		t.Fatalf("checkpointed segmented run produced duplicate canonical prose: %#v", page)
	}
	if page.Entries[0].Role != "user" || page.Entries[1].DisplaySegmentID == "" || page.Entries[2].DisplaySegmentID == "" {
		t.Fatalf("unexpected resumed run projection: %#v", page.Entries)
	}
}
