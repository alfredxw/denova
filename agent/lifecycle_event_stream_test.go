package agent

import "testing"

func TestPublishLatestEventReportsBoundedLoss(t *testing.T) {
	events := make(chan Event, 2)
	var drops eventDropState
	publishLatestEvent(events, Event{Cursor: 1, Payload: AssistantDelta{Delta: "one"}}, &drops)
	publishLatestEvent(events, Event{Cursor: 2, Payload: AssistantDelta{Delta: "two"}}, &drops)
	publishLatestEvent(events, Event{Cursor: 3, Payload: AssistantDelta{Delta: "three"}}, &drops)

	gapEvent := <-events
	gap, ok := gapEvent.Payload.(EventStreamGap)
	if !ok || gap.Dropped != 2 || gap.ResumeAfter != 2 {
		t.Fatalf("gap = %#v", gapEvent)
	}
	latest := <-events
	if latest.Cursor != 3 {
		t.Fatalf("latest event = %#v", latest)
	}
}
