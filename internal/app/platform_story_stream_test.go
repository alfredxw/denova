package app

import (
	"context"
	"reflect"
	"testing"

	agentrun "denova/internal/agents/run"
	apptask "denova/internal/app/task"
	"denova/internal/platform"
)

func TestPlatformStoryStreamReplaysOnlyRootProseAndRetractions(t *testing.T) {
	task, err := apptask.NewDeferred(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer task.Finish()
	for _, event := range []agentrun.Event{
		{Type: "agent_cycle_started"},
		{Type: "thinking", Data: map[string]string{"content": "private reasoning"}},
		{Type: "chunk", Data: map[string]any{"subagent": true, "content": "private child"}},
		{Type: "tool_args_delta", Data: map[string]string{"delta": "private arguments"}},
		{Type: "chunk", Data: map[string]string{"content": "provisional"}},
		{Type: "interactive_content_reclassified"},
		{Type: "chunk", Data: map[string]string{"content": "[lin|smile] Welcome"}},
	} {
		task.Emit(event)
	}
	var got []platform.StoryStreamEvent
	err = streamStoryTask(context.Background(), task, func(event platform.StoryStreamEvent) error {
		got = append(got, event)
		if event.Text == "[lin|smile] Welcome" {
			// A live delta must arrive before settlement, not after a full turn.
			if task.Finished() {
				t.Fatal("stream waited for completion")
			}
			task.Finish()
		}
		return nil
	})
	want := []platform.StoryStreamEvent{{Kind: "reset"}, {Kind: "reset"}, {Kind: "delta", Text: "provisional"}, {Kind: "reset"}, {Kind: "delta", Text: "[lin|smile] Welcome"}, {Kind: "settled"}}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("stream = %#v, %v", got, err)
	}
}

func TestPlatformStoryStreamDisconnectDoesNotCancelGeneration(t *testing.T) {
	task, err := apptask.NewDeferred(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer task.Finish()
	ctx, cancel := context.WithCancel(context.Background())
	err = streamStoryTask(ctx, task, func(platform.StoryStreamEvent) error { cancel(); return nil })
	if err != context.Canceled || task.Finished() || task.Snapshot().CancelRequested {
		t.Fatalf("disconnect changed task: %v %+v", err, task.Snapshot())
	}
}
