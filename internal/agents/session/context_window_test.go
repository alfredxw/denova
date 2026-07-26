package session

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	agent "github.com/alfredxw/denova/agent"

	"denova/internal/conversationjournal"
)

func TestContextWindowProjectionSurvivesReloadAndClearResetsIt(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.GetOrCreate("context-window")
	if err != nil {
		t.Fatal(err)
	}
	if err := sess.Append(agent.UserMessage("question")); err != nil {
		t.Fatal(err)
	}
	boundary, err := NewContextCheckpointBoundary(
		ContextCursor{MessageCount: 1},
		[]*agent.Message{agent.UserMessage("question")},
		[]*agent.Message{agent.UserMessage("question")},
		4*1024*1024,
	)
	if err != nil {
		t.Fatal(err)
	}
	checkpoint := ContextOperation{Kind: ContextOperationCheckpoint, AgentKind: "ide", CheckpointID: "cp-1", MessageCount: 1, Boundary: boundary}
	if err := sess.AppendWithMetadata(agent.AssistantMessage("checkpointed", nil), MessageMetadata{ContextOperations: []ContextOperation{checkpoint}}); err != nil {
		t.Fatal(err)
	}
	if err := sess.Append(agent.UserMessage("exploration")); err != nil {
		t.Fatal(err)
	}
	rewind := ContextOperation{Kind: ContextOperationRewind, AgentKind: "ide", CheckpointID: "cp-1", MessageCount: 1, Boundary: boundary, Report: "finding"}
	if err := sess.AppendWithMetadata(agent.AssistantMessage("rewound", nil), MessageMetadata{ContextOperations: []ContextOperation{rewind}}); err != nil {
		t.Fatal(err)
	}
	reloadedStore, err := NewStore(store.dir)
	if err != nil {
		t.Fatal(err)
	}
	reloaded, err := reloadedStore.GetOrCreate("context-window")
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := reloaded.SnapshotContext("ide")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.ContextWindow == nil || snapshot.ContextWindow.Rewind.Report != "finding" || snapshot.ContextWindow.RewindAfterIndex != 3 {
		t.Fatalf("context window projection = %#v", snapshot.ContextWindow)
	}
	if err := reloaded.AppendClearMarker(); err != nil {
		t.Fatal(err)
	}
	cleared, err := reloaded.SnapshotContext("ide")
	if err != nil {
		t.Fatal(err)
	}
	if got := cleared.ContextWindow; got != nil {
		t.Fatalf("clear should reset context rewind, got %#v", got)
	}
}

