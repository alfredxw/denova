package agents

import (
	"context"
	"reflect"
	"strings"
	"testing"

	agent "github.com/alfredxw/denova/agent"

	"denova/config"
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
			Kind: session.ContextOperationRewind, AgentKind: AgentKindIDE,
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

	conversation := NewSessionConversationForAgent(sess, &config.Config{}, AgentKindIDE)
	snapshot, err := sess.SnapshotContext(AgentKindIDE)
	if err != nil {
		t.Fatal(err)
	}
	primary := conversation.modelHistory(snapshot)
	if containsMessageContent(primary, "DISCARDED EXPLORATION") {
		t.Fatalf("rewind primary context leaked discarded branch: %#v", primary)
	}
	source, existing, sourceStart, sourceEnd, err := conversation.compactionIncrementalSource(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	if existing != "" || containsMessageContent(source, "DISCARDED EXPLORATION") {
		t.Fatalf("rewind compaction source = existing:%q source:%#v", existing, source)
	}
	if positions, _, visible := locateCompactionSourceInPrimary(primary, source); !visible || len(positions) != len(source) {
		t.Fatalf("rewind-effective source is not an exact primary interval: positions=%v visible=%t", positions, visible)
	}

	model := &compactionForkCaptureModel{response: agent.AssistantMessage(
		"## Goal\nPreserve the stable conclusion.\n\n## Current state\nPost-rewind work remains active.", nil,
	)}
	request := &agent.ModelCall{Model: model, Messages: primary}
	ctx := contextWithCompactionRequestSnapshot(context.Background(), request.Snapshot())
	transient, result, err := conversation.CompactContextIfNeeded(ctx, ContextCompactionInput{
		Messages: primary, Force: true, KeepLatestUser: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Triggered || result.ExecutionMode != contextCompactionExecutionCacheSafeFork {
		t.Fatalf("rewind compaction result = %#v", result)
	}
	if containsMessageContent(transient, "DISCARDED EXPLORATION") {
		t.Fatalf("transient compaction restored discarded branch: %#v", transient)
	}

	record := contextCompactionRecordFromResult(
		result, AgentKindIDE, sourceStart, sourceEnd, result.RetainedTurns, result.Summary,
	)
	record.ID = "rewind-compaction-record"
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
	reloadedSnapshot, err := reloaded.SnapshotContext(AgentKindIDE)
	if err != nil {
		t.Fatal(err)
	}
	durable := NewSessionConversationForAgent(reloaded, &config.Config{}, AgentKindIDE).modelHistory(reloadedSnapshot)
	if containsMessageContent(durable, "DISCARDED EXPLORATION") {
		t.Fatalf("durable compaction restored discarded branch: %#v", durable)
	}
	if !reflect.DeepEqual(transient, durable) {
		t.Fatalf("transient and durable rewind compaction diverged:\ntransient=%#v\ndurable=%#v", transient, durable)
	}
}
