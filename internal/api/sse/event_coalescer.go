package sse

import (
	"reflect"
	"time"

	novaApp "denova/internal/app"
	apptask "denova/internal/app/task"
)

// This is a transport flush window, not an Agent timeout. It bounds display
// latency while collapsing provider character deltas before repeated SSE
// metadata reaches the browser.
const taskEventCoalesceWindow = 8 * time.Millisecond

func coalesceTaskEvents(events []apptask.Event) []apptask.Event {
	if len(events) < 2 {
		return append([]apptask.Event(nil), events...)
	}
	coalesced := make([]apptask.Event, 0, len(events))
	for _, item := range events {
		last := len(coalesced) - 1
		if last >= 0 {
			if merged, ok := mergeTaskEvents(coalesced[last], item); ok {
				coalesced[last] = merged
				continue
			}
		}
		coalesced = append(coalesced, item)
	}
	return coalesced
}

func mergeTaskEvents(left, right apptask.Event) (apptask.Event, bool) {
	field := coalescedTaskEventField(left.Event.Type)
	if field == "" || left.Event.Type != right.Event.Type || right.Cursor < left.Cursor {
		return apptask.Event{}, false
	}
	leftData, leftOK := taskEventDataMap(left.Event.Data)
	rightData, rightOK := taskEventDataMap(right.Event.Data)
	if !leftOK || !rightOK {
		return apptask.Event{}, false
	}
	leftText, leftOK := leftData[field].(string)
	rightText, rightOK := rightData[field].(string)
	if !leftOK || !rightOK {
		return apptask.Event{}, false
	}
	delete(leftData, field)
	delete(rightData, field)
	if !reflect.DeepEqual(leftData, rightData) {
		return apptask.Event{}, false
	}
	leftData[field] = leftText + rightText
	return apptask.Event{
		Cursor: right.Cursor,
		Event:  novaApp.AgentEvent{Type: left.Event.Type, Data: leftData},
	}, true
}

func coalescedTaskEventField(eventType string) string {
	switch eventType {
	case "chunk", "thinking":
		return "content"
	case "tool_args_delta":
		return "delta"
	default:
		return ""
	}
}

func taskEventDataMap(value any) (map[string]any, bool) {
	switch data := value.(type) {
	case map[string]any:
		cloned := make(map[string]any, len(data))
		for key, item := range data {
			cloned[key] = item
		}
		return cloned, true
	case map[string]string:
		cloned := make(map[string]any, len(data))
		for key, item := range data {
			cloned[key] = item
		}
		return cloned, true
	default:
		return nil, false
	}
}

// writeCoalescedTaskEventStream preserves event order and exact delta text.
// Only adjacent deltas from the same display source share one final cursor;
// all semantic boundaries flush immediately.
func writeCoalescedTaskEventStream(events <-chan apptask.Event, write func(apptask.Event) error) (apptask.Event, error) {
	var pending *apptask.Event
	var flush <-chan time.Time
	for {
		if pending == nil {
			item, ok := <-events
			if !ok {
				return apptask.Event{}, nil
			}
			if coalescedTaskEventField(item.Event.Type) == "" {
				if err := write(item); err != nil {
					return item, err
				}
				continue
			}
			pending = &item
			flush = time.After(taskEventCoalesceWindow)
			continue
		}

		select {
		case item, ok := <-events:
			if !ok {
				if err := write(*pending); err != nil {
					return *pending, err
				}
				return apptask.Event{}, nil
			}
			if merged, ok := mergeTaskEvents(*pending, item); ok {
				pending = &merged
				continue
			}
			if err := write(*pending); err != nil {
				return *pending, err
			}
			pending = nil
			flush = nil
			if coalescedTaskEventField(item.Event.Type) == "" {
				if err := write(item); err != nil {
					return item, err
				}
				continue
			}
			pending = &item
			flush = time.After(taskEventCoalesceWindow)
		case <-flush:
			if err := write(*pending); err != nil {
				return *pending, err
			}
			pending = nil
			flush = nil
		}
	}
}
