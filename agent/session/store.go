// Package session defines the transcript storage seam used by Agent.
//
// Stores own append-only records and an exclusive execution lease per exact
// Key. They do not interpret Agent record kinds, model messages, or product
// state. Implementations can therefore use files, databases, object storage,
// or a remote log without depending on Agent internals.
package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"strings"
)

const (
	MaxKeyFieldBytes   = 64 << 10
	MaxAttributes      = 64
	MaxRecordKindBytes = 4 << 10
	DefaultNamespace   = "default"
)

var (
	ErrInvalidKey       = errors.New("invalid agent session key")
	ErrInvalidRecord    = errors.New("invalid agent session record")
	ErrRevisionConflict = errors.New("agent session revision conflict")
	ErrLogClosed        = errors.New("agent session log is closed")
	// ErrCommitUnknown means an I/O failure happened after a write may have
	// crossed the storage boundary. Callers must close the lease and replay;
	// they must never blindly retry the same expected revision in-place.
	ErrCommitUnknown = errors.New("agent session append commit is unknown")
)

// Key is the exact storage identity of one Session. Attributes participate in
// identity; mutable display metadata must not be stored here.
type Key struct {
	Namespace  string            `json:"namespace"`
	ID         string            `json:"id"`
	Attributes map[string]string `json:"attributes,omitempty"`
}

// Named constructs a Key in the default namespace.
func Named(id string) Key { return Key{Namespace: DefaultNamespace, ID: id} }

// NormalizeKey validates and defensively clones a Session identity.
func NormalizeKey(key Key) (Key, error) {
	key.Namespace = strings.TrimSpace(key.Namespace)
	key.ID = strings.TrimSpace(key.ID)
	if key.Namespace == "" {
		key.Namespace = DefaultNamespace
	}
	if key.ID == "" || len(key.Namespace) > MaxKeyFieldBytes || len(key.ID) > MaxKeyFieldBytes {
		return Key{}, ErrInvalidKey
	}
	if len(key.Attributes) > MaxAttributes {
		return Key{}, fmt.Errorf("%w: attributes exceed %d", ErrInvalidKey, MaxAttributes)
	}
	key.Attributes = maps.Clone(key.Attributes)
	for name, value := range key.Attributes {
		if strings.TrimSpace(name) != name || name == "" || strings.TrimSpace(value) != value ||
			len(name) > MaxKeyFieldBytes || len(value) > MaxKeyFieldBytes {
			return Key{}, fmt.Errorf("%w: malformed attribute", ErrInvalidKey)
		}
	}
	return key, nil
}

// CanonicalKey returns the stable storage identity of a validated Key.
func CanonicalKey(key Key) (string, error) {
	normalized, err := NormalizeKey(key)
	if err != nil {
		return "", err
	}
	encoded, err := json.Marshal(normalized)
	if err != nil {
		return "", fmt.Errorf("encode agent session key: %w", err)
	}
	return string(encoded), nil
}

// Selector identifies a non-empty lifecycle scope. Every populated field is
// an AND predicate; IDPrefix is intentionally explicit and never implied by ID.
type Selector struct {
	Namespace  string
	ID         string
	IDPrefix   string
	Attributes map[string]string
	// All is reserved for owner-level catalog operations such as recursive
	// descendant deletion. Product lifecycle calls should prefer constrained
	// selectors so accidental broad mutations remain impossible.
	All bool
}

func (selector Selector) Validate() error {
	if !selector.All && strings.TrimSpace(selector.Namespace) == "" && strings.TrimSpace(selector.ID) == "" &&
		strings.TrimSpace(selector.IDPrefix) == "" && len(selector.Attributes) == 0 {
		return fmt.Errorf("%w: selector must constrain at least one field", ErrInvalidKey)
	}
	probe := Key{Namespace: selector.Namespace, ID: selector.ID, Attributes: selector.Attributes}
	if probe.Namespace == "" {
		probe.Namespace = DefaultNamespace
	}
	if probe.ID == "" {
		probe.ID = "selector"
	}
	if _, err := NormalizeKey(probe); err != nil {
		return err
	}
	if prefix := strings.TrimSpace(selector.IDPrefix); prefix != selector.IDPrefix || len(prefix) > MaxKeyFieldBytes {
		return ErrInvalidKey
	}
	return nil
}

func (selector Selector) Matches(key Key) bool {
	if selector.Validate() != nil {
		return false
	}
	key, err := NormalizeKey(key)
	if err != nil {
		return false
	}
	if selector.All {
		return true
	}
	if selector.Namespace != "" && selector.Namespace != key.Namespace ||
		selector.ID != "" && selector.ID != key.ID ||
		selector.IDPrefix != "" && !strings.HasPrefix(key.ID, selector.IDPrefix) {
		return false
	}
	for name, value := range selector.Attributes {
		if key.Attributes[name] != value {
			return false
		}
	}
	return true
}

type Revision uint64

// Record is one immutable Session transcript record. Kind and Version select the
// codec; Data must be a complete JSON value. Revision is assigned by Log.
type Record struct {
	Revision Revision        `json:"revision"`
	Kind     string          `json:"kind"`
	Version  uint16          `json:"version"`
	Data     json.RawMessage `json:"data"`
}

func ValidateRecord(record Record) error {
	if strings.TrimSpace(record.Kind) == "" || strings.TrimSpace(record.Kind) != record.Kind ||
		len(record.Kind) > MaxRecordKindBytes || record.Version == 0 || !json.Valid(record.Data) {
		return ErrInvalidRecord
	}
	return nil
}

type ReplayStats struct {
	RecordsRead int64
	BytesRead   int64
}

// Store owns the catalog and append-only Log for every Session Key.
// List and Delete are part of the contract because closing an execution lease
// is not equivalent to deleting user data. Delete is idempotent; implementations
// must not expose a partially deleted Session and must serialize it with Open.
type Store interface {
	Open(context.Context, Key) (Log, error)
	List(context.Context, Selector) ([]Key, error)
	Delete(context.Context, Key) error
}

// Log is the complete transcript Adapter contract. Append commits the whole batch
// atomically at expected or changes nothing. Replay is streaming and ordered.
type Log interface {
	Replay(context.Context, func(Record) error) (ReplayStats, error)
	Append(context.Context, Revision, ...Record) (Revision, error)
	Close() error
}

// CanonicalMessageLog marks a host journal whose conversation messages are
// committed by the host itself. Agent still appends capability and turn
// records to the same log, but it must not persist a second message snapshot.
//
// The marker is intentionally optional: Agent's built-in memory and file
// stores remain self-contained for standalone embedding.
type CanonicalMessageLog interface {
	Log
	CanonicalMessages() bool
}

type RevisionConflictError struct {
	Expected Revision
	Actual   Revision
}

func (err *RevisionConflictError) Error() string {
	return fmt.Sprintf("%v: expected=%d actual=%d", ErrRevisionConflict, err.Expected, err.Actual)
}

func (err *RevisionConflictError) Unwrap() error { return ErrRevisionConflict }

func cloneRecord(record Record) Record {
	record.Data = append(json.RawMessage(nil), record.Data...)
	return record
}