func TestActiveContextCheckpointSurvivesLiveMaterializationTrim(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.GetOrCreate("context-window-live-trim")
	if err != nil {
		t.Fatal(err)
	}
	if err := sess.Append(agent.UserMessage("stable prefix")); err != nil {
		t.Fatal(err)
	}
	cursor := sess.ContextCursor()
	boundary, err := NewContextCheckpointBoundary(
		cursor,
		[]*agent.Message{agent.UserMessage("stable prefix")},
		[]*agent.Message{agent.UserMessage("stable prefix")},
		4*1024*1024,
	)
	if err != nil {
		t.Fatal(err)
	}
	checkpoint := ContextOperation{
		Kind: ContextOperationCheckpoint, AgentKind: "ide", CheckpointID: "cp-retained",
		Purpose: "long exploration", MessageCount: cursor.MessageCount, Boundary: boundary,
	}
	if err := sess.AppendWithMetadata(agent.AssistantMessage("checkpoint", nil), MessageMetadata{
		ContextOperations: []ContextOperation{checkpoint},
	}); err != nil {
		t.Fatal(err)
	}

	// One transaction can carry many display-only records. Pairing that with a
	// >200-message tail exercises both bounded resident caches without turning
	// this durability regression into hundreds of extra fsyncs.
	appendContextWindowDisplayBatch(t, sess, sessionRecentTransactionLimit+50)
	appendContextWindowFiller(t, sess, sessionRecentTransactionLimit+25, "exploration")
	if len(sess.messages) > sessionRecentTransactionLimit {
		t.Fatalf("resident messages = %d, want <= %d", len(sess.messages), sessionRecentTransactionLimit)
	}
	for _, record := range sess.records {
		for _, operation := range record.messageMetadata.ContextOperations {
			if operation.CheckpointID == checkpoint.CheckpointID {
				t.Fatal("checkpoint-bearing record unexpectedly remained resident")
			}
		}
	}
	active, err := sess.ActiveContextCheckpoints("ide")
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 1 || active[0].CheckpointID != checkpoint.CheckpointID || active[0].Boundary.EffectiveSHA256 != boundary.EffectiveSHA256 {
		t.Fatalf("active checkpoint after live trim = %#v", active)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestLatestContextRewindSurvivesBoundedColdRestart(t *testing.T) {
	directory := t.TempDir()
	store, err := NewStore(directory)
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.GetOrCreate("context-window-cold-restart")
	if err != nil {
		t.Fatal(err)
	}
	if err := sess.Append(agent.UserMessage("stable prefix")); err != nil {
		t.Fatal(err)
	}
	cursor := sess.ContextCursor()
	boundary, err := NewContextCheckpointBoundary(
		cursor,
		[]*agent.Message{agent.UserMessage("stable prefix")},
		[]*agent.Message{agent.UserMessage("stable prefix")},
		4*1024*1024,
	)
	if err != nil {
		t.Fatal(err)
	}
	checkpoint := ContextOperation{
		Kind: ContextOperationCheckpoint, AgentKind: "ide", CheckpointID: "cp-cold-restart",
		Purpose: "long exploration", MessageCount: cursor.MessageCount, Boundary: boundary,
	}
	if err := sess.AppendWithMetadata(agent.AssistantMessage("checkpoint", nil), MessageMetadata{
		ContextOperations: []ContextOperation{checkpoint},
	}); err != nil {
		t.Fatal(err)
	}
	if err := sess.Append(agent.UserMessage("discarded exploration")); err != nil {
		t.Fatal(err)
	}

	rewindIndex := sess.MessageCountTotal()
	rewind := ContextOperation{
		Kind: ContextOperationRewind, AgentKind: "ide", CheckpointID: checkpoint.CheckpointID,
		Purpose: checkpoint.Purpose, MessageCount: cursor.MessageCount, Boundary: boundary,
		Report: "retained finding",
	}
	if err := sess.AppendWithMetadata(agent.AssistantMessage("rewound answer", nil), MessageMetadata{
		ContextOperations: []ContextOperation{rewind},
	}); err != nil {
		t.Fatal(err)
	}
	appendContextWindowFiller(t, sess, sessionRecentTransactionLimit+25, "after-rewind")
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	reloadedStore, err := NewStore(directory)
	if err != nil {
		t.Fatal(err)
	}
	reloaded, err := reloadedStore.GetOrCreate("context-window-cold-restart")
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.lastReplayRecords != 0 {
		t.Fatalf("cold restart rebuilt canonical history instead of restoring the sidecar: replayed=%d", reloaded.lastReplayRecords)
	}
	resident, base, total := reloaded.MessageWindow()
	if len(resident) > sessionRecentTransactionLimit || base <= rewindIndex || total <= base {
		t.Fatalf("cold resident window = len:%d base:%d total:%d rewind:%d", len(resident), base, total, rewindIndex)
	}
	snapshot, err := reloaded.SnapshotContext("ide")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.ContextWindow == nil || snapshot.ContextWindow.RewindAfterIndex != rewindIndex ||
		snapshot.ContextWindow.Rewind.Report != rewind.Report ||
		snapshot.ContextWindow.Checkpoint.Boundary.CanonicalSHA256 != boundary.CanonicalSHA256 {
		t.Fatalf("cold context window projection = %#v", snapshot.ContextWindow)
	}
	active, err := reloaded.ActiveContextCheckpoints("ide")
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 0 {
		t.Fatalf("rewound checkpoint became active after restart: %#v", active)
	}
}

func TestContextWindowProjectionRestoreRejectsCorruptLocatorAndBoundary(t *testing.T) {
	boundary, err := NewContextCheckpointBoundary(
		ContextCursor{Revision: 1, MessageCount: 1},
		[]*agent.Message{agent.UserMessage("prefix")},
		[]*agent.Message{agent.UserMessage("prefix")},
		4*1024*1024,
	)
	if err != nil {
		t.Fatal(err)
	}
	projection := newSessionJournalProjection("projection-corruption", "generation-1")
	projection.MessageCount = 2
	projection.ContextRevision = 2
	projection.RecentCursors = []conversationjournal.Cursor{1, 2}
	projection.lastCursor = 2
	projection.ContextWindows = []agentContextWindowProjection{{
		AgentKind: "ide",
		ActiveCheckpoints: []contextOperationLocator{{
			Cursor: 2, MessageIndex: 1, ContextRevision: 2,
			Operation: ContextOperation{
				Kind: ContextOperationCheckpoint, AgentKind: "ide", CheckpointID: "cp-corrupt",
				MessageCount: 1, Boundary: boundary,
			},
		}},
	}}
	encoded, err := projection.Checkpoint()
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name   string
		mutate func(*sessionJournalProjection)
		want   string
	}{
		{
			name: "locator outside canonical head",
			mutate: func(stored *sessionJournalProjection) {
				stored.ContextWindows[0].ActiveCheckpoints[0].Cursor = 3
			},
			want: "locator is invalid",
		},
		{
			name: "boundary integrity mismatch",
			mutate: func(stored *sessionJournalProjection) {
				stored.ContextWindows[0].ActiveCheckpoints[0].Operation.Boundary.EffectiveSHA256 = "corrupt"
			},
			want: "invalid durable boundary",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var stored sessionJournalProjection
			if err := json.Unmarshal(encoded, &stored); err != nil {
				t.Fatal(err)
			}
			test.mutate(&stored)
			corrupt, err := json.Marshal(stored)
			if err != nil {
				t.Fatal(err)
			}
			restored := newSessionJournalProjection("projection-corruption", "generation-1")
			if err := restored.Restore(corrupt); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("restore error = %v, want %q", err, test.want)
			}
		})
	}
}

func appendContextWindowFiller(t *testing.T, sess *Session, count int, prefix string) {
	t.Helper()
	for index := 0; index < count; index++ {
		if err := sess.Append(agent.UserMessage(fmt.Sprintf("%s-%d", prefix, index))); err != nil {
			t.Fatalf("append filler %d: %v", index, err)
		}
	}
}

func appendContextWindowDisplayBatch(t *testing.T, sess *Session, count int) {
	t.Helper()
	payloads := make([]json.RawMessage, count)
	for index := range payloads {
		encoded, err := json.Marshal(displayRecord{
			Type:     historyTypeDisplay,
			RecordID: fmt.Sprintf("context-window-display-%d", index),
			DisplayEvent: DisplayEvent{
				Role: "thinking", Content: fmt.Sprintf("display-%d", index),
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		payloads[index] = encoded
	}
	head := sess.journal.Head()
	if _, err := sess.journal.Append(
		context.Background(),
		conversationjournal.Guard{Cursor: sess.materializedCursor, RecordSHA256: head.RecordSHA256},
		payloads...,
	); err != nil {
		t.Fatal(err)
	}
	if err := sess.RefreshCanonical(context.Background()); err != nil {
		t.Fatal(err)
	}
}
