package transform

import (
	agentrun "denova/internal/agents/run"
	"testing"
)

type sseEventCollector struct {
	events []agentrun.Event
}

func (c *sseEventCollector) Handle(ev agentrun.Event) error {
	c.events = append(c.events, ev)
	return nil
}

func mustForwardSSEEvent(t *testing.T, collector *sseEventCollector, handler SSEEventHandler, ev agentrun.Event) agentrun.Event {
	t.Helper()
	events := mustForwardSSEEvents(t, collector, handler, ev, 1)
	return events[0]
}

func mustForwardSSEEvents(t *testing.T, collector *sseEventCollector, handler SSEEventHandler, ev agentrun.Event, want int) []agentrun.Event {
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

func mustSuppressSSEEvent(t *testing.T, collector *sseEventCollector, handler SSEEventHandler, ev agentrun.Event) {
	t.Helper()
	before := len(collector.events)
	if err := handler(ev); err != nil {
		t.Fatalf("handler failed: %v", err)
	}
	if len(collector.events) != before {
		t.Fatalf("event should be suppressed: %#v", collector.events[len(collector.events)-1])
	}
}
