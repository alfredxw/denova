package conversation

import (
	"context"
	"denova/internal/agents/run"
	"reflect"
	"strings"
	"testing"

	agent "github.com/alfredxw/denova/agent"

	"denova/config"
	agentcompaction "denova/internal/agents/context/compaction"
	"denova/internal/agents/session"
)

func TestCompactionSupersedesRewindWithoutRestoringDiscardedBranch(t *testing.T) {
	directory := t.TempDir()
	store, err := session.NewStore(directory)
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.GetOrCreate("rewind-then-compact")
	if err != nil {
		t.Fatal(err)
	}
	prefix := []*agent.Message{
		agent.UserMessage(strings.Repeat("stable evidence before rewind ", 300)),
		agent.AssistantMessage("stable conclusion", nil),
	}
	for _, message := range prefix {
		if err := sess.Append(message); err != nil {
			t.Fatal(err)
		}
	}
	checkpointCursor := sess.ContextCursor()
	boundary, err := session.NewContextBoundarySnapshot(checkpointCursor, prefix, prefix, 4*1024*1024)
	if err != nil {
		t.Fatal(err)
	}
	locator, err := sess.StoreContextBoundary("rewind-before-compaction", boundary)
	if err != nil {
		t.Fatal(err)
	}
	if err := sess.Append(agent.UserMessage("DISCARDED EXPLORATION MUST NEVER RETURN")); err != nil {
		t.Fatal(err)
	}
	if err := sess.AppendWithMetadata(agent.AssistantMessage("answer after rewind", nil), session.MessageMetadata{
		ContextOperations: []session.ContextOperation{{
			Kind: session.ContextOperationRewind, AgentKind: agentrun.AgentKindIDE,
			CheckpointID: "rewind-before-compaction", MessageCount: checkpointCursor.MessageCount,
			BoundaryID: "rewind-before-compaction", BoundaryLocator: locator,
			Report: "retain the stable conclusion only",
		}},
	}); err != nil {
		t.Fatal(err)
	}
	for _, message := range []*agent.Message{
		agent.UserMessage(strings.Repeat("post rewind growth ", 300)),
		agent.AssistantMessage("post rewind answer", nil),
	} {
		if err := sess.Append(message); err != nil {
			t.Fatal(err)
		}
	}

	conversation := NewSessionConversationForAgent(sess, &config.Config{}, agentrun.AgentKindIDE)
	snapshot, err := sess.SnapshotContext(agentrun.AgentKindIDE)
	if err != nil {
		t.Fatal(err)
	}
	primary := conversation.modelHistory(snapshot)
	if containsCompactionMessageContent(primary, "DISCARDED EXPLORATION") {
		t.Fatalf("rewind primary context leaked discarded branch: %#v", primary)
	}
	source, existing, sourceStart, sourceEnd, err := conversation.compactionIncrementalSource(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	if existing != "" || containsCompactionMessageContent(source, "DISCARDED EXPLORATION") {
		t.Fatalf("rewind compaction source = existing:%q source:%#v", existing, source)
	}
	model := &compactionForkCaptureModel{response: agent.AssistantMessage(
		"## Goal\nPreserve the stable conclusion.\n\n## Current state\nPost-rewind work remains active.", nil,
	)}
	request := &agent.ModelCall{Model: model, Messages: primary}
	transient, result, err := conversation.CompactContextIfNeeded(context.Background(), agentcompaction.Input{
		Messages: primary, Force: true, KeepLatestUser: true, PrimaryRequestSnapshot: request.Snapshot(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Triggered || result.ExecutionMode != agentcompaction.ExecutionCacheSafeFork {
		t.Fatalf("rewind compaction result = %#v", result)
	}
	if containsCompactionMessageContent(transient, "DISCARDED EXPLORATION") {
		t.Fatalf("transient compaction restored discarded branch: %#v", transient)
	}

	record := sessionContextCompactionRecord("rewind-compaction-record", agentrun.AgentKindIDE, preparedSessionContextCompaction{
		Result: result, SourceStartIndex: sourceStart, SourceEndIndex: sourceEnd,
	})
	if _, err := sess.AppendContextCompactionAt(sess.ContextCursor(), record); err != nil {
		t.Fatal(err)
	}
	reloadedStore, err := session.NewStore(directory)
	if err != nil {
		t.Fatal(err)
	}
	reloaded, err := reloadedStore.GetOrCreate("rewind-then-compact")
	if err != nil {
		t.Fatal(err)
	}
	reloadedSnapshot, err := reloaded.SnapshotContext(agentrun.AgentKindIDE)
	if err != nil {
		t.Fatal(err)
	}
	durable := NewSessionConversationForAgent(reloaded, &config.Config{}, agentrun.AgentKindIDE).modelHistory(reloadedSnapshot)
	if containsCompactionMessageContent(durable, "DISCARDED EXPLORATION") {
		t.Fatalf("durable compaction restored discarded branch: %#v", durable)
	}
	if !reflect.DeepEqual(transient, durable) {
		t.Fatalf("transient and durable rewind compaction diverged:\ntransient=%#v\ndurable=%#v", transient, durable)
	}
}
