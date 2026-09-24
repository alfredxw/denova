package sessionfile

import (
	"context"
	"fmt"
	"os"

	"github.com/alfredxw/denova/agent/session"
)

// ReadRecord reuses offsets collected during checksum-verified replay/append.
// The map is disposable; the canonical file format is unchanged.
func (log *logFile) ReadRecord(ctx context.Context, revision session.Revision) (session.Record, error) {
	log.mu.Lock()
	defer log.mu.Unlock()
	if log.closed {
		return session.Record{}, session.ErrLogClosed
	}
	if ctx != nil && ctx.Err() != nil {
		return session.Record{}, ctx.Err()
	}
	if !log.initialized {
		if _, err := log.replayLocked(ctx, func(session.Record) error { return nil }); err != nil {
			return session.Record{}, err
		}
	}
	location, found := log.locations[revision]
	if !found {
		return session.Record{}, fmt.Errorf("Agent record revision %d is missing", revision)
	}
	file, err := os.Open(log.path)
	if err != nil {
		return session.Record{}, err
	}
	defer file.Close()
	data := make([]byte, location.length)
	if _, err := file.ReadAt(data, location.offset); err != nil {
		return session.Record{}, err
	}
	records, err := decodeTransaction(data[:len(data)-1], location.previous)
	if err != nil {
		return session.Record{}, err
	}
	for _, record := range records {
		if record.Revision == revision {
			return record, nil
		}
	}
	return session.Record{}, fmt.Errorf("Agent record revision %d is missing from indexed transaction", revision)
}
