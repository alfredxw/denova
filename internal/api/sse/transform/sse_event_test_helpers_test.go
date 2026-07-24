package transform

import (
	"testing"

	agents "denova/internal/agents"
)

type sseEventCollector struct {
	events []agents.Event
}

func (c *sseEventCollector) Handle(ev agents.Event) error {
	c.events = append(c.events, ev)
	return nil
}

func mustForwardSSEEvent(t *testing.T, collector *sseEventCollector, handler SSEEventHandler, ev agents.Event) agents.Event {
	t.Helper()
	events := mustForwardSSEEvents(t, collector, handler, ev, 1)
	return events[0]
}

func mustForwardSSEEvents(t *testing.T, collector *sseEventCollector, handler SSEEventHandler, ev agents.Event, want int) []agents.Event {
	t.Helper()
	before := len(collector.events)
	if err := handler(ev); err != nil {
		t.Fatalf("handler failed: %v", err)
	}
	if len(collector.events) != before+want {
		t.Fatalf("forwarded events = %d, want %d for %#v", len(collector.events)-before, want, ev)
	}
	return collector.events[before:]
}

func mustSuppressSSEEvent(t *testing.T, collector *sseEventCollector, handler SSEEventHandler, ev agents.Event) {
	t.Helper()
	before := len(collector.events)
	if err := handler(ev); err != nil {
		t.Fatalf("handler failed: %v", err)
	}
	if len(collector.events) != before {
		t.Fatalf("event should be suppressed: %#v", collector.events[len(collector.events)-1])
	}
}
