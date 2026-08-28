package interactive

import (
	"encoding/json"
	"fmt"
	"strings"

	agent "github.com/alfredxw/denova/agent"
	"github.com/alfredxw/denova/agent/providers"
)

// providerContinuationEvent stores the exact provider-owned state associated
// with one completed Turn. It is an atomic side event rather than a Turn field
// so public Story snapshots cannot serialize opaque model context by accident.
type providerContinuationEvent struct {
	V                    int            `json:"v"`
	Type                 string         `json:"type"`
	ID                   string         `json:"id"`
	ParentID             string         `json:"parent_id"`
	BranchID             string         `json:"branch_id"`
	Ts                   string         `json:"ts"`
	TurnID               string         `json:"turn_id"`
	ProviderContinuation map[string]any `json:"provider_continuation"`
}

func newProviderContinuationEvent(turn TurnEvent, continuation map[string]any) providerContinuationEvent {
	return providerContinuationEvent{
		V: schemaVersion, Type: StoryEventTypeProviderContinuation, ID: newID("tpc"),
		ParentID: turn.ID, BranchID: turn.BranchID, Ts: turn.Ts, TurnID: turn.ID,
		ProviderContinuation: cloneProviderContinuation(continuation),
	}
}

func normalizeProviderContinuation(extra map[string]any) (map[string]any, error) {
	selected := providers.ContinuationExtra(extra)
	if len(selected) == 0 {
		return nil, nil
	}
	data, err := json.Marshal(selected)
	if err != nil {
		return nil, fmt.Errorf("encode Game provider continuation: %w", err)
	}
	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("decode Game provider continuation: %w", err)
	}
	return result, nil
}

func cloneProviderContinuation(extra map[string]any) map[string]any {
	message := agent.CloneMessage(&agent.Message{Extra: providers.ContinuationExtra(extra)})
	return message.Extra
}

func normalizeProviderContinuationEvent(event providerContinuationEvent) (providerContinuationEvent, error) {
	event.TurnID = strings.TrimSpace(event.TurnID)
	event.ParentID = strings.TrimSpace(event.ParentID)
	if event.TurnID == "" || event.TurnID != event.ParentID {
		return providerContinuationEvent{}, fmt.Errorf("Game provider continuation event has invalid Turn identity")
	}
	continuation, err := normalizeProviderContinuation(event.ProviderContinuation)
	if err != nil {
		return providerContinuationEvent{}, err
	}
	if len(continuation) == 0 {
		return providerContinuationEvent{}, fmt.Errorf("Game provider continuation event is empty")
	}
	event.ProviderContinuation = continuation
	return event, nil
}

func providerContinuationsByTurn(lines []StoryEventRecord) (map[string]map[string]any, error) {
	result := make(map[string]map[string]any)
	for _, record := range lines {
		if record.Envelope.Type != StoryEventTypeProviderContinuation {
			continue
		}
		var event providerContinuationEvent
		if err := mapToStruct(record.Raw, &event); err != nil {
			return nil, fmt.Errorf("decode Game provider continuation event: %w", err)
		}
		event, err := normalizeProviderContinuationEvent(event)
		if err != nil {
			return nil, err
		}
		if _, duplicate := result[event.TurnID]; duplicate {
			return nil, fmt.Errorf("Game Turn %s has duplicate provider continuation events", event.TurnID)
		}
		result[event.TurnID] = event.ProviderContinuation
	}
	return result, nil
}
