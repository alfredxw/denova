// Package conversationjournal provides the durable append-only substrate for
// user-visible conversations. Product domains keep their event schemas and
// projections; this package owns only physical journal correctness.
package conversationjournal

import (
	"encoding/json"
	"errors"
	"fmt"
)

const (
	transactionKind    = "denova.conversation.append"
	transactionVersion = 1
	indexVersion       = 1

	defaultFlushEvery    = 128
	defaultSparseEvery   = 1024
	defaultRecentRecords = 200
)

var ErrConflict = errors.New("conversation journal revision conflict")

// Cursor is the monotonically increasing physical transaction position. A
// legacy JSONL record is treated as one transaction during mixed-format replay.
type Cursor uint64

// Identity prevents a handle opened for a deleted journal incarnation from
// writing into a newly-created file at the same path.
type Identity struct {
	ID         string `json:"id"`
	Generation string `json:"generation"`
}

// Head is the exact append barrier observed by a caller.
type Head struct {
	Identity         Identity `json:"identity"`
	Cursor           Cursor   `json:"cursor"`
	RecordSHA256     string   `json:"record_sha256,omitempty"`
	VerifiedBytes    int64    `json:"verified_bytes"`
	TransactionCount int64    `json:"transaction_count"`
}

// Guard rejects writes prepared from stale canonical state.
type Guard struct {
	Cursor       Cursor
	RecordSHA256 string
}

// Location identifies one domain record inside a physical JSONL transaction.
// Offset and Length are implementation details intentionally kept inside the
// storage and domain adapters; HTTP cursors must remain opaque.
type Location struct {
	Cursor               Cursor `json:"cursor"`
	Offset               int64  `json:"offset"`
	Length               int    `json:"length"`
	RecordIndex          int    `json:"record_index,omitempty"`
	PreviousRecordSHA256 string `json:"previous_record_sha256,omitempty"`
}

// Record is the immutable payload delivered to a domain Reducer.
type Record struct {
	Location Location
	Payload  json.RawMessage
	Legacy   bool
}

// Commit reports a durably appended transaction and its domain records.
type Commit struct {
	Head    Head
	Records []Record
}

// Range selects physical cursors after After and through Through. Through=0
// means the current head. Limit bounds physical transactions, not bytes.
type Range struct {
	After   Cursor
	Through Cursor
	Limit   int
}

// ReplayStats makes the hot-path complexity observable without exposing file
// handles or index internals.
type ReplayStats struct {
	IndexLoaded        bool  `json:"index_loaded"`
	IndexRebuilt       bool  `json:"index_rebuilt"`
	BytesRead          int64 `json:"bytes_read"`
	TransactionsRead   int64 `json:"transactions_read"`
	DomainRecordsRead  int64 `json:"domain_records_read"`
	TailBytesRead      int64 `json:"tail_bytes_read"`
	TailRecordsRead    int64 `json:"tail_records_read"`
	LastRangeBytesRead int64 `json:"last_range_bytes_read"`
}

// Reducer is the internal adapter seam between physical journal mechanics and
// a domain projection. Checkpoints are derived and must never be the sole copy
// of user content.
type Reducer interface {
	Reset() error
	Restore(json.RawMessage) error
	Apply(Record) error
	Checkpoint() (json.RawMessage, error)
}

// Options contains implementation tuning, not user-facing product settings.
type Options struct {
	FlushEvery    int
	SparseEvery   int
	RecentRecords int
}

func (options Options) normalized() Options {
	if options.FlushEvery <= 0 {
		options.FlushEvery = defaultFlushEvery
	}
	if options.SparseEvery <= 0 {
		options.SparseEvery = defaultSparseEvery
	}
	if options.RecentRecords <= 0 {
		options.RecentRecords = defaultRecentRecords
	}
	return options
}

// ConflictError includes both sides of a failed append CAS.
type ConflictError struct {
	Expected Guard
	Actual   Head
}

func (err *ConflictError) Error() string {
	return fmt.Sprintf("%v: expected_cursor=%d actual_cursor=%d", ErrConflict, err.Expected.Cursor, err.Actual.Cursor)
}

func (err *ConflictError) Unwrap() error { return ErrConflict }

// SidecarPath returns the rebuildable index path for a canonical journal.
func SidecarPath(journalPath string) string { return sidecarPath(journalPath) }
