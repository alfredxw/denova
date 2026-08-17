package task

import (
	"errors"
	"log/slog"
	"sync"

	"denova/internal/agents/run"
)

// Event is one display event together with its monotonic, task-local
// cursor. Durable Agent Runtime cursors describe state-machine journal events;
// this cursor deliberately remains separate and only resumes the display SSE.
type Event struct {
	Cursor uint64
	Event  agentrun.Event
}

// DisplayCheckpoint is a bounded, display-only projection of a Task at
// Cursor. It lets stream adapters rebuild UI state after the raw event suffix
// has rotated. Status and terminal reason are bounded semantic settlement state
// and therefore remain available even when Events is evicted. These events are
// never read by Agent context assembly.
type DisplayCheckpoint struct {
	Version                 int
	TaskID                  string
	Cursor                  uint64
	Complete                bool
	Settled                 bool
	Status                  Status
	TerminalReason          string
	TerminalReasonTruncated bool
	PersistenceRequired     bool
	Events                  []agentrun.Event
}

// DisplayReplay contains either the exact retained suffix or a checkpoint
// at a newer cursor. The live subscription is admitted under the same Task
// lock, so no event can fall between replay and following.
type DisplayReplay struct {
	Checkpoint *DisplayCheckpoint
	Events     []Event
}

// SubscriptionEnd tells stream adapters whether a closed subscription
// represents normal task settlement or a lagging/detached consumer that must
// reconnect before it can claim the stream is complete.
type SubscriptionEnd string

const (
	SubscriptionActive       SubscriptionEnd = "active"
	SubscriptionTaskFinished SubscriptionEnd = "task_finished"
	SubscriptionLagged       SubscriptionEnd = "subscriber_lagged"
	SubscriptionDetached     SubscriptionEnd = "subscriber_detached"
)

// Subscription owns one live event channel and its terminal reason.
// Closing is serialized through Task.mu, while reason reads use sub.mu so an
// SSE writer can inspect the outcome after ranging the channel.
type Subscription struct {
	mu     sync.RWMutex
	events chan Event
	end    SubscriptionEnd
}

func newTaskSubscription() *Subscription {
	return &Subscription{
		events: make(chan Event, 256),
		end:    SubscriptionActive,
	}
}

// Events returns the ordered live display stream for this subscription.
func (s *Subscription) Events() <-chan Event {
	if s == nil {
		return nil
	}
	return s.events
}

// EndReason reports why Events was closed.
func (s *Subscription) EndReason() SubscriptionEnd {
	if s == nil {
		return SubscriptionDetached
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.end
}

func (s *Subscription) close(reason SubscriptionEnd) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.end != SubscriptionActive {
		return
	}
	s.end = reason
	close(s.events)
}

// Subscribe returns the retained raw display suffix and a live subscription.
// Reconnectable transports must use SubscribeDisplayAfter so overflow recovers
// through an explicit checkpoint instead of silently accepting an empty replay.
func (t *Task) Subscribe() ([]Event, *Subscription) {
	snapshot, subscription, _ := t.SubscribeAfter(0)
	return snapshot, subscription
}

// SubscribeAfter returns events strictly after the supplied display cursor,
// then follows newly admitted events. The snapshot and subscription are
// created under one lock, so no event can fall between replay and live mode.
func (t *Task) SubscribeAfter(after uint64) ([]Event, *Subscription, error) {
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
// instead of ErrCursorExpired; an ahead cursor remains a protocol error.
func (t *Task) SubscribeDisplayAfter(after uint64) (DisplayReplay, *Subscription, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	snapshot, err := t.replayAfterLocked(after)
	if err == nil {
		subscription := t.subscribeLocked(len(snapshot), false)
		return DisplayReplay{Events: snapshot}, subscription, nil
	}
	if !errors.Is(err, ErrCursorExpired) {
		return DisplayReplay{}, nil, err
	}
	checkpoint := t.displayCheckpointLocked()
	subscription := t.subscribeLocked(len(checkpoint.Events), true)
	return DisplayReplay{Checkpoint: &checkpoint}, subscription, nil
}

func (t *Task) subscribeLocked(replayCount int, checkpoint bool) *Subscription {
	subscription := newTaskSubscription()
	if t.finished {
		subscription.close(SubscriptionTaskFinished)
		slog.LogAttrs(t.ctx, slog.LevelInfo, "task_subscribe", slog.String("component", "agent-task"), slog.String("task_id", t.id), slog.String("status", string(t.status)), slog.Int("replay", replayCount), slog.Bool("checkpoint", checkpoint), slog.Bool("live", false))
		return subscription
	}

	t.subs = append(t.subs, subscription)
	slog.LogAttrs(t.ctx, slog.LevelInfo, "task_subscribe", slog.String("component", "agent-task"), slog.String("task_id", t.id), slog.String("status", string(t.status)), slog.Int("replay", replayCount), slog.Bool("checkpoint", checkpoint), slog.Int("subscribers", len(t.subs)), slog.Bool("live", true))
	return subscription
}

// Unsubscribe removes and closes one live subscriber.
func (t *Task) Unsubscribe(subscription *Subscription) {
	if subscription == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	for i, sub := range t.subs {
		if sub == subscription {
			t.subs = append(t.subs[:i], t.subs[i+1:]...)
			sub.close(SubscriptionDetached)
			slog.LogAttrs(t.ctx, slog.LevelInfo, "task_unsubscribe", slog.String("component", "agent-task"), slog.String("task_id", t.id), slog.Int("subscribers", len(t.subs)))
			return
		}
	}
}
