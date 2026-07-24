package session

import (
	"context"
	"fmt"
	"sync"

	"github.com/alfredxw/denova/agent"
)

// MemoryStore is a concurrency-safe Store for tests, embedding, and ephemeral
// agents. It has exactly the same append-only CAS semantics expected from a
// durable Store implementation.
type MemoryStore struct {
	mu        sync.RWMutex
	snapshots map[string]Snapshot
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{snapshots: make(map[string]Snapshot)}
}

func (store *MemoryStore) Load(ctx context.Context, id string) (Snapshot, error) {
	if err := ctx.Err(); err != nil {
		return Snapshot{}, err
	}
	id, err := normalizeID(id)
	if err != nil {
		return Snapshot{}, err
	}
	store.mu.RLock()
	snapshot, found := store.snapshots[id]
	store.mu.RUnlock()
	if !found {
		return Snapshot{ID: id}, nil
	}
	return cloneSnapshot(snapshot), nil
}

func (store *MemoryStore) CompareAndSwap(ctx context.Context, id string, expected Revision, mutations ...Mutation) (Snapshot, error) {
	if err := ctx.Err(); err != nil {
		return Snapshot{}, err
	}
	id, err := normalizeID(id)
	if err != nil {
		return Snapshot{}, err
	}
	for index, mutation := range mutations {
		if err := validateMutation(mutation); err != nil {
			return Snapshot{}, fmt.Errorf("agent session mutation %d: %w", index, err)
		}
	}

	store.mu.Lock()
	defer store.mu.Unlock()
	current, found := store.snapshots[id]
	if !found {
		current = Snapshot{ID: id}
	}
	if current.Revision != expected {
		return Snapshot{}, &RevisionConflictError{Expected: expected, Actual: current.Revision}
	}
	if err := ctx.Err(); err != nil {
		return Snapshot{}, err
	}
	next := cloneSnapshot(current)
	for _, mutation := range mutations {
		next.Revision++
		next.entries = append(next.entries, Entry{
			Revision: next.Revision,
			Type:     mutation.Type,
			Message:  agent.CloneMessage(mutation.Message),
		})
	}
	store.snapshots[id] = cloneSnapshot(next)
	return cloneSnapshot(next), nil
}

// RevisionConflictError reports both sides of a rejected CAS and supports
// errors.Is(err, ErrRevisionConflict).
type RevisionConflictError struct {
	Expected Revision
	Actual   Revision
}

func (err *RevisionConflictError) Error() string {
	return fmt.Sprintf("%v: expected=%d actual=%d", ErrRevisionConflict, err.Expected, err.Actual)
}

func (err *RevisionConflictError) Unwrap() error {
	return ErrRevisionConflict
}

func cloneSnapshot(snapshot Snapshot) Snapshot {
	clone := Snapshot{ID: snapshot.ID, Revision: snapshot.Revision, entries: make([]Entry, len(snapshot.entries))}
	for index, entry := range snapshot.entries {
		clone.entries[index] = cloneEntry(entry)
	}
	return clone
}
