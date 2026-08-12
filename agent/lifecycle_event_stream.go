package agent

// eventDropState coalesces bounded observer loss without coupling a slow
// display consumer to authoritative Agent execution.
type eventDropState struct {
	count int
	after Cursor
}

func publishLatestEvent(output chan Event, event Event, drops *eventDropState) {
	if output == nil || cap(output) == 0 {
		return
	}
	if drops == nil {
		drops = &eventDropState{}
	}
	needed := 1
	for {
		if drops.count > 0 && cap(output) >= 2 {
			needed = 2
		}
		if len(output)+needed <= cap(output) {
			break
		}
		select {
		case evicted := <-output:
			recordDroppedEvent(drops, evicted)
		default:
			// The consumer freed capacity between len and receive.
		}
	}
	if drops.count > 0 && cap(output) >= 2 {
		output <- Event{
			Cursor: event.Cursor, Durability: EphemeralEvent, RunID: event.RunID,
			Payload: EventStreamGap{Dropped: drops.count, ResumeAfter: drops.after},
		}
		drops.count, drops.after = 0, 0
	}
	output <- event
}

func recordDroppedEvent(drops *eventDropState, event Event) {
	if gap, ok := event.Payload.(EventStreamGap); ok {
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
