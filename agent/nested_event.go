package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// NestedEvent is a child Session event carried through its parent's live event
// stream. The parent lifecycle owns the outer Event cursor and RunID; Child
// retains the original child cursor, RunID, and typed payload. SessionID plus
// Source make detached task identity explicit without
// exposing the child Session's host-only routing attributes.
type NestedEvent struct {
	Source       EventSource
	ParentCallID string
	SessionID    string
	Child        Event
}

func (NestedEvent) eventPayload() {}

type nestedEventForwarder func(NestedEvent) error
type nestedEventForwarderContextKey struct{}

func contextWithNestedEventForwarder(ctx context.Context, forward nestedEventForwarder) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, nestedEventForwarderContextKey{}, forward)
}

// ForwardNestedEvent forwards a child lifecycle event through the currently
// executing tool. It deliberately accepts no outer Event cursor or RunID: the
// parent Session assigns those identities when the event crosses its Run.
func ForwardNestedEvent(ctx context.Context, event NestedEvent) error {
	if ctx == nil {
		return errors.New("nested Agent event has no execution context")
	}
	forward, ok := ctx.Value(nestedEventForwarderContextKey{}).(nestedEventForwarder)
	if !ok || forward == nil {
		return nil
	}
	event.SessionID = strings.TrimSpace(event.SessionID)
	if event.SessionID == "" || strings.TrimSpace(event.Child.RunID) == "" || event.Child.Payload == nil {
		return errors.New("nested Agent event requires child session, run, and payload")
	}
	return forward(cloneNestedEvent(event))
}

type nestedEventRecord struct {
	Source       EventSource     `json:"source"`
	ParentCallID string          `json:"parent_call_id,omitempty"`
	SessionID    string          `json:"session_id"`
	ChildCursor  Cursor          `json:"child_cursor"`
	ChildRunID   string          `json:"child_run_id"`
	PayloadType  string          `json:"payload_type"`
	Payload      json.RawMessage `json:"payload"`
}

func encodeNestedEvent(event NestedEvent) (nestedEventRecord, error) {
	payloadType, payload, err := encodeEventPayload(event.Child.Payload)
	if err != nil {
		return nestedEventRecord{}, err
	}
	return nestedEventRecord{
		Source: event.Source, ParentCallID: strings.TrimSpace(event.ParentCallID), SessionID: strings.TrimSpace(event.SessionID),
		ChildCursor: event.Child.Cursor, ChildRunID: strings.TrimSpace(event.Child.RunID),
		PayloadType: payloadType, Payload: payload,
	}, nil
}

func decodeNestedEvent(record nestedEventRecord) (NestedEvent, error) {
	payload, err := decodeEventPayload(record.PayloadType, record.Payload)
	if err != nil {
		return NestedEvent{}, err
	}
	event := NestedEvent{
		Source: record.Source, ParentCallID: strings.TrimSpace(record.ParentCallID), SessionID: strings.TrimSpace(record.SessionID),
		Child: Event{Cursor: record.ChildCursor, RunID: strings.TrimSpace(record.ChildRunID), Payload: payload},
	}
	if event.SessionID == "" || event.Child.RunID == "" {
		return NestedEvent{}, errors.New("nested Agent event identity is incomplete")
	}
	return event, nil
}

func encodeEventPayload(payload EventPayload) (string, json.RawMessage, error) {
	if payload == nil {
		return "", nil, errors.New("nested Agent event payload is nil")
	}
	if nested, ok := payload.(NestedEvent); ok {
		record, err := encodeNestedEvent(nested)
		if err != nil {
			return "", nil, err
		}
		encoded, err := json.Marshal(record)
		return "nested", encoded, err
	}
	var kind string
	switch payload.(type) {
	case RunAccepted:
		kind = "run_accepted"
	case RunStarted:
		kind = "run_started"
	case AssistantDelta:
		kind = "assistant_delta"
	case ThinkingDelta:
		kind = "thinking_delta"
	case ModelCompleted:
		kind = "model_completed"
	case ContextNormalized:
		kind = "context_normalized"
	case AssistantFinal:
		kind = "assistant_final"
	case ToolInputStarted:
		kind = "tool_input_started"
	case ToolInputDelta:
		kind = "tool_input_delta"
	case ToolStarted:
		kind = "tool_started"
	case ToolProgress:
		kind = "tool_progress"
	case ToolFinished:
		kind = "tool_finished"
	case ArtifactProduced:
		kind = "artifact_produced"
	case EventStreamGap:
		kind = "event_stream_gap"
	case GoalUpdated:
		kind = "goal_updated"
	case GoalEvaluationFailed:
		kind = "goal_evaluation_failed"
	case TodoUpdated:
		kind = "todo_updated"
	case InteractionRequested:
		kind = "interaction_requested"
	case InteractionResolved:
		kind = "interaction_resolved"
	case CompactionStarted:
		kind = "compaction_started"
	case CompactionCommitted:
		kind = "compaction_committed"
	case CompactionRemoved:
		kind = "compaction_removed"
	case CompactionFailed:
		kind = "compaction_failed"
	case CompactionSkipped:
		kind = "compaction_skipped"
	case CleanupStarted:
		kind = "cleanup_started"
	case CleanupCompleted:
		kind = "cleanup_completed"
	case CleanupFailed:
		kind = "cleanup_failed"
	case CleanupSkipped:
		kind = "cleanup_skipped"
	case CleanupCommitted:
		kind = "cleanup_committed"
	case SessionCleared:
		kind = "session_cleared"
	case TranscriptSynchronized:
		kind = "transcript_synchronized"
	case ContextLimitReached:
		kind = "context_limit_reached"
	case RunSettled:
		kind = "run_settled"
	default:
		return "", nil, fmt.Errorf("unsupported nested Agent event payload %T", payload)
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", nil, fmt.Errorf("encode nested Agent event %s: %w", kind, err)
	}
	return kind, encoded, nil
}

