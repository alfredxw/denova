package agent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	agentsession "github.com/alfredxw/denova/agent/session"
)

func TestSessionClearAtomicallyResetsConversationStateAndPreservesGoalAcrossReopen(t *testing.T) {
	model := &lifecycleModel{responses: []*Message{
		AssistantMessage("answer before clear", nil),
		AssistantMessage("answer after clear", nil),
	}}
	store := &persistentMemoryStore{Store: agentsession.Memory()}
	owner, err := New(context.Background(), Definition{
		Model: model, ModelIdentity: CapabilityIdentity{Kind: "model.clear-test", Version: 1},
		Goal: admissionGoalManager{},
	}, WithSessionStore(store))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close(context.Background()) })
	key := NamedSession("clear-atomic-reopen")
	session, err := owner.Session(context.Background(), key)
	if err != nil {
		t.Fatal(err)
	}
	before, err := session.Run(context.Background(), Input{Text: "message before clear", IdempotencyKey: "before-clear"})
	if err != nil {
		t.Fatal(err)
	}
	if result, waitErr := before.Wait(context.Background()); waitErr != nil || result.Status != ResultCompleted {
		t.Fatalf("pre-clear result=%#v error=%v", result, waitErr)
	}
	goal, err := session.UpdateGoal(context.Background(), GoalMutation{
		Kind: GoalSet, Objective: "preserve this objective", MutationID: "set-clear-goal",
	})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, time.August, 12, 12, 0, 0, 0, time.UTC)
	setClearTestCapability(t, session, TodoCapability, TodoState{
		Revision: 1, Items: []TodoItem{{ID: "old-todo", Text: "clear me", Status: TodoPending}},
	})
	setClearTestCapability(t, session, compactionCapability, CompactionState{
		ID: "old-compaction", Revision: 3, SourceRevision: "old", SourceHash: "old-hash",
		Summary: "old checkpoint", ReplacementFrom: 0, ReplacementTo: 2, CreatedAt: now,
	})
	setClearTestCapability(t, session, cleanupCapability, CleanupState{
		ID: "old-cleanup", Revision: 4, SourceRevision: "old", SourceHash: "old-hash",
		SourceStart: 0, SourceEnd: 1,
		Replacements: []CleanupReplacement{{MessageIndex: 0, ToolCallID: "old-call", Placeholder: "[cleared]"}},
		Renderer:     "test.clear", CreatedAt: now, UpdatedAt: now,
	})
	setClearTestCapability(t, session, compactionHealthCapability, compactionHealthState{
		Fingerprint: "old-failure", ConsecutiveFailures: 2, FailureCode: "failed twice",
	})

	if err := session.Clear(context.Background()); err != nil {
		t.Fatal(err)
	}
	assertClearedSessionProjection(t, session, goal)
	assertClearRawState(t, session)

	if err := session.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	session, err = owner.Session(context.Background(), key)
	if err != nil {
		t.Fatal(err)
	}
	assertClearedSessionProjection(t, session, goal)
	assertClearRawState(t, session)

	after, err := session.Run(context.Background(), Input{Text: "message after clear", IdempotencyKey: "after-clear"})
	if err != nil {
		t.Fatal(err)
	}
	if result, waitErr := after.Wait(context.Background()); waitErr != nil || result.Status != ResultCompleted {
		t.Fatalf("post-clear result=%#v error=%v", result, waitErr)
	}
	calls := model.calls()
	if len(calls) != 2 {
		t.Fatalf("model calls=%d, want 2", len(calls))
	}
	for _, message := range calls[1] {
		if message != nil && (strings.Contains(message.Content, "message before clear") ||
			strings.Contains(message.Content, "answer before clear")) {
			t.Fatalf("cleared transcript reached the next model call: %#v", calls[1])
		}
	}
}

func TestSessionClearRejectsBusySessionWithoutPartialReset(t *testing.T) {
	model := newGatedLifecycleModel()
	owner, err := New(context.Background(), Definition{Model: model})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close(context.Background()) })
	session, err := owner.Session(context.Background(), NamedSession("clear-busy"))
	if err != nil {
		t.Fatal(err)
	}
	wantTodo := TodoState{Revision: 1, Items: []TodoItem{{ID: "kept", Text: "still present", Status: TodoPending}}}
	setClearTestCapability(t, session, TodoCapability, wantTodo)
	run, err := session.Run(context.Background(), Input{Text: "block", IdempotencyKey: "busy-clear"})
	if err != nil {
		t.Fatal(err)
	}
	<-model.calls
	if err := session.Clear(context.Background()); !errors.Is(err, ErrSessionBusy) {
		t.Fatalf("Clear error=%v, want ErrSessionBusy", err)
	}
	snapshot, err := session.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.ClearRevision != 0 || snapshot.Todo == nil || snapshot.Todo.Revision != wantTodo.Revision {
		t.Fatalf("busy Clear partially reset Session: %#v", snapshot)
	}
	if _, err := run.Abort(context.Background(), AbortRequest{Reason: "finish busy-clear test"}); err != nil {
		t.Fatal(err)
	}
	if result, waitErr := run.Wait(context.Background()); waitErr != nil || result.Status != ResultAborted {
		t.Fatalf("aborted result=%#v error=%v", result, waitErr)
	}
}

func setClearTestCapability(t testing.TB, session *Session, capability string, value any) {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	current, err := session.harness.CapabilityState(context.Background(), capability)
	if err != nil {
		t.Fatal(err)
	}
	if err := session.harness.SetCapabilityState(
		context.Background(), capability, current.Descriptor, encoded, false,
	); err != nil {
		t.Fatal(err)
	}
}

func assertClearedSessionProjection(t testing.TB, session *Session, wantGoal GoalState) {
	t.Helper()
	snapshot, err := session.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.ClearRevision != 1 || snapshot.Todo != nil || snapshot.Compaction != nil || snapshot.Cleanup != nil {
		t.Fatalf("cleared projection=%#v", snapshot)
	}
	if snapshot.Goal == nil || snapshot.Goal.ID != wantGoal.ID || snapshot.Goal.Revision != wantGoal.Revision {
		t.Fatalf("Goal after Clear=%#v, want %#v", snapshot.Goal, wantGoal)
	}
}

func assertClearRawState(t testing.TB, session *Session) {
	t.Helper()
	for _, test := range []struct {
		capability string
		wantExists bool
	}{
		{capability: TodoCapability, wantExists: false},
		{capability: compactionHealthCapability, wantExists: false},
		{capability: goalCapability, wantExists: true},
		{capability: compactionCapability, wantExists: true},
		{capability: cleanupCapability, wantExists: true},
		{capability: clearCapability, wantExists: true},
	} {
		state, err := session.harness.CapabilityState(context.Background(), test.capability)
		if err != nil {
			t.Fatal(err)
		}
		if state.Exists != test.wantExists {
			t.Fatalf("capability %q exists=%t, want %t", test.capability, state.Exists, test.wantExists)
		}
	}
	checkpoint, err := session.harness.EngineCheckpoint(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	transcript, err := decodeEngineTranscript(checkpoint.State)
	if err != nil {
		t.Fatal(err)
	}
	clearState, present, err := applyClearToTranscript(&transcript, checkpoint.Capabilities)
	if err != nil || !present || clearState.Revision != 1 || len(transcript.Messages) != 0 {
		t.Fatalf("effective transcript=%#v clear=%#v present=%t error=%v", transcript.Messages, clearState, present, err)
	}
	if checkpoint.Capabilities[clearCapability] == nil {
		t.Fatal("Clear marker is not durable")
	}
}
