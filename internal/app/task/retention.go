package task

import (
	"encoding/json"
	"errors"
	"fmt"

	"denova/internal/agents/run"
)

const (
	// Display retention is deliberately separate from model context and the
	// durable Agent journal. Live subscribers still receive every event; these
	// limits only bound the in-memory replay window for detached SSE clients.
	DefaultRetainedEventLimit = 8192
	DefaultRetainedByteLimit  = 32 << 20
)

// ErrCursorExpired means the requested display suffix has already left
// the bounded replay window. Callers must recover from canonical history or a
// runtime projection instead of silently accepting a partial event stream.
var ErrCursorExpired = errors.New("task event cursor is older than the retained display window")

func (t *Task) appendRetainedEventLocked(ev agentrun.Event) Event {
	t.nextCursor++
	item := Event{Cursor: t.nextCursor, Event: ev}
	size := taskEventSize(item)
	t.events = append(t.events, item)
	t.eventBytes = append(t.eventBytes, size)
	t.retainedBytes += size

	eventLimit := t.retainedEventLimit
	if eventLimit <= 0 {
		eventLimit = DefaultRetainedEventLimit
	}
	byteLimit := t.retainedByteLimit
	if byteLimit <= 0 {
		byteLimit = DefaultRetainedByteLimit
	}
	for len(t.events) > 0 && (len(t.events) > eventLimit || t.retainedBytes > byteLimit) {
		t.eventBaseCursor = t.events[0].Cursor
		t.retainedBytes -= t.eventBytes[0]
		t.events[0] = Event{}
		t.eventBytes[0] = 0
		t.events = t.events[1:]
		t.eventBytes = t.eventBytes[1:]
	}
	return item
}

func (t *Task) replayAfterLocked(after uint64) ([]Event, error) {
	if after > t.nextCursor {
		return nil, fmt.Errorf("%w: after=%d latest=%d", ErrCursorAhead, after, t.nextCursor)
	}
	if after < t.eventBaseCursor {
		return nil, fmt.Errorf(
			"%w: after=%d earliest=%d latest=%d",
			ErrCursorExpired,
			after,
			t.eventBaseCursor+1,
			t.nextCursor,
		)
	}
	start := int(after - t.eventBaseCursor)
	snapshot := make([]Event, len(t.events)-start)
	copy(snapshot, t.events[start:])
	return snapshot, nil
}

func taskEventSize(item Event) int {
	if size, ok := estimateEventDataSize(item.Event.Data); ok {
		return len(item.Event.Type) + size + 16
	}
	data, err := json.Marshal(item.Event.Data)
	if err != nil {
		// The SSE encoder will surface the real serialization error. Keep a
		// conservative accounting charge meanwhile rather than making display
		// retention itself a second failure boundary.
		return len(item.Event.Type) + 256
	}
	return len(item.Event.Type) + len(data) + 16
}

func estimateEventDataSize(value any) (int, bool) {
	switch typed := value.(type) {
	case nil:
		return 4, true
	case string:
		return len(typed) + 2, true
	case map[string]string:
		size := 2
		for k, v := range typed {
			size += len(k) + len(v) + 6
		}
		return size, true
	case map[string]any:
		size := 2
		for k, v := range typed {
			elemSize, ok := estimateScalarSize(v)
			if !ok {
				return 0, false
			}
			size += len(k) + elemSize + 4
		}
		return size, true
	default:
		return 0, false
	}
}

func estimateScalarSize(value any) (int, bool) {
	switch typed := value.(type) {
	case nil:
		return 4, true
	case string:
		return len(typed) + 2, true
	case bool:
		return 5, true
	case int, int32, int64, uint, uint32, uint64, float32, float64:
		return 16, true
	case []string:
		size := 2
		for _, item := range typed {
			size += len(item) + 3
		}
		return size, true
	default:
		return 0, false
	}
}
