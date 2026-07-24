package app

import (
	"encoding/json"
	"errors"
	"fmt"

	agents "denova/internal/agents"
)

const (
	// Display retention is deliberately separate from model context and the
	// durable Agent journal. Live subscribers still receive every event; these
	// limits only bound the in-memory replay window for detached SSE clients.
	defaultTaskRetainedEventLimit = 8192
	defaultTaskRetainedByteLimit  = 32 << 20
)

// ErrTaskCursorExpired means the requested display suffix has already left
// the bounded replay window. Callers must recover from canonical history or a
// runtime projection instead of silently accepting a partial event stream.
var ErrTaskCursorExpired = errors.New("task event cursor is older than the retained display window")

func (t *Task) appendRetainedEventLocked(ev agents.Event) TaskEvent {
	t.nextCursor++
	item := TaskEvent{Cursor: t.nextCursor, Event: ev}
	size := taskEventSize(item)
	t.events = append(t.events, item)
	t.eventBytes = append(t.eventBytes, size)
	t.retainedBytes += size

	eventLimit := t.retainedEventLimit
	if eventLimit <= 0 {
		eventLimit = defaultTaskRetainedEventLimit
	}
	byteLimit := t.retainedByteLimit
	if byteLimit <= 0 {
		byteLimit = defaultTaskRetainedByteLimit
	}
	for len(t.events) > 0 && (len(t.events) > eventLimit || t.retainedBytes > byteLimit) {
		t.eventBaseCursor = t.events[0].Cursor
		t.retainedBytes -= t.eventBytes[0]
		t.events[0] = TaskEvent{}
		t.eventBytes[0] = 0
		t.events = t.events[1:]
		t.eventBytes = t.eventBytes[1:]
	}
	return item
}

func (t *Task) replayAfterLocked(after uint64) ([]TaskEvent, error) {
	if after > t.nextCursor {
		return nil, fmt.Errorf("%w: after=%d latest=%d", ErrTaskCursorAhead, after, t.nextCursor)
	}
	if after < t.eventBaseCursor {
		return nil, fmt.Errorf(
			"%w: after=%d earliest=%d latest=%d",
			ErrTaskCursorExpired,
			after,
			t.eventBaseCursor+1,
			t.nextCursor,
		)
	}
	start := int(after - t.eventBaseCursor)
	snapshot := make([]TaskEvent, len(t.events)-start)
	copy(snapshot, t.events[start:])
	return snapshot, nil
}

func taskEventSize(item TaskEvent) int {
	data, err := json.Marshal(item.Event.Data)
	if err != nil {
		// The SSE encoder will surface the real serialization error. Keep a
		// conservative accounting charge meanwhile rather than making display
		// retention itself a second failure boundary.
		return len(item.Event.Type) + 256
	}
	return len(item.Event.Type) + len(data) + 16
}