func decodeEventPayload(kind string, data json.RawMessage) (EventPayload, error) {
	var target EventPayload
	switch strings.TrimSpace(kind) {
	case "nested":
		var record nestedEventRecord
		if err := json.Unmarshal(data, &record); err != nil {
			return nil, fmt.Errorf("decode nested Agent event: %w", err)
		}
		return decodeNestedEvent(record)
	case "run_accepted":
		target = &RunAccepted{}
	case "run_started":
		target = &RunStarted{}
	case "assistant_delta":
		target = &AssistantDelta{}
	case "thinking_delta":
		target = &ThinkingDelta{}
	case "model_completed":
		target = &ModelCompleted{}
	case "context_normalized":
		target = &ContextNormalized{}
	case "assistant_final":
		target = &AssistantFinal{}
	case "tool_input_started":
		target = &ToolInputStarted{}
	case "tool_input_delta":
		target = &ToolInputDelta{}
	case "tool_started":
		target = &ToolStarted{}
	case "tool_progress":
		target = &ToolProgress{}
	case "tool_finished":
		target = &ToolFinished{}
	case "artifact_produced":
		target = &ArtifactProduced{}
	case "event_stream_gap":
		target = &EventStreamGap{}
	case "goal_updated":
		target = &GoalUpdated{}
	case "goal_evaluation_failed":
		target = &GoalEvaluationFailed{}
	case "todo_updated":
		target = &TodoUpdated{}
	case "interaction_requested":
		target = &InteractionRequested{}
	case "interaction_resolved":
		target = &InteractionResolved{}
	case "compaction_started":
		target = &CompactionStarted{}
	case "compaction_committed":
		target = &CompactionCommitted{}
	case "compaction_removed":
		target = &CompactionRemoved{}
	case "compaction_failed":
		target = &CompactionFailed{}
	case "compaction_skipped":
		target = &CompactionSkipped{}
	case "cleanup_started":
		target = &CleanupStarted{}
	case "cleanup_completed":
		target = &CleanupCompleted{}
	case "cleanup_failed":
		target = &CleanupFailed{}
	case "cleanup_skipped":
		target = &CleanupSkipped{}
	case "cleanup_committed":
		target = &CleanupCommitted{}
	case "session_cleared":
		target = &SessionCleared{}
	case "transcript_synchronized":
		target = &TranscriptSynchronized{}
	case "context_limit_reached":
		target = &ContextLimitReached{}
	case "run_settled":
		target = &RunSettled{}
	default:
		return nil, fmt.Errorf("unsupported nested Agent event type %q", kind)
	}
	if err := json.Unmarshal(data, target); err != nil {
		return nil, fmt.Errorf("decode nested Agent event %s: %w", kind, err)
	}
	return dereferenceEventPayload(target), nil
}

func dereferenceEventPayload(payload EventPayload) EventPayload {
	switch value := payload.(type) {
	case *RunAccepted:
		return *value
	case *RunStarted:
		return *value
	case *AssistantDelta:
		return *value
	case *ThinkingDelta:
		return *value
	case *ModelCompleted:
		return *value
	case *ContextNormalized:
		return *value
	case *AssistantFinal:
		return *value
	case *ToolInputStarted:
		return *value
	case *ToolInputDelta:
		return *value
	case *ToolStarted:
		return *value
	case *ToolProgress:
		return *value
	case *ToolFinished:
		return *value
	case *ArtifactProduced:
		return *value
	case *EventStreamGap:
		return *value
	case *GoalUpdated:
		return *value
	case *GoalEvaluationFailed:
		return *value
	case *TodoUpdated:
		return *value
	case *InteractionRequested:
		return *value
	case *InteractionResolved:
		return *value
	case *CompactionStarted:
		return *value
	case *CompactionCommitted:
		return *value
	case *CompactionRemoved:
		return *value
	case *CompactionFailed:
		return *value
	case *CompactionSkipped:
		return *value
	case *CleanupStarted:
		return *value
	case *CleanupCompleted:
		return *value
	case *CleanupFailed:
		return *value
	case *CleanupSkipped:
		return *value
	case *CleanupCommitted:
		return *value
	case *SessionCleared:
		return *value
	case *TranscriptSynchronized:
		return *value
	case *ContextLimitReached:
		return *value
	case *RunSettled:
		return *value
	default:
		return payload
	}
}

func cloneNestedEvent(event NestedEvent) NestedEvent {
	event.Source.Path = append([]string(nil), event.Source.Path...)
	if record, err := encodeNestedEvent(event); err == nil {
		if cloned, decodeErr := decodeNestedEvent(record); decodeErr == nil {
			return cloned
		}
	}
	return event
}
