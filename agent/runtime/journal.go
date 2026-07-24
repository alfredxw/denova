package runtime

import (
	"context"
	"fmt"
	"sync"
)

// Journal is the durable append seam. Implementations must atomically append
// the whole payload batch at expected and return contiguous committed events.
// A context cancellation error is a definite no-commit result. When work may
// have committed, implementations must either reconcile and return the events
// or return a non-context error so the Harness releases its lease and replays.
type Journal interface {
	Load(context.Context) ([]Event, error)
	Append(context.Context, Cursor, []EventPayload) ([]Event, error)
	// Close releases the exclusive binding lease acquired by OpenJournal. The
	// lease is held for the whole Harness lifetime so another runtime cannot
	// mistake a live operation for crash residue and recover it concurrently.
	Close() error
}

// JournalReplayStats makes cold-start I/O observable without retaining the
// replayed timeline. BytesRead includes framing bytes read from the canonical
// journal; RecordsRead counts committed transactions and EventsRead counts
// the durable events delivered to the reducer.
type JournalReplayStats struct {
	BytesRead          int64
	SnapshotBytesRead  int64
	TailBytesRead      int64
	RecordsRead        int64
	EventsRead         int64
	SnapshotGeneration uint64
}

// StreamingJournal is the bounded event-history cold replay seam. Runtime
// owners can reduce events directly into their retained projection instead of
// first materializing the complete journal as []Event. Implementations may
// still retain a compact command-receipt index to preserve historical
// idempotency. Reducers are called in cursor order while the journal's
// exclusive binding lease is held.
//
// Load remains part of Journal for compatibility with existing stores and
// diagnostics. Callers that build retained state should prefer Replay when it
// is available and fall back to Load otherwise.
type StreamingJournal interface {
	Replay(context.Context, func(Event) error) (JournalReplayStats, error)
}

type harnessStateReplayJournal interface {
	ReplayHarnessState(context.Context, *harnessState) (JournalReplayStats, error)
}

func replayHarnessJournalState(ctx context.Context, journal Journal, state *harnessState) (JournalReplayStats, error) {
	if stateful, ok := journal.(harnessStateReplayJournal); ok {
		return stateful.ReplayHarnessState(ctx, state)
	}
	return replayJournalState(ctx, journal, func(event Event) error {
		return state.reduce(event)
	})
}

// harnessStateCheckpointJournal is implemented only by journals that can
// atomically switch a reducer checkpoint plus bounded tail generation. The
// actor calls it after a complete durable transaction has been reduced.
type harnessStateCheckpointJournal interface {
	MaybeCheckpoint(context.Context, *harnessState) error
}

// replayJournalState prefers bounded-memory streaming replay while preserving
// compatibility with Journal implementations that only expose Load. The
// reducer owns retention; this helper never accumulates a second event slice
// when Replay is available.
func replayJournalState(ctx context.Context, journal Journal, reduce func(Event) error) (JournalReplayStats, error) {
	if journal == nil || reduce == nil {
		return JournalReplayStats{}, fmt.Errorf("journal and replay reducer are required")
	}
	if streaming, ok := journal.(StreamingJournal); ok {
		return streaming.Replay(ctx, reduce)
	}
	events, err := journal.Load(ctx)
	if err != nil {
		return JournalReplayStats{}, err
	}
	stats := JournalReplayStats{EventsRead: int64(len(events))}
	for _, event := range events {
		if err := reduce(event); err != nil {
			return stats, err
		}
	}
	return stats, nil
}

type CommandRecord struct {
	Receipt     Receipt
	Fingerprint string
}

// CommandJournalLookup is the cold idempotency index used after the actor's
// bounded hot command cache evicts an old ID. Journal implementations may use
// an on-disk index; a linear scan is acceptable because this path is rare.
type CommandJournalLookup interface {
	LookupCommand(context.Context, CommandID) (CommandRecord, bool, error)
}

type JournalStore interface {
	OpenJournal(context.Context, string) (Journal, error)
}

type MemoryJournalStore struct {
	mu       sync.Mutex
	journals map[string]*memoryJournalData
	leases   map[string]chan struct{}
}

func NewMemoryJournalStore() *MemoryJournalStore {
	return &MemoryJournalStore{
		journals: make(map[string]*memoryJournalData),
		leases:   make(map[string]chan struct{}),
	}
}

