package session

import (
	"context"
	"os"
	"strings"
	"testing"

	agentschema "github.com/alfredxw/denova/agent/schema"
)

func TestHistorySearchCacheTracksExternalMutationsWithoutOpeningSessions(t *testing.T) {
	dir := t.TempDir()
	writer, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = writer.Close() })
	sess, err := writer.GetOrCreate("search")
	if err != nil {
		t.Fatal(err)
	}
	var revision uint64
	appendSegmentedHistoryRun(t, sess, "old", 260, &revision)
	reader, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reader.Close() })
	search := func(query string, found bool) {
		t.Helper()
		excerpt, err := reader.SearchHistoryContent(context.Background(), "search", query)
		if err != nil || (excerpt != "") != found {
			t.Fatalf("query=%q excerpt=%q err=%v", query, excerpt, err)
		}
		if found && !strings.Contains(strings.ToLower(excerpt), query) {
			t.Fatal("excerpt lost query")
		}
	}
	search("answer-01-old", true)
	search("think-00-old", false)
	search("new beacon", false)
	if len(reader.cache) != 0 {
		t.Fatal("search materialized an execution Session")
	}
	if err := sess.Append(agentschema.UserMessage("new Beacon")); err != nil {
		t.Fatal(err)
	}
	search("new beacon", true)
	if err := sess.AppendDisplayEvent(DisplayEvent{ID: "preview", Role: "assistant", Content: "progress preview", DisplayPhase: DisplayPhaseProgress}); err != nil {
		t.Fatal(err)
	}
	search("progress preview", true)
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(sess.filePath); err != nil {
		t.Fatal(err)
	}
	if _, err := reader.SearchHistoryContent(context.Background(), "search", "new beacon"); !os.IsNotExist(err) {
		t.Fatalf("deleted journal returned cached content: %v", err)
	}
}
