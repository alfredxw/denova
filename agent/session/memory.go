package session

import (
	"context"
	"fmt"
	"sync"
)

// Memory returns a concurrency-safe Store with production-equivalent lease,
// replay, and atomic CAS behavior. Data lasts for the Store lifetime.
func Memory() Store {
	return &memoryStore{
		logs:   make(map[string]*memoryData),
		leases: make(map[string]chan struct{}),
	}
}

type memoryStore struct {
	mu     sync.Mutex
	logs   map[string]*memoryData
	leases map[string]chan struct{}
}

func (*memoryStore) Volatile() bool { return true }

type memoryData struct {
	mu      sync.Mutex
	records []Record
}

func (store *memoryStore) Open(ctx context.Context, key Key) (Log, error) {
	if store == nil {
		return nil, fmt.Errorf("open memory agent session: store is nil")
	}
	canonical, err := CanonicalKey(key)
	if err != nil {
		return nil, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	store.mu.Lock()
	data := store.logs[canonical]
	if data == nil {
		data = &memoryData{}
		store.logs[canonical] = data
	}
	lease := store.leases[canonical]
	if lease == nil {
		lease = make(chan struct{}, 1)
		lease <- struct{}{}
		store.leases[canonical] = lease
	}
	store.mu.Unlock()

	select {
	case <-lease:
		return &memoryLog{data: data, release: func() { lease <- struct{}{} }}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
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
