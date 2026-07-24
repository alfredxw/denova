package app

import (
	"errors"
	"log/slog"
	"sync"

	agents "denova/internal/agents"
	"denova/internal/observability"
)

// TaskEvent is one display event together with its monotonic, task-local
// cursor. Durable Agent Runtime cursors describe state-machine journal events;
// this cursor deliberately remains separate and only resumes the display SSE.
type TaskEvent struct {
	Cursor uint64
	Event  agents.Event
}

// TaskDisplayCheckpoint is a bounded, display-only projection of a Task at
// Cursor. It lets stream adapters rebuild UI state after the raw event suffix
// has rotated. Status and terminal reason are bounded semantic settlement state
// and therefore remain available even when Events is evicted. These events are
// never read by Agent context assembly.
type TaskDisplayCheckpoint struct {
	Version                 int
	TaskID                  string
	Cursor                  uint64
	Complete                bool
	Settled                 bool
	Status                  TaskStatus
	TerminalReason          string
	TerminalReasonTruncated bool
	PersistenceRequired     bool
	Events                  []agents.Event
}

// TaskDisplayReplay contains either the exact retained suffix or a checkpoint
// at a newer cursor. The live subscription is admitted under the same Task
// lock, so no event can fall between replay and following.
type TaskDisplayReplay struct {
	Checkpoint *TaskDisplayCheckpoint
	Events     []TaskEvent
}

// TaskSubscriptionEnd tells stream adapters whether a closed subscription
// represents normal task settlement or a lagging/detached consumer that must
// reconnect before it can claim the stream is complete.
type TaskSubscriptionEnd string

const (
	TaskSubscriptionActive       TaskSubscriptionEnd = "active"
	TaskSubscriptionTaskFinished TaskSubscriptionEnd = "task_finished"
	TaskSubscriptionLagged       TaskSubscriptionEnd = "subscriber_lagged"
	TaskSubscriptionDetached     TaskSubscriptionEnd = "subscriber_detached"
)

// TaskSubscription owns one live event channel and its terminal reason.
// Closing is serialized through Task.mu, while reason reads use sub.mu so an
// SSE writer can inspect the outcome after ranging the channel.
type TaskSubscription struct {
	mu     sync.RWMutex
	events chan TaskEvent
	end    TaskSubscriptionEnd
}

func newTaskSubscription() *TaskSubscription {
	return &TaskSubscription{
		events: make(chan TaskEvent, 256),
		end:    TaskSubscriptionActive,
	}
}

// Events returns the ordered live display stream for this subscription.
func (s *TaskSubscription) Events() <-chan TaskEvent {
	if s == nil {
		return nil
	}
	return s.events
}

// EndReason reports why Events was closed.
func (s *TaskSubscription) EndReason() TaskSubscriptionEnd {
	if s == nil {
		return TaskSubscriptionDetached
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.end
}

func (s *TaskSubscription) close(reason TaskSubscriptionEnd) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.end != TaskSubscriptionActive {
		return
	}
	s.end = reason
	close(s.events)
}

// Subscribe returns the retained raw display suffix and a live subscription.
// Reconnectable transports must use SubscribeDisplayAfter so overflow recovers
// through an explicit checkpoint instead of silently accepting an empty replay.
func (t *Task) Subscribe() ([]TaskEvent, *TaskSubscription) {
	snapshot, subscription, _ := t.SubscribeAfter(0)
	return snapshot, subscription
}

// SubscribeAfter returns events strictly after the supplied display cursor,
// then follows newly admitted events. The snapshot and subscription are
// created under one lock, so no event can fall between replay and live mode.
func (t *Task) SubscribeAfter(after uint64) ([]TaskEvent, *TaskSubscription, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	snapshot, err := t.replayAfterLocked(after)
	if err != nil {
		return nil, nil, err
	}
	subscription := t.subscribeLocked(len(snapshot), false)
	return snapshot, subscription, nil
}

// SubscribeDisplayAfter resumes a display stream without requiring an
// unbounded raw event log. An expired cursor receives the current checkpoint
// instead of ErrTaskCursorExpired; an ahead cursor remains a protocol error.
func (t *Task) SubscribeDisplayAfter(after uint64) (TaskDisplayReplay, *TaskSubscription, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	snapshot, err := t.replayAfterLocked(after)
	if err == nil {
		subscription := t.subscribeLocked(len(snapshot), false)
		return TaskDisplayReplay{Events: snapshot}, subscription, nil
	}
	if !errors.Is(err, ErrTaskCursorExpired) {
		return TaskDisplayReplay{}, nil, err
	}
	checkpoint := t.displayCheckpointLocked()
	subscription := t.subscribeLocked(len(checkpoint.Events), true)
	return TaskDisplayReplay{Checkpoint: &checkpoint}, subscription, nil
}

func (t *Task) subscribeLocked(replayCount int, checkpoint bool) *TaskSubscription {
	subscription := newTaskSubscription()
	if t.finished {
		subscription.close(TaskSubscriptionTaskFinished)
		observability.Info("agent-task", "task_subscribe", slog.String("task_id", t.id), slog.String("status", string(t.status)), slog.Int("replay", replayCount), slog.Bool("checkpoint", checkpoint), slog.Bool("live", false))
		return subscription
	}

	t.subs = append(t.subs, subscription)
	observability.Info("agent-task", "task_subscribe", slog.String("task_id", t.id), slog.String("status", string(t.status)), slog.Int("replay", replayCount), slog.Bool("checkpoint", checkpoint), slog.Int("subscribers", len(t.subs)), slog.Bool("live", true))
	return subscription
}

// Unsubscribe removes and closes one live subscriber.
func (t *Task) Unsubscribe(subscription *TaskSubscription) {
	if subscription == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	for i, sub := range t.subs {
		if sub == subscription {
			t.subs = append(t.subs[:i], t.subs[i+1:]...)
			sub.close(TaskSubscriptionDetached)
			observability.Info("agent-task", "task_unsubscribe", slog.String("task_id", t.id), slog.Int("subscribers", len(t.subs)))
			return
		}
	}
}
