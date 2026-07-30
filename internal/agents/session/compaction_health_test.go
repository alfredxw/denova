package session

import (
	"context"
	"testing"

	agent "github.com/alfredxw/denova/agent"
)

func TestContextCompactionHealthPersistsAcrossTranscriptRevisionsAndReload(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.Create("compaction health")
	if err != nil {
		t.Fatal(err)
	}
	if err := sess.Append(agent.UserMessage("first")); err != nil {
		t.Fatal(err)
	}
	firstCursor := sess.ContextCursor()
	first, err := sess.CommitContextCompactionHealthAtContext(context.Background(), firstCursor, ContextCompactionHealth{
		ID: "health-1", AgentKind: "ide", StructureFingerprint: "stable", Outcome: contextCompactionHealthFailure, FailureCode: "summary_failed",
	})
	if err != nil {
		t.Fatal(err)
	}
	if first.ConsecutiveFailures != 1 || sess.ContextCursor().Revision != firstCursor.Revision {
		t.Fatalf("first health=%#v cursor=%#v", first, sess.ContextCursor())
	}

	if err := sess.Append(agent.AssistantMessage("ordinary tail growth", nil)); err != nil {
		t.Fatal(err)
	}
	secondCursor := sess.ContextCursor()
	if latest, ok := sess.LatestContextCompactionHealth("ide"); !ok || latest.ID != first.ID {
		t.Fatalf("ordinary transcript growth discarded health: %#v ok=%t", latest, ok)
	}
	secondIntent := ContextCompactionHealth{
		ID: "health-2", AgentKind: "ide", StructureFingerprint: "stable", Outcome: contextCompactionHealthFailure, FailureCode: "summary_failed",
	}
	second, err := sess.CommitContextCompactionHealthAtContext(context.Background(), secondCursor, secondIntent)
	if err != nil {
		t.Fatal(err)
	}
	if second.ConsecutiveFailures != 2 || sess.ContextCursor().Revision != secondCursor.Revision {
		t.Fatalf("second health=%#v cursor=%#v", second, sess.ContextCursor())
	}
	replayed, err := sess.CommitContextCompactionHealthAtContext(context.Background(), secondCursor, secondIntent)
	if err != nil || replayed.ConsecutiveFailures != 2 {
		t.Fatalf("exact retry=%#v err=%v", replayed, err)
	}

	changed, err := sess.CommitContextCompactionHealthAtContext(context.Background(), secondCursor, ContextCompactionHealth{
		ID: "health-3", AgentKind: "ide", StructureFingerprint: "changed", Outcome: contextCompactionHealthFailure, FailureCode: "summary_failed",
	})
	if err != nil || changed.ConsecutiveFailures != 1 {
		t.Fatalf("changed structure=%#v err=%v", changed, err)
	}
	reset, err := sess.CommitContextCompactionHealthAtContext(context.Background(), secondCursor, ContextCompactionHealth{
		ID: "health-reset", AgentKind: "ide", StructureFingerprint: "manual", Outcome: contextCompactionHealthManualRetry,
	})
	if err != nil || reset.ConsecutiveFailures != 0 {
		t.Fatalf("manual reset=%#v err=%v", reset, err)
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
	latest, ok := reloaded.LatestContextCompactionHealth("ide")
	if !ok || latest.ID != reset.ID || latest.ConsecutiveFailures != 0 || reloaded.ContextCursor().Revision != secondCursor.Revision {
		t.Fatalf("reloaded health=%#v ok=%t cursor=%#v", latest, ok, reloaded.ContextCursor())
	}
}

func TestContextCompactionHealthIsReleasedByNewerRewind(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.GetOrCreate("compaction-health-rewind")
	if err != nil {
		t.Fatal(err)
	}
	if err := sess.Append(agent.UserMessage("stable prefix")); err != nil {
		t.Fatal(err)
	}
	if _, err := sess.CommitContextCompactionHealthAtContext(context.Background(), sess.ContextCursor(), ContextCompactionHealth{
		ID: "health-before-rewind", AgentKind: "ide", StructureFingerprint: "old-branch",
		Outcome: contextCompactionHealthFailure, FailureCode: "summary_failed",
	}); err != nil {
		t.Fatal(err)
	}
	cursor := sess.ContextCursor()
	prefix := []*agent.Message{agent.UserMessage("stable prefix")}
	_, locator := storeTestContextBoundary(t, sess, "health-rewind-checkpoint", cursor, prefix, prefix)
	checkpoint := ContextOperation{
		Kind: ContextOperationCheckpoint, AgentKind: "ide", CheckpointID: "health-rewind-checkpoint",
		MessageCount: cursor.MessageCount, BoundaryID: "health-rewind-checkpoint", BoundaryLocator: locator,
	}
	if err := sess.AppendWithMetadata(agent.AssistantMessage("checkpoint", nil), MessageMetadata{ContextOperations: []ContextOperation{checkpoint}}); err != nil {
		t.Fatal(err)
	}
	if err := sess.Append(agent.UserMessage("discarded exploration")); err != nil {
		t.Fatal(err)
	}
	rewind := ContextOperation{
		Kind: ContextOperationRewind, AgentKind: "ide", CheckpointID: checkpoint.CheckpointID,
		MessageCount: checkpoint.MessageCount, BoundaryID: checkpoint.BoundaryID, BoundaryLocator: checkpoint.BoundaryLocator,
		Report: "selected a safe branch",
	}
	if err := sess.AppendWithMetadata(agent.AssistantMessage("rewound", nil), MessageMetadata{ContextOperations: []ContextOperation{rewind}}); err != nil {
		t.Fatal(err)
	}
	if health, ok := sess.LatestContextCompactionHealth("ide"); ok {
		t.Fatalf("rewind retained stale compaction health: %#v", health)
	}
	id := sess.ID
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reloadedStore, err := NewStore(store.dir)
	if err != nil {
		t.Fatal(err)
	}
	defer reloadedStore.Close()
	reloaded, err := reloadedStore.Get(id)
	if err != nil {
		t.Fatal(err)
	}
	if health, ok := reloaded.LatestContextCompactionHealth("ide"); ok {
		t.Fatalf("reload restored stale pre-rewind health: %#v", health)
	}
}
