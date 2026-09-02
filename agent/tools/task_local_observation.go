package tools

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	agent "github.com/alfredxw/denova/agent"
)

func (tasks *LocalTasks) taskFromSessionSnapshot(
	ctx context.Context,
	session *agent.Session,
	ref TaskRef,
	snapshot agent.SessionSnapshot,
) (Task, error) {
	task, err := taskFromSnapshot(ref, snapshot)
	if err != nil || task.Output != "" || !isTaskTerminal(task.Status) {
		return task, err
	}
	output, _, replayErr := replayTaskFinal(ctx, session, ref.Run, snapshot.RetentionStart)
	task.Output = output
	return task, replayErr
}

func taskFromSnapshot(ref TaskRef, snapshot agent.SessionSnapshot) (Task, error) {
	status := taskStatus(snapshot, ref.Run)
	switch status {
	case "unknown":
		return Task{}, errors.New("task Run was not found")
	case "running", "waiting_input", "aborting",
		string(agent.ResultCompleted), string(agent.ResultFailed), string(agent.ResultIncomplete),
		string(agent.ResultBlocked), string(agent.ResultAborted):
		return Task{
			Ref: ref, Status: status, Reason: taskSnapshotReason(snapshot, ref.Run),
			Output: taskSnapshotOutput(snapshot, ref.Run),
		}, nil
	default:
		return Task{}, fmt.Errorf("unsupported task status %q", status)
	}
}

func taskSnapshotReason(snapshot agent.SessionSnapshot, runID string) string {
	for _, recent := range snapshot.RecentRuns {
		if recent.ID == runID {
			return recent.Reason
		}
	}
	return ""
}

func isTaskTerminal(status string) bool {
	switch status {
	case string(agent.ResultCompleted), string(agent.ResultFailed), string(agent.ResultIncomplete),
		string(agent.ResultBlocked), string(agent.ResultAborted):
		return true
	case "running", "waiting_input", "aborting":
		return false
	default:
		return false
	}
}

func collectTaskEvents(ctx context.Context, observation agent.Observation, runID string, after agent.Cursor) ([]TaskEvent, string, string, bool, error) {
	var events []TaskEvent
	var output string
	target := observation.Snapshot.Cursor
	cursor := after
	if cursor >= target {
		return nil, strconv.FormatUint(uint64(target), 10), "", observation.Snapshot.MessagesTruncated, nil
	}
	for {
		select {
		case event, ok := <-observation.Events:
			if !ok {
				return events, strconv.FormatUint(uint64(cursor), 10), output, true, nil
			}
			cursor = event.Cursor
			if event.RunID != runID {
				if cursor >= target {
					return events, strconv.FormatUint(uint64(target), 10), output, observation.Snapshot.MessagesTruncated, nil
				}
				continue
			}
			projected := TaskEvent{
				Cursor: strconv.FormatUint(uint64(event.Cursor), 10), Type: taskEventType(event.Payload),
				Run: event.RunID, Event: event,
			}
			switch payload := event.Payload.(type) {
			case agent.AssistantDelta:
				projected.Type, projected.Text = "assistant_delta", payload.Delta
			case agent.ThinkingDelta:
				projected.Type, projected.Text = "thinking_delta", payload.Delta
			case agent.AssistantFinal:
				projected.Type, projected.Text, output = "assistant_final", payload.Content, payload.Content
			case agent.ToolStarted:
				projected.Type, projected.Tool = "tool_started", payload.Name
			case agent.ToolProgress:
				projected.Type, projected.Tool, projected.Text = "tool_progress", payload.Name, payload.Delta
			case agent.ToolFinished:
				projected.Type, projected.Tool, projected.Text = "tool_finished", payload.Name, payload.Result
			case agent.InteractionRequested:
				projected.Type = "interaction_requested"
			case agent.RunSettled:
				projected.Type, projected.Text = "run_settled", string(payload.Status)
			}
			events = append(events, projected)
			if cursor >= target {
				return events, strconv.FormatUint(uint64(target), 10), output, observation.Snapshot.MessagesTruncated, nil
			}
		case err, ok := <-observation.Errors:
			if ok && err != nil {
				return events, strconv.FormatUint(uint64(cursor), 10), output, true, err
			}
			observation.Errors = nil
		case <-ctx.Done():
			return events, strconv.FormatUint(uint64(cursor), 10), output, true, ctx.Err()
		}
	}
}

