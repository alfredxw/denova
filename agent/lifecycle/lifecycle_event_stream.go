package lifecycle

import (
	agentevent "github.com/alfredxw/denova/agent/lifecycle/event"
)

// eventDropState coalesces bounded observer loss without coupling a slow
// display consumer to authoritative Agent execution.
type eventDropState struct {
	count int
	after agentevent.Cursor
}

func publishLatestEvent(output chan agentevent.Event, event agentevent.Event, drops *eventDropState) {
	if output == nil || cap(output) == 0 {
		return
	}
	if drops == nil {
		drops = &eventDropState{}
	}
	needed := 1
	if drops.count > 0 && cap(output) >= 2 {
		needed = 2
	}
	if len(output)+needed > cap(output) {
		queued := make([]agentevent.Event, 0, len(output))
		for {
			select {
			case existing := <-output:
				queued = append(queued, existing)
			default:
				goto drained
			}
		}
	drained:
		if drops.count > 0 && cap(output) >= 2 {
			needed = 2
		}
		for len(queued)+needed > cap(output) && len(queued) > 0 {
			evictIndex := oldestStreamingDeltaIndex(queued)
			if evictIndex < 0 {
				evictIndex = 0
			}
			recordDroppedEvent(drops, queued[evictIndex])
			queued = append(queued[:evictIndex], queued[evictIndex+1:]...)
			if drops.count > 0 && cap(output) >= 2 {
				needed = 2
			}
		}
		for _, item := range queued {
			output <- item
		}
	}
	if drops.count > 0 && cap(output) >= 2 {
		output <- agentevent.Event{
			Cursor: event.Cursor, RunID: event.RunID,
			Payload: agentevent.EventStreamGap{Dropped: drops.count, ResumeAfter: drops.after},
		}
		drops.count, drops.after = 0, 0
	}
	output <- event
}

func oldestStreamingDeltaIndex(events []agentevent.Event) int {
	for index, item := range events {
		if isStreamingDeltaPayload(item.Payload) {
			return index
		}
	}
	return -1
}

func isStreamingDeltaPayload(payload agentevent.EventPayload) bool {
	switch value := payload.(type) {
	case agentevent.AssistantDelta, agentevent.ThinkingDelta, agentevent.ToolInputDelta:
		return true
	case agentevent.NestedEvent:
		return isStreamingDeltaPayload(value.Child.Payload)
	default:
		return false
	}
}

func recordDroppedEvent(drops *eventDropState, event agentevent.Event) {
	if gap, ok := event.Payload.(agentevent.EventStreamGap); ok {
		drops.count += gap.Dropped
		if gap.ResumeAfter > drops.after {
			drops.after = gap.ResumeAfter
		}
		return
	}
	drops.count++
	if event.Cursor > drops.after {
		drops.after = event.Cursor
	}
}
