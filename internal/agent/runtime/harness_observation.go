package runtime

import (
	"context"
	"log"
)

func (h *Harness) handleObserve(state *harnessState, ctx context.Context, after Cursor) (Observation, error) {
	if after > state.cursor {
		return Observation{}, ErrInvalidCursor
	}
	if state.cursorExpired(after) {
		return Observation{}, ErrCursorExpired
	}
	return h.newObservation(state, ctx, after)
}

func (h *Harness) newObservation(state *harnessState, ctx context.Context, after Cursor) (Observation, error) {
	if after > state.cursor {
		return Observation{}, ErrInvalidCursor
	}
	replay := make([]Event, 0)
	for _, event := range state.events {
		if event.Cursor > after {
			replay = append(replay, event)
		}
	}
	state.nextSubscriber++
	sub := &subscriber{
		id:     state.nextSubscriber,
		events: make(chan Event, len(replay)+h.observationBuffer),
		errors: make(chan error, 1),
	}
	for _, event := range replay {
		sub.events <- event
	}
	state.subscribers[sub.id] = sub
	go func(id uint64) {
		defer func() {
			if recovered := recover(); recovered != nil {
				log.Printf("runtime: binding=%+v subscriber=%d watcher panic: %v", h.binding, id, recovered)
			}
		}()
		select {
		case <-ctx.Done():
			select {
			case h.requests <- unsubscribeRequest{id: id}:
			case <-h.done:
			}
		case <-h.done:
		}
	}(sub.id)
	return Observation{Snapshot: state.snapshot(), Events: sub.events, Errors: sub.errors}, nil
}