func (s *MemoryJournalStore) OpenJournal(ctx context.Context, key string) (Journal, error) {
	if s == nil {
		return nil, fmt.Errorf("memory journal store is nil")
	}
	s.mu.Lock()
	data := s.journals[key]
	if data == nil {
		data = &memoryJournalData{}
		s.journals[key] = data
	}
	lease := s.leases[key]
	if lease == nil {
		lease = make(chan struct{}, 1)
		lease <- struct{}{}
		s.leases[key] = lease
	}
	s.mu.Unlock()

	select {
	case <-lease:
		return &memoryJournal{data: data, release: func() error {
			lease <- struct{}{}
			return nil
		}}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

type memoryJournalData struct {
	mu     sync.Mutex
	events []Event
}

type memoryJournal struct {
	data      *memoryJournalData
	release   func() error
	closeOnce sync.Once
	closeErr  error
}

func (j *memoryJournal) Load(ctx context.Context) ([]Event, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	j.data.mu.Lock()
	defer j.data.mu.Unlock()
	return cloneEvents(j.data.events), nil
}

func (j *memoryJournal) Append(ctx context.Context, expected Cursor, payloads []EventPayload) ([]Event, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	j.data.mu.Lock()
	defer j.data.mu.Unlock()
	current := Cursor(len(j.data.events))
	if current != expected {
		return nil, fmt.Errorf("journal cursor conflict: have %d, expected %d", current, expected)
	}
	committed := make([]Event, 0, len(payloads))
	for _, payload := range payloads {
		current++
		committed = append(committed, Event{Cursor: current, Durability: EventDurable, Payload: payload})
	}
	// The journal owns its durable copy. In particular, RawMessage payloads in
	// a HostEffect must not remain caller-owned and mutable after Append.
	j.data.events = append(j.data.events, cloneEvents(committed)...)
	return cloneEvents(committed), nil
}

func (j *memoryJournal) LookupCommand(ctx context.Context, commandID CommandID) (CommandRecord, bool, error) {
	if err := ctx.Err(); err != nil {
		return CommandRecord{}, false, err
	}
	j.data.mu.Lock()
	defer j.data.mu.Unlock()
	for _, event := range j.data.events {
		if accepted, ok := event.Payload.(CommandAcceptedEvent); ok && accepted.CommandID == commandID {
			return CommandRecord{
				Receipt:     Receipt{CommandID: commandID, OperationID: accepted.OperationID, Cursor: event.Cursor},
				Fingerprint: accepted.Fingerprint,
			}, true, nil
		}
	}
	return CommandRecord{}, false, nil
}

func (j *memoryJournal) Close() error {
	if j == nil {
		return nil
	}
	j.closeOnce.Do(func() {
		if j.release != nil {
			j.closeErr = j.release()
		}
	})
	return j.closeErr
}

func cloneEvents(events []Event) []Event {
	cloned := make([]Event, len(events))
	for index, event := range events {
		event.Payload = cloneEventPayload(event.Payload)
		cloned[index] = event
	}
	return cloned
}

func cloneEventPayload(payload EventPayload) EventPayload {
	switch payload := payload.(type) {
	case OperationStartedEvent:
		payload.Structural = cloneStructuralOperationSnapshot(payload.Structural)
		return payload
	case QueueEnqueuedEvent:
		payload.Item = cloneQueuedInput(payload.Item)
		return payload
	case UserMessageCommittedEvent:
		payload.Message = cloneMessage(payload.Message)
		return payload
	case AssistantMessageCommittedEvent:
		payload.Message = cloneMessage(payload.Message)
		return payload
	case ToolCallStartedEvent:
		payload.Call = normalizeToolCallState(payload.Call)
		return payload
	case CommandAcceptedEvent, QueueConsumedEvent, QueueSteerRequestedEvent,
		QueueCancelledEvent, CycleStartedEvent, OperationRecoveryPausedEvent,
		InputMaterializationRecoveryPendingEvent, InputMaterializationRecoveryResumedEvent, ToolCallFinishedEvent,
		HostEffectAcknowledgedEvent, HostEffectAbandonedEvent,
		AbortRequestedEvent, SavePointCommittedEvent, OperationSettledEvent,
		OperationInterruptedEvent, AssistantDeltaEvent, ThinkingDeltaEvent,
		ToolProgressEvent, DomainCommitIntentAcceptedEvent, DomainCommitReconciliationAbandonedEvent, DomainCommitReceiptEvent:
		if finished, ok := payload.(ToolCallFinishedEvent); ok {
			return normalizeToolFinished(finished)
		}
		return payload
	default:
		return payload
	}
}
