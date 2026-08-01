package session

import (
	"errors"
	"testing"

	agent "github.com/alfredxw/denova/agent"

	agentcontext "denova/internal/agents/context"
)

func TestContextCompactionStructuralCommitReconcilesExactIdentityBeforeCAS(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.Create("compaction structural commit")
	if err != nil {
		t.Fatal(err)
	}
	if err := sess.Append(agent.UserMessage("long history")); err != nil {
		t.Fatal(err)
	}
	cursor := sess.ContextCursor()
	intent := ContextCompaction{
		ID: "cc-command-1",
		CompactionCheckpoint: agentcontext.CompactionCheckpoint{
			AgentKind: "ide", Epoch: 1, Summary: "checkpoint", RetainedTurns: 2, TriggerReason: "manual", Phase: "manual",
		},
		SourceStartIndex: 0, SourceEndIndex: 1, SourceMessageCount: 1,
	}
	first, err := sess.AppendContextCompactionAt(cursor, intent)
	if err != nil {
		t.Fatal(err)
	}
	second, err := sess.AppendContextCompactionAt(cursor, intent)
	if err != nil {
		t.Fatalf("exact retry after cursor advance failed: %v", err)
	}
	if second.ID != first.ID || second.ContextRevision != first.ContextRevision {
		t.Fatalf("retry created a different compaction: first=%#v second=%#v", first, second)
	}
	conflictingReplay := intent
	conflictingReplay.CandidateGeneration = 2
	if _, err := sess.AppendContextCompactionAt(cursor, conflictingReplay); !errors.Is(err, ErrDomainCommitIdentityConflict) {
		t.Fatalf("same checkpoint id with changed durable fields error = %v, want %v", err, ErrDomainCommitIdentityConflict)
	}
	removalCursor := sess.ContextCursor()
	removalIntent := ContextCompactionRemoval{ID: "ccr-command-1", AgentKind: "ide", CompactionID: first.ID, Reason: "user_removed"}
	removed, ok, err := sess.CommitContextCompactionRemovalAt(removalCursor, removalIntent)
	if err != nil || !ok {
		t.Fatalf("remove compaction: ok=%t err=%v", ok, err)
	}
	replayed, ok, err := sess.CommitContextCompactionRemovalAt(removalCursor, removalIntent)
	if err != nil || !ok {
		t.Fatalf("replay removal after cursor advance: ok=%t err=%v", ok, err)
	}
	if replayed.ID != removed.ID || replayed.ContextRevision != removed.ContextRevision {
		t.Fatalf("retry created a different removal: first=%#v second=%#v", removed, replayed)
	}
}

func TestContextCompactionStructuralCommitRefreshesAcrossSessionInstances(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	primary, err := store.Create("cross-instance compaction")
	if err != nil {
		t.Fatal(err)
	}
	if err := primary.Append(agent.UserMessage("long history")); err != nil {
		t.Fatal(err)
	}
	staleStore, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	stale, err := staleStore.Get(primary.ID)
	if err != nil {
		t.Fatal(err)
	}
	cursor := stale.ContextCursor()
	intent := ContextCompaction{
		ID: "cc-cross-instance",
		CompactionCheckpoint: agentcontext.CompactionCheckpoint{
			AgentKind: "ide", Epoch: 1, Summary: "checkpoint", RetainedTurns: 2, TriggerReason: "manual", Phase: "manual",
		},
		SourceStartIndex: 0, SourceEndIndex: 1, SourceMessageCount: 1,
	}
	committed, err := primary.AppendContextCompactionAt(primary.ContextCursor(), intent)
	if err != nil {
		t.Fatal(err)
	}
	if replayed, err := stale.AppendContextCompactionAt(cursor, intent); err != nil || replayed.ContextRevision != committed.ContextRevision {
		t.Fatalf("exact retry through stale instance: record=%#v err=%v", replayed, err)
	}
	conflict := intent
	conflict.ID = "cc-cross-instance-conflict"
	if _, err := stale.AppendContextCompactionAt(cursor, conflict); !errors.Is(err, ErrContextRevisionConflict) {
		t.Fatalf("stale distinct checkpoint error = %v, want %v", err, ErrContextRevisionConflict)
	}

	removalObserverStore, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	removalObserver, err := removalObserverStore.Get(primary.ID)
	if err != nil {
		t.Fatal(err)
	}
	removalCursor := removalObserver.ContextCursor()
	removal := ContextCompactionRemoval{
		ID: "ccr-cross-instance", AgentKind: "ide", CompactionID: committed.ID, Reason: "user_removed",
	}
	removed, ok, err := primary.CommitContextCompactionRemovalAt(primary.ContextCursor(), removal)
	if err != nil || !ok {
		t.Fatalf("primary removal: record=%#v ok=%t err=%v", removed, ok, err)
	}
	if replayed, ok, err := removalObserver.CommitContextCompactionRemovalAt(removalCursor, removal); err != nil || !ok || replayed.ContextRevision != removed.ContextRevision {
		t.Fatalf("exact removal retry through stale instance: record=%#v ok=%t err=%v", replayed, ok, err)
	}
}
