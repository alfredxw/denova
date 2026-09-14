package app

import (
	"context"
	"fmt"

	agentrun "denova/internal/agents/run"
	apptask "denova/internal/app/task"
	"denova/internal/platform"
)

// Stream binds the subscription to the observed operation and workspace lease.
// Only root prose crosses this boundary; tools and private reasoning stay local.
func (h platformStoryHost) Stream(ctx context.Context, scope platform.Scope, operationID string, emit func(platform.StoryStreamEvent) error) error {
	operation, err := h.acquire(ctx, scope.ProjectID)
	if err != nil {
		return err
	}
	defer operation.Release()
	ctx = operation.Context()
	view := h.app.InteractiveAgentActiveView(ctx, scope.StoryID, "")
	if operationID == "" || string(view.Runtime.ActiveOperation) != operationID {
		return platformStoryError("STORY_CONFLICT", "The observed Story operation has changed; reload its snapshot")
	}
	task, info := h.app.ActiveInteractiveTaskFor(scope.StoryID, view.Runtime.Binding.BranchID)
	if task == nil || view.Task == nil || info.TaskID != view.Task.ID {
		return platformStoryError("STORY_CONFLICT", "Story stream is no longer available; reload its snapshot")
	}
	return streamStoryTask(ctx, task, emit)
}

func streamStoryTask(ctx context.Context, task *apptask.Task, emit func(platform.StoryStreamEvent) error) error {
	replay, sub, err := task.SubscribeDisplayAfter(0)
	if err != nil {
		return err
	}
	defer task.Unsubscribe(sub)
	if err := emit(platform.StoryStreamEvent{Kind: "reset"}); err != nil {
		return err
	}
	forward := func(event agentrun.Event) error {
		if event.DataString("subagent") == "true" {
			return nil
		}
		switch event.Type {
		case "agent_cycle_started", "interactive_content_reclassified":
			return emit(platform.StoryStreamEvent{Kind: "reset"})
		case "chunk":
			return emit(platform.StoryStreamEvent{Kind: "delta", Text: event.DataString("content")})
		}
		return nil
	}
	if replay.Checkpoint != nil {
		if !replay.Checkpoint.Complete {
			return platformStoryError("STORY_CONFLICT", "Story display history expired; reload its canonical snapshot")
		}
		for _, event := range replay.Checkpoint.Events {
			if err := forward(event); err != nil {
				return err
			}
		}
	}
	for _, item := range replay.Events {
		if err := forward(item.Event); err != nil {
			return err
		}
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case item, ok := <-sub.Events():
			if !ok {
				if sub.EndReason() != apptask.SubscriptionTaskFinished {
					return fmt.Errorf("Story display subscription ended: %s", sub.EndReason())
				}
				return emit(platform.StoryStreamEvent{Kind: "settled"})
			}
			if err := forward(item.Event); err != nil {
				return err
			}
		}
	}
}
