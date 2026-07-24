// Package session provides an append-only, provider-neutral model transcript.
//
// It intentionally excludes UI events, product metadata, compaction policy,
// and domain commits. Those concerns may share a durable journal with a
// transcript, but they are not part of the stable Agent session contract.
package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/alfredxw/denova/agent"
)

var ErrRevisionConflict = errors.New("agent session revision conflict")

// Revision is the compare-and-swap version of one append-only transcript.
type Revision uint64

type EntryType string

const (
	EntryMessage EntryType = "message"
	EntryClear   EntryType = "clear"
)

// Entry is one immutable transcript event. Clear entries are filtering
// markers: earlier messages remain durable and available through Messages.
type Entry struct {
	Revision Revision       `json:"revision"`
	Type     EntryType      `json:"type"`
	Message  *agent.Message `json:"message,omitempty"`
}

// Mutation is an append request passed through the Store CAS boundary.
type Mutation struct {
	Type    EntryType
	Message *agent.Message
}

func AppendMessage(message *agent.Message) Mutation {
	return Mutation{Type: EntryMessage, Message: message}
}

func MarkClear() Mutation {
	return Mutation{Type: EntryClear}
}

// Snapshot is an immutable view of one complete transcript. The entry slice
// is private so messages cannot be changed behind a Store's revision barrier.
type Snapshot struct {
	ID       string   `json:"id"`
	Revision Revision `json:"revision"`
	entries  []Entry
}

type snapshotWire struct {
	ID       string   `json:"id"`
	Revision Revision `json:"revision"`
	Entries  []Entry  `json:"entries"`
}

func (snapshot Snapshot) MarshalJSON() ([]byte, error) {
	return json.Marshal(snapshotWire{ID: snapshot.ID, Revision: snapshot.Revision, Entries: snapshot.Entries()})
}

func (snapshot *Snapshot) UnmarshalJSON(data []byte) error {
	if snapshot == nil {
		return fmt.Errorf("unmarshal agent session snapshot into nil receiver")
	}
	var wire snapshotWire
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	restored, err := Restore(wire.ID, wire.Revision, wire.Entries)
	if err != nil {
		return err
	}
	*snapshot = restored
	return nil
}

// Restore validates and clones persisted entries for a Store implementation.
// Revision may be newer than the last transcript entry when a product shares
// the same CAS revision with other model-context state such as compaction.
func Restore(id string, revision Revision, entries []Entry) (Snapshot, error) {
	id, err := normalizeID(id)
	if err != nil {
		return Snapshot{}, err
	}
	result := Snapshot{ID: id, Revision: revision, entries: make([]Entry, 0, len(entries))}
	var previous Revision
	for index, entry := range entries {
		if entry.Revision == 0 || entry.Revision <= previous || entry.Revision > revision {
			return Snapshot{}, fmt.Errorf("restore agent session %q: entry %d has invalid revision %d", id, index, entry.Revision)
		}
		if err := validateEntry(entry); err != nil {
			return Snapshot{}, fmt.Errorf("restore agent session %q: entry %d: %w", id, index, err)
		}
		result.entries = append(result.entries, cloneEntry(entry))
		previous = entry.Revision
	}
	return result, nil
}

// Entries returns a deep copy suitable for persistence or audit.
func (snapshot Snapshot) Entries() []Entry {
	entries := make([]Entry, len(snapshot.entries))
	for index, entry := range snapshot.entries {
		entries[index] = cloneEntry(entry)
	}
	return entries
}

// Messages returns the complete raw model transcript. Clear markers are not
// messages and do not physically remove anything from this view.
func (snapshot Snapshot) Messages() []*agent.Message {
	return messagesAfter(snapshot.entries, 0)
}

// EffectiveMessages returns messages after the latest clear marker. Display
// events and product logs never enter this transcript in the first place.
func (snapshot Snapshot) EffectiveMessages() []*agent.Message {
	start := 0
	for index := len(snapshot.entries) - 1; index >= 0; index-- {
		if snapshot.entries[index].Type == EntryClear {
			start = index + 1
			break
		}
	}
	return messagesAfter(snapshot.entries, start)
}

func messagesAfter(entries []Entry, start int) []*agent.Message {
	messages := make([]*agent.Message, 0, len(entries)-start)
	for _, entry := range entries[start:] {
		if entry.Type == EntryMessage {
			messages = append(messages, agent.CloneMessage(entry.Message))
		}
	}
	return messages
}

// Store is the durable transcript Seam. Implementations append mutations only
// when expected matches the current revision; a missing ID loads as an empty
// revision-zero transcript so creation and update share the same operation.
type Store interface {
	Load(context.Context, string) (Snapshot, error)
	CompareAndSwap(context.Context, string, Revision, ...Mutation) (Snapshot, error)
}

// Session binds a stable ID to a Store while keeping revision ownership at the
// call site. Callers must load a Snapshot and commit against its Revision.
type Session struct {
	id    string
	store Store
}

func Open(id string, store Store) (*Session, error) {
	id, err := normalizeID(id)
	if err != nil {
		return nil, err
	}
	if store == nil {
		return nil, fmt.Errorf("open agent session %q: store is nil", id)
	}
	return &Session{id: id, store: store}, nil
}

func (session *Session) ID() string {
	if session == nil {
		return ""
	}
	return session.id
}

func (session *Session) Snapshot(ctx context.Context) (Snapshot, error) {
	if session == nil || session.store == nil {
		return Snapshot{}, fmt.Errorf("agent session is nil")
	}
	return session.store.Load(ctx, session.id)
}

// Commit atomically appends mutations at the expected transcript revision.
func (session *Session) Commit(ctx context.Context, expected Revision, mutations ...Mutation) (Snapshot, error) {
	if session == nil || session.store == nil {
		return Snapshot{}, fmt.Errorf("agent session is nil")
	}
	return session.store.CompareAndSwap(ctx, session.id, expected, mutations...)
}

func (session *Session) Append(ctx context.Context, expected Revision, message *agent.Message) (Snapshot, error) {
	return session.Commit(ctx, expected, AppendMessage(message))
}

func (session *Session) Clear(ctx context.Context, expected Revision) (Snapshot, error) {
	return session.Commit(ctx, expected, MarkClear())
}

func normalizeID(id string) (string, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return "", fmt.Errorf("agent session id is required")
	}
	return id, nil
}

func validateEntry(entry Entry) error {
	switch entry.Type {
	case EntryMessage:
		return validateMessage(entry.Message)
	case EntryClear:
		if entry.Message != nil {
			return fmt.Errorf("clear entry must not contain a message")
		}
		return nil
	default:
		return fmt.Errorf("unsupported transcript entry type %q", entry.Type)
	}
}

func validateMutation(mutation Mutation) error {
	return validateEntry(Entry{Revision: 1, Type: mutation.Type, Message: mutation.Message})
}

func validateMessage(message *agent.Message) error {
	if message == nil {
		return fmt.Errorf("message is nil")
	}
	if message.Role == "" && strings.TrimSpace(message.Content) == "" && len(message.ToolCalls) == 0 && len(message.MultiContent) == 0 && len(message.UserInputMultiContent) == 0 && len(message.AssistantGenMultiContent) == 0 {
		return fmt.Errorf("message has no role, content, or tool calls")
	}
	return nil
}

func cloneEntry(entry Entry) Entry {
	entry.Message = agent.CloneMessage(entry.Message)
	return entry
}
