package runtime_test

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"

	runstate "github.com/alfredxw/denova/agent/internal/runtime"
)

type streamOnlyJournalStore struct {
	events      []runstate.Event
	loadCalls   atomic.Int32
	replayCalls atomic.Int32
}

func (s *streamOnlyJournalStore) OpenJournal(context.Context, string) (runstate.Journal, error) {
	return &streamOnlyJournal{store: s}, nil
}

type streamOnlyJournal struct{ store *streamOnlyJournalStore }

func (j *streamOnlyJournal) Load(context.Context) ([]runstate.Event, error) {
	j.store.loadCalls.Add(1)
	return nil, fmt.Errorf("Load must not be used when Replay is available")
}

func (j *streamOnlyJournal) Replay(ctx context.Context, reduce func(runstate.Event) error) (runstate.JournalReplayStats, error) {
	j.store.replayCalls.Add(1)
	stats := runstate.JournalReplayStats{}
	for _, event := range j.store.events {
		if err := ctx.Err(); err != nil {
			return stats, err
		}
		stats.EventsRead++
		if err := reduce(event); err != nil {
			return stats, err
		}
	}
	return stats, nil
}

func (*streamOnlyJournal) Append(context.Context, runstate.Cursor, []runstate.EventPayload) ([]runstate.Event, error) {
	return nil, fmt.Errorf("unexpected append in streaming replay test")
}

func (*streamOnlyJournal) Close() error { return nil }

func TestRuntimeOpenAndProjectPreferStreamingJournalReplay(t *testing.T) {
	store := &streamOnlyJournalStore{events: settledStreamingReplayEvents()}
	runtime, err := runstate.NewRuntime(
		runstate.NewScriptedEngine(),
		store,
		runstate.RuntimeConfig{RetainedEventLimit: 2, RetainedMessageLimit: 1, RetainedCommandLimit: 1},
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	binding := testBindingAt("/book", "streaming-replay")

	projected, err := runtime.Project(context.Background(), binding)
	if err != nil {
		t.Fatal(err)
	}
	if projected.Phase != runstate.PhaseIdle || projected.LastOperation == nil || projected.LastOperation.Status != runstate.OperationSucceeded {
		t.Fatalf("projected status = %#v", projected)
	}
	harness, err := runtime.Open(context.Background(), binding)
	if err != nil {
		t.Fatal(err)
	}
	status, err := harness.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if status.Phase != runstate.PhaseIdle || status.LastOperation == nil || status.LastOperation.Status != runstate.OperationSucceeded {
		t.Fatalf("opened status = %#v", status)
	}
	if got := store.loadCalls.Load(); got != 0 {
		t.Fatalf("Load calls = %d, want 0", got)
	}
	if got := store.replayCalls.Load(); got != 2 {
		t.Fatalf("Replay calls = %d, want project + open", got)
	}
}

func settledStreamingReplayEvents() []runstate.Event {
	payloads := []runstate.EventPayload{
		runstate.CommandAcceptedEvent{
			CommandID: "stream-start", CommandKind: "start_turn",
			OperationID: "stream-operation", Fingerprint: "stream-fingerprint",
		},
		runstate.OperationStartedEvent{OperationID: "stream-operation", Phase: runstate.PhaseRunning},
		runstate.UserMessageCommittedEvent{Message: runstate.Message{
			ID: "stream-user", Role: runstate.RoleUser, Content: "write",
			Input: runstate.UserInput{Text: "write"}, Operation: "stream-operation",
		}},
		runstate.CycleStartedEvent{OperationID: "stream-operation", Cycle: 1, SnapshotID: "stream-snapshot"},
		runstate.AssistantMessageCommittedEvent{Message: runstate.Message{
			ID: "stream-assistant", Role: runstate.RoleAssistant,
			Content: "done", Operation: "stream-operation",
		}},
		runstate.OperationSettledEvent{OperationID: "stream-operation", Status: runstate.OperationSucceeded},
	}
	events := make([]runstate.Event, len(payloads))
	for index, payload := range payloads {
		events[index] = runstate.Event{
			Cursor: runstate.Cursor(index + 1), Durability: runstate.EventDurable, Payload: payload,
		}
	}
	return events
}
