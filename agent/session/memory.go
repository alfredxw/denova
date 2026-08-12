package session

import (
	"context"
	"fmt"
	"maps"
	"sort"
	"sync"
)

// Memory returns a concurrency-safe Store with production-equivalent lease,
// replay, and atomic CAS behavior. Data lasts for the Store lifetime.
func Memory() Store {
	return &memoryStore{
		entries: make(map[string]*memoryEntry),
	}
}

type memoryStore struct {
	mu      sync.Mutex
	entries map[string]*memoryEntry
}

func (*memoryStore) Volatile() bool { return true }

type memoryData struct {
	mu      sync.Mutex
	records []Record
}

type memoryEntry struct {
	key   Key
	data  *memoryData
	lease chan struct{}
}

func (store *memoryStore) Open(ctx context.Context, key Key) (Log, error) {
	if store == nil {
		return nil, fmt.Errorf("open memory agent session: store is nil")
	}
	key, err := NormalizeKey(key)
	if err != nil {
		return nil, err
	}
	canonical, err := CanonicalKey(key)
	if err != nil {
		return nil, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	for {
		store.mu.Lock()
		entry := store.entries[canonical]
		if entry == nil {
			entry = &memoryEntry{key: key, data: &memoryData{}, lease: make(chan struct{}, 1)}
			entry.lease <- struct{}{}
			store.entries[canonical] = entry
		}
		store.mu.Unlock()

		select {
		case <-entry.lease:
			// Delete can replace the catalog entry while this Open waits for the
			// old lease. Revalidate after acquisition so deleted records can never
			// be reopened through a stale waiter.
			store.mu.Lock()
			current := store.entries[canonical]
			store.mu.Unlock()
			if current != entry {
				entry.lease <- struct{}{}
				continue
			}
			return &memoryLog{data: entry.data, release: func() { entry.lease <- struct{}{} }}, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

func (store *memoryStore) List(ctx context.Context, selector Selector) ([]Key, error) {
	if store == nil {
		return nil, fmt.Errorf("list memory agent sessions: store is nil")
	}
	if err := selector.Validate(); err != nil {
		return nil, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	result := make([]Key, 0, len(store.entries))
	for _, entry := range store.entries {
		if selector.Matches(entry.key) {
			key := entry.key
			key.Attributes = maps.Clone(key.Attributes)
			result = append(result, key)
		}
	}
	sort.Slice(result, func(left, right int) bool {
		leftKey, _ := CanonicalKey(result[left])
		rightKey, _ := CanonicalKey(result[right])
		return leftKey < rightKey
	})
	return result, nil
}

func (store *memoryStore) Delete(ctx context.Context, key Key) error {
	if store == nil {
		return fmt.Errorf("delete memory agent session: store is nil")
	}
	key, err := NormalizeKey(key)
	if err != nil {
		return err
	}
	canonical, err := CanonicalKey(key)
	if err != nil {
		return err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	store.mu.Lock()
	entry := store.entries[canonical]
	store.mu.Unlock()
	if entry == nil {
		return nil
	}
	select {
	case <-entry.lease:
		store.mu.Lock()
		if store.entries[canonical] == entry {
			delete(store.entries, canonical)
		}
		store.mu.Unlock()
		// Wake stale Open waiters. They revalidate the catalog generation and
		// retry against a fresh, empty entry.
		entry.lease <- struct{}{}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

type memoryLog struct {
	data      *memoryData
	release   func()
	closeOnce sync.Once
	closedMu  sync.RWMutex
	closed    bool
}

func (log *memoryLog) Replay(ctx context.Context, apply func(Record) error) (ReplayStats, error) {
	if apply == nil {
		return ReplayStats{}, fmt.Errorf("replay agent session: reducer is required")
	}
	if err := log.usable(); err != nil {
		return ReplayStats{}, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	log.data.mu.Lock()
	records := make([]Record, len(log.data.records))
	for index, record := range log.data.records {
		records[index] = cloneRecord(record)
	}
	log.data.mu.Unlock()
	stats := ReplayStats{RecordsRead: int64(len(records))}
	for _, record := range records {
		if err := ctx.Err(); err != nil {
			return stats, err
		}
		stats.BytesRead += int64(len(record.Kind) + len(record.Data))
		if err := apply(record); err != nil {
			return stats, err
		}
	}
	return stats, nil
}

func (log *memoryLog) Append(ctx context.Context, expected Revision, records ...Record) (Revision, error) {
	if err := log.usable(); err != nil {
		return 0, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	for _, record := range records {
		if err := ValidateRecord(record); err != nil {
			return 0, err
		}
	}

	log.data.mu.Lock()
	defer log.data.mu.Unlock()
	current := Revision(len(log.data.records))
	if current != expected {
		return current, &RevisionConflictError{Expected: expected, Actual: current}
	}
	if err := ctx.Err(); err != nil {
		return current, err
	}
	for _, record := range records {
		current++
		record = cloneRecord(record)
		record.Revision = current
		log.data.records = append(log.data.records, record)
	}
	return current, nil
}

func (log *memoryLog) Close() error {
	if log == nil {
		return nil
	}
	log.closeOnce.Do(func() {
		log.closedMu.Lock()
		log.closed = true
		log.closedMu.Unlock()
		if log.release != nil {
			log.release()
		}
	})
	return nil
}

func (log *memoryLog) usable() error {
	if log == nil || log.data == nil {
		return ErrLogClosed
	}
	log.closedMu.RLock()
	closed := log.closed
	log.closedMu.RUnlock()
	if closed {
		return ErrLogClosed
	}
	return nil
}
