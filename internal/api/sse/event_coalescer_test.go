package sse

import (
	"testing"

	novaApp "denova/internal/app"
	apptask "denova/internal/app/task"
)

func TestCoalesceTaskEventsPreservesExactTextAndLastCursor(t *testing.T) {
	events := []apptask.Event{
		{Cursor: 1, Event: novaApp.AgentEvent{Type: "thinking", Data: map[string]any{"content": "逐", "run_id": "run-1"}}},
		{Cursor: 2, Event: novaApp.AgentEvent{Type: "thinking", Data: map[string]any{"content": "字", "run_id": "run-1"}}},
		{Cursor: 3, Event: novaApp.AgentEvent{Type: "chunk", Data: map[string]any{"content": "正", "run_id": "run-1"}}},
		{Cursor: 4, Event: novaApp.AgentEvent{Type: "chunk", Data: map[string]any{"content": "文", "run_id": "run-1"}}},
	}

	coalesced := coalesceTaskEvents(events)
	if len(coalesced) != 2 {
		t.Fatalf("coalesced events = %#v, want two semantic segments", coalesced)
	}
	if coalesced[0].Cursor != 2 || coalesced[0].Event.DataString("content") != "逐字" {
		t.Fatalf("thinking event = %#v", coalesced[0])
	}
	if coalesced[1].Cursor != 4 || coalesced[1].Event.DataString("content") != "正文" {
		t.Fatalf("narrative event = %#v", coalesced[1])
	}
}

func TestCoalesceTaskEventsKeepsDifferentSourcesSeparate(t *testing.T) {
	events := []apptask.Event{
		{Cursor: 1, Event: novaApp.AgentEvent{Type: "thinking", Data: map[string]any{"content": "root", "run_id": "run-1", "agent_name": "root"}}},
		{Cursor: 2, Event: novaApp.AgentEvent{Type: "thinking", Data: map[string]any{"content": "sub", "run_id": "run-1", "agent_name": "sub"}}},
	}

	if coalesced := coalesceTaskEvents(events); len(coalesced) != 2 {
		t.Fatalf("different display sources were merged: %#v", coalesced)
	}
}

func TestWriteCoalescedTaskEventStreamFlushesBeforeSemanticBoundary(t *testing.T) {
	events := make(chan apptask.Event, 4)
	events <- apptask.Event{Cursor: 1, Event: novaApp.AgentEvent{Type: "thinking", Data: map[string]any{"content": "保", "run_id": "run-1"}}}
	events <- apptask.Event{Cursor: 2, Event: novaApp.AgentEvent{Type: "thinking", Data: map[string]any{"content": "留", "run_id": "run-1"}}}
	events <- apptask.Event{Cursor: 3, Event: novaApp.AgentEvent{Type: "done", Data: map[string]any{"status": "completed"}}}
	close(events)

	written := make([]apptask.Event, 0, 2)
	last, err := writeCoalescedTaskEventStream(events, func(event apptask.Event) error {
		written = append(written, event)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if last.Cursor != 0 {
		t.Fatalf("terminal stream result = %#v, want zero event", last)
	}
	if len(written) != 2 || written[0].Cursor != 2 || written[0].Event.DataString("content") != "保留" || written[1].Event.Type != "done" {
		t.Fatalf("written stream = %#v, want exact delta before done", written)
	}
}