func taskEventType(payload agent.EventPayload) string {
	switch payload.(type) {
	case agent.RunAccepted:
		return "run_accepted"
	case agent.RunStarted:
		return "run_started"
	case agent.AssistantDelta:
		return "assistant_delta"
	case agent.ThinkingDelta:
		return "thinking_delta"
	case agent.ModelCompleted:
		return "model_completed"
	case agent.ContextNormalized:
		return "context_normalized"
	case agent.AssistantFinal:
		return "assistant_final"
	case agent.ToolInputStarted:
		return "tool_input_started"
	case agent.ToolInputDelta:
		return "tool_input_delta"
	case agent.ToolStarted:
		return "tool_started"
	case agent.ToolProgress:
		return "tool_progress"
	case agent.ToolFinished:
		return "tool_finished"
	case agent.ArtifactProduced:
		return "artifact_produced"
	case agent.EventStreamGap:
		return "event_stream_gap"
	case agent.GoalUpdated:
		return "goal_updated"
	case agent.GoalEvaluationFailed:
		return "goal_evaluation_failed"
	case agent.TodoUpdated:
		return "todo_updated"
	case agent.InteractionRequested:
		return "interaction_requested"
	case agent.InteractionResolved:
		return "interaction_resolved"
	case agent.CompactionStarted:
		return "compaction_started"
	case agent.CompactionCommitted:
		return "compaction_committed"
	case agent.CompactionRemoved:
		return "compaction_removed"
	case agent.CompactionFailed:
		return "compaction_failed"
	case agent.CompactionSkipped:
		return "compaction_skipped"
	case agent.CleanupStarted:
		return "cleanup_started"
	case agent.CleanupCompleted:
		return "cleanup_completed"
	case agent.CleanupFailed:
		return "cleanup_failed"
	case agent.CleanupSkipped:
		return "cleanup_skipped"
	case agent.CleanupCommitted:
		return "cleanup_committed"
	case agent.SessionCleared:
		return "session_cleared"
	case agent.ContextLimitReached:
		return "context_limit_reached"
	case agent.RunSettled:
		return "run_settled"
	case agent.NestedEvent:
		return "nested"
	default:
		return "unknown"
	}
}

func taskStatus(snapshot agent.SessionSnapshot, runID string) string {
	if snapshot.ActiveRunID == runID {
		if snapshot.ActiveAbortPending {
			return "aborting"
		}
		if len(snapshot.PendingInteractions) != 0 {
			return "waiting_input"
		}
		return "running"
	}
	for _, recent := range snapshot.RecentRuns {
		if recent.ID == runID {
			return string(recent.Status)
		}
	}
	return "unknown"
}

func taskSnapshotOutput(snapshot agent.SessionSnapshot, runID string) string {
	if snapshot.ActiveRunID == runID {
		return snapshot.ActiveOutput.Content
	}
	for _, recent := range snapshot.RecentRuns {
		if recent.ID == runID {
			return recent.Output
		}
	}
	return ""
}

func parseTaskCursor(value string) (agent.Cursor, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, nil
	}
	parsed, err := strconv.ParseUint(value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid task cursor: %w", err)
	}
	return agent.Cursor(parsed), nil
}

func replayTaskFinal(
	ctx context.Context,
	session *agent.Session,
	runID string,
	retentionStart agent.Cursor,
) (string, bool, error) {
	after := agent.Cursor(0)
	if retentionStart > 0 {
		after = retentionStart - 1
	}
	observation, err := session.Observe(ctx, after)
	if err != nil {
		return "", true, err
	}
	_, _, output, incomplete, err := collectTaskEvents(ctx, observation, runID, after)
	return output, incomplete || output == "", err
}
