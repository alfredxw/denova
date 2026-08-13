package session

import (
	"errors"
	"testing"

	agent "github.com/alfredxw/denova/agent"

	"denova/internal/agents/conversationjournal"
)

func TestLoaderIgnoresRetiredNonContentRecords(t *testing.T) {
	const sessionID = "retired-records"
	retiredTypes := []string{
		"ask",
		"ask_patch",
		"context_compaction",
		"context_compaction_removed",
		"context_compaction_health",
		"tool_result_cleanup",
		"goal_changed",
	}
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.GetOrCreate(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if err := sess.Append(agent.UserMessage("preserved user message")); err != nil {
		t.Fatal(err)
	}
	retiredRecords := make([]any, len(retiredTypes))
	for index, recordType := range retiredTypes {
		retiredRecords[index] = map[string]string{"type": recordType}
	}
	sess.mu.Lock()
	_, err = sess.appendJournalRecordsLocked(retiredRecords...)
	sess.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	if err := sess.Append(agent.AssistantMessage("preserved assistant message", nil)); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	reloaded, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer reloaded.Close()
	sess, err = reloaded.Get(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	messages := sess.GetMessages()
	if len(messages) != 2 || messages[0].Content != "preserved user message" || messages[1].Content != "preserved assistant message" {
		t.Fatalf("messages after retired records = %#v", messages)
	}
}

func TestSessionProjectionMarksVersionMismatchAsRebuildable(t *testing.T) {
	projection := newSessionJournalProjection("session-id", "generation-id")
	err := projection.Restore([]byte(`{"version":16}`))
	if !errors.Is(err, conversationjournal.ErrProjectionCheckpointIncompatible) {
		t.Fatalf("projection version error = %v", err)
	}
}
