package app

import (
	"testing"
	"time"

	"denova/internal/agent"
)

func TestTaskSubscribeScalesChannelBufferWithReplaySnapshot(t *testing.T) {
	task := &Task{
		status: TaskRunning,
		events: make([]agent.Event, 3000),
	}

	snapshot, ch := task.Subscribe()
	if len(snapshot) != 3000 {
		t.Fatalf("expected replay snapshot length 3000, got %d", len(snapshot))
	}
	if got, wantMin := cap(ch), len(snapshot)+taskSubscribeReplaySlack; got < wantMin {
		t.Fatalf("subscriber channel capacity too small: got %d want >= %d", got, wantMin)
	}

	task.Unsubscribe(ch)
}

func TestTaskSubscribeUsesDefaultBufferForSmallReplay(t *testing.T) {
	task := &Task{
		status: TaskRunning,
		events: make([]agent.Event, 8),
	}

	_, ch := task.Subscribe()
	if got := cap(ch); got != taskSubscriberBuffer {
		t.Fatalf("unexpected subscriber channel capacity for small replay: got %d want %d", got, taskSubscriberBuffer)
	}

	task.Unsubscribe(ch)
}

func TestTaskBatchPreservesCrossTypeEmissionOrder(t *testing.T) {
	task := &Task{status: TaskRunning}
	_, ch := task.Subscribe()

	task.emit(agent.Event{Type: "thinking", Data: map[string]any{"content": "先想"}})
	task.emit(agent.Event{Type: "chunk", Data: map[string]any{"content": "再写"}})
	task.emit(agent.Event{Type: "tool_args_delta", Data: map[string]any{"delta": "{}"}})
	task.flushBatch()

	select {
	case merged := <-ch:
		if merged.Type != "batch" {
			t.Fatalf("expected merged batch event, got %q", merged.Type)
		}
		data, ok := merged.Data.(map[string]interface{})
		if !ok {
			t.Fatalf("unexpected merged data type: %#v", merged.Data)
		}
		events, ok := data["events"].([]agent.Event)
		if !ok {
			t.Fatalf("expected typed agent events payload, got %#v", data["events"])
		}
		if len(events) != 3 {
			t.Fatalf("expected 3 events in batch, got %d", len(events))
		}
		if events[0].Type != "thinking" || events[1].Type != "chunk" || events[2].Type != "tool_args_delta" {
			t.Fatalf("unexpected event order: %#v", []string{events[0].Type, events[1].Type, events[2].Type})
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for merged batch")
	}

	task.Unsubscribe(ch)
}
