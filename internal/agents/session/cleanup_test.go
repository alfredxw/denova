package session

import (
	"errors"
	"reflect"
	"testing"

	agent "github.com/alfredxw/denova/agent"
)

func TestToolResultCleanupRoundTripsWithoutChangingRawOrDisplayHistory(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.Create("cleanup round trip")
	if err != nil {
		t.Fatal(err)
	}
	appendCleanupFixtureMessages(t, sess)
	rawBefore := sess.GetEffectiveMessages()
	historyBefore := sess.History()
	cursor := sess.ContextCursor()
	record := cleanupFixtureRecord("cleanup-1", "[Read result moved to artifact: chapters/one.md]")

	committed, err := sess.AppendToolResultCleanupAt(cursor, record)
	if err != nil {
		t.Fatal(err)
	}
	if committed.EarliestChanged != 2 || committed.ContextRevision != cursor.Revision+1 {
		t.Fatalf("unexpected committed cleanup: %#v", committed)
	}
	if got := sess.GetEffectiveMessages(); !reflect.DeepEqual(got, rawBefore) {
		t.Fatalf("cleanup changed canonical raw messages:\nbefore=%#v\nafter=%#v", rawBefore, got)
	}
	if got := sess.History(); !reflect.DeepEqual(got, historyBefore) {
		t.Fatalf("cleanup changed display history:\nbefore=%#v\nafter=%#v", historyBefore, got)
	}
	snapshot, err := sess.SnapshotContext("ide")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.ToolResultCleanup == nil || !reflect.DeepEqual(*snapshot.ToolResultCleanup, committed) {
		t.Fatalf("snapshot cleanup = %#v, want %#v", snapshot.ToolResultCleanup, committed)
	}
	// Snapshot callers cannot mutate the canonical projection through its slice.
	snapshot.ToolResultCleanup.Replacements[0].Placeholder = "mutated"
	if latest, ok := sess.LatestToolResultCleanup("ide"); !ok || latest.Replacements[0].Placeholder != record.Replacements[0].Placeholder {
		t.Fatalf("cleanup projection was aliased: %#v", latest)
	}

	id := sess.ID
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reloadedStore, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer reloadedStore.Close()
	reloaded, err := reloadedStore.Get(id)
	if err != nil {
		t.Fatal(err)
	}
	latest, ok := reloaded.LatestToolResultCleanup("ide")
	if !ok || !reflect.DeepEqual(latest, committed) {
		t.Fatalf("reloaded cleanup = %#v, want %#v", latest, committed)
	}
	if got := reloaded.GetEffectiveMessages(); !reflect.DeepEqual(got, rawBefore) {
		t.Fatalf("reload changed canonical raw messages:\nbefore=%#v\nafter=%#v", rawBefore, got)
	}
}

func TestToolResultCleanupCASReconcilesExactRetryAcrossSessionInstances(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	primary, err := store.Create("cleanup CAS")
	if err != nil {
		t.Fatal(err)
	}
	appendCleanupFixtureMessages(t, primary)
	staleStore, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer staleStore.Close()
	stale, err := staleStore.Get(primary.ID)
	if err != nil {
		t.Fatal(err)
	}
	cursor := stale.ContextCursor()
	record := cleanupFixtureRecord("cleanup-cas", "[Read result available at chapters/one.md]")

	committed, err := primary.AppendToolResultCleanupAt(primary.ContextCursor(), record)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := stale.AppendToolResultCleanupAt(cursor, record)
	if err != nil {
		t.Fatalf("exact stale retry failed: %v", err)
	}
	if !reflect.DeepEqual(replayed, committed) {
		t.Fatalf("exact retry differs:\nfirst=%#v\nretry=%#v", committed, replayed)
	}

	conflicting := record
	conflicting.Replacements = append([]ToolResultReplacement(nil), record.Replacements...)
	conflicting.Replacements[0].Placeholder = "different"
	if _, err := stale.AppendToolResultCleanupAt(cursor, conflicting); !errors.Is(err, ErrDomainCommitIdentityConflict) {
		t.Fatalf("same id with different cleanup error = %v, want %v", err, ErrDomainCommitIdentityConflict)
	}
	distinct := record
	distinct.ID = "cleanup-stale-distinct"
	if _, err := stale.AppendToolResultCleanupAt(cursor, distinct); !errors.Is(err, ErrContextRevisionConflict) {
		t.Fatalf("stale distinct cleanup error = %v, want %v", err, ErrContextRevisionConflict)
	}
}

