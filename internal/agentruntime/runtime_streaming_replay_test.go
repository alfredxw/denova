package agentruntime_test

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"

	"denova/internal/agentruntime"
)

type streamOnlyJournalStore struct {
	events      []agentruntime.Event
	loadCalls   atomic.Int32
	replayCalls atomic.Int32
}

func (s *streamOnlyJournalStore) OpenJournal(context.Context, string) (agentruntime.Journal, error) {
	return &streamOnlyJournal{store: s}, nil
}

type streamOnlyJournal struct{ store *streamOnlyJournalStore }

func (j *streamOnlyJournal) Load(context.Context) ([]agentruntime.Event, error) {
	j.store.loadCalls.Add(1)
	return nil, fmt.Errorf("Load must not be used when Replay is available")
}

func (j *streamOnlyJournal) Replay(ctx context.Context, reduce func(agentruntime.Event) error) (agentruntime.JournalReplayStats, error) {
	j.store.replayCalls.Add(1)
	stats := agentruntime.JournalReplayStats{}
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

func (*streamOnlyJournal) Append(context.Context, agentruntime.Cursor, []agentruntime.EventPayload) ([]agentruntime.Event, error) {
	return nil, fmt.Errorf("unexpected append in streaming replay test")
}

func (*streamOnlyJournal) Close() error { return nil }

func TestRuntimeOpenAndProjectPreferStreamingJournalReplay(t *testing.T) {
	store := &streamOnlyJournalStore{events: settledStreamingReplayEvents()}
	runtime, err := agentruntime.NewRuntime(
		agentruntime.NewScriptedEngine(),
		store,
		agentruntime.RuntimeConfig{RetainedEventLimit: 2, RetainedMessageLimit: 1, RetainedCommandLimit: 1},
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	binding := agentruntime.WritingBinding{Workspace: "/book", SessionID: "streaming-replay"}

	projected, err := runtime.Project(context.Background(), binding)
	if err != nil {
		t.Fatal(err)
	}
	if projected.Phase != agentruntime.PhaseIdle || projected.LastOperation == nil || projected.LastOperation.Status != agentruntime.OperationSucceeded {
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
	if status.Phase != agentruntime.PhaseIdle || status.LastOperation == nil || status.LastOperation.Status != agentruntime.OperationSucceeded {
		t.Fatalf("opened status = %#v", status)
	}
	if got := store.loadCalls.Load(); got != 0 {
		t.Fatalf("Load calls = %d, want 0", got)
	}
	if got := store.replayCalls.Load(); got != 2 {
		t.Fatalf("Replay calls = %d, want project + open", got)
	}
}

func settledStreamingReplayEvents() []agentruntime.Event {
	payloads := []agentruntime.EventPayload{
		agentruntime.CommandAcceptedEvent{
			CommandID: "stream-start", CommandKind: "start_turn",
			OperationID: "stream-operation", Fingerprint: "stream-fingerprint",
		},
		agentruntime.OperationStartedEvent{OperationID: "stream-operation", Phase: agentruntime.PhaseRunning},
		agentruntime.UserMessageCommittedEvent{Message: agentruntime.Message{
			ID: "stream-user", Role: agentruntime.RoleUser, Content: "write",
			Input: agentruntime.UserInput{Text: "write"}, Operation: "stream-operation",
		}},
		agentruntime.CycleStartedEvent{OperationID: "stream-operation", Cycle: 1, SnapshotID: "stream-snapshot"},
		agentruntime.AssistantMessageCommittedEvent{Message: agentruntime.Message{
			ID: "stream-assistant", Role: agentruntime.RoleAssistant,
			Content: "done", Operation: "stream-operation",
		}},
		agentruntime.OperationSettledEvent{OperationID: "stream-operation", Status: agentruntime.OperationSucceeded},
	}
	events := make([]agentruntime.Event, len(payloads))
	for index, payload := range payloads {
		events[index] = agentruntime.Event{
			Cursor: agentruntime.Cursor(index + 1), Durability: agentruntime.EventDurable, Payload: payload,
		}
	}
	return events
}