func TestToolResultCleanupIsInvalidatedByCompactionAndClearBoundaries(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	sess, err := store.Create("cleanup boundaries")
	if err != nil {
		t.Fatal(err)
	}
	appendCleanupFixtureMessages(t, sess)
	if _, err := sess.AppendToolResultCleanupAt(sess.ContextCursor(), cleanupFixtureRecord("cleanup-boundary", "[read result elided]")); err != nil {
		t.Fatal(err)
	}
	if _, ok := sess.LatestToolResultCleanup("ide"); !ok {
		t.Fatal("cleanup should be active before compaction")
	}
	compaction, err := sess.AppendContextCompactionAt(sess.ContextCursor(), ContextCompaction{
		ID: "compaction-after-cleanup", AgentKind: "ide", Epoch: 1, Summary: "checkpoint",
		SourceStartIndex: 0, SourceEndIndex: 3, SourceMessageCount: 3, RetainedTurns: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := sess.LatestToolResultCleanup("ide"); ok {
		t.Fatal("cleanup before the latest compaction must not be projected")
	}
	if _, removed, err := sess.CommitContextCompactionRemovalAt(sess.ContextCursor(), ContextCompactionRemoval{
		ID: "remove-compaction-after-cleanup", AgentKind: "ide", CompactionID: compaction.ID,
	}); err != nil || !removed {
		t.Fatalf("remove compaction: removed=%t err=%v", removed, err)
	}
	if _, ok := sess.LatestToolResultCleanup("ide"); ok {
		t.Fatal("removing a compaction must not reactivate an older cleanup")
	}
	if err := sess.AppendClearMarker(); err != nil {
		t.Fatal(err)
	}
	snapshot, err := sess.SnapshotContext("ide")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.ToolResultCleanup != nil {
		t.Fatalf("clear retained cleanup projection: %#v", snapshot.ToolResultCleanup)
	}
	if len(sess.History()) == 0 {
		t.Fatal("clear should append a marker instead of deleting display history")
	}
}

func TestToolResultCleanupRejectsInvalidFrozenProjection(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	sess, err := store.Create("invalid cleanup")
	if err != nil {
		t.Fatal(err)
	}
	appendCleanupFixtureMessages(t, sess)
	record := cleanupFixtureRecord("cleanup-invalid", "placeholder")
	record.Replacements = append(record.Replacements, ToolResultReplacement{
		MessageIndex: record.Replacements[0].MessageIndex, ToolCallID: "different-call", Placeholder: "duplicate message target",
	})
	if _, err := sess.AppendToolResultCleanupAt(sess.ContextCursor(), record); err == nil {
		t.Fatal("duplicate cleanup replacement target should be rejected")
	}
	record = cleanupFixtureRecord("cleanup-out-of-range", "placeholder")
	record.SourceEnd = 4
	if _, err := sess.AppendToolResultCleanupAt(sess.ContextCursor(), record); err == nil {
		t.Fatal("cleanup source beyond the canonical transcript should be rejected")
	}
}

func appendCleanupFixtureMessages(t *testing.T, sess *Session) {
	t.Helper()
	call := agent.ToolCall{ID: "call-read-1", Type: "function", Function: agent.FunctionCall{Name: "read", Arguments: `{"path":"chapters/one.md"}`}}
	for _, message := range []*agent.Message{
		agent.UserMessage("Read the chapter"),
		agent.AssistantMessage("", []agent.ToolCall{call}),
		agent.ToolMessage(agent.TextToolResult("full rich chapter content"), call.ID, agent.WithToolName("read")),
	} {
		if err := sess.Append(message); err != nil {
			t.Fatal(err)
		}
	}
}

func cleanupFixtureRecord(id, placeholder string) ToolResultCleanupRecord {
	return ToolResultCleanupRecord{
		ID: id, AgentKind: "ide", SourceStart: 0, SourceEnd: 3,
		Replacements:    []ToolResultReplacement{{MessageIndex: 2, ToolCallID: "call-read-1", Placeholder: placeholder}},
		ReclaimedTokens: 20_000, TriggeredAtUsage: 280_000, WarmSuffixTokens: 4_000, RendererVersion: "receipt/v1",
	}
}
