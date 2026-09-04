package interactive

import (
	"crypto/sha256"
	"encoding/hex"
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

// modelContextProviderContinuationEvent stores provider-owned state for one
// assistant tool-call message. Keeping it outside Turn and batch payloads
// prevents opaque signed or encrypted data from entering public Story JSON.
type modelContextProviderContinuationEvent struct {
	V                    int            `json:"v"`
	Type                 string         `json:"type"`
	ID                   string         `json:"id"`
	ParentID             string         `json:"parent_id"`
	BranchID             string         `json:"branch_id"`
	Ts                   string         `json:"ts"`
	OwnerID              string         `json:"owner_id"`
	MessageIndex         int            `json:"message_index"`
	ProviderContinuation map[string]any `json:"provider_continuation"`
}

func newProviderContinuationEvent(turn TurnEvent, continuation map[string]any) providerContinuationEvent {
	return providerContinuationEvent{
		V: schemaVersion, Type: StoryEventTypeProviderContinuation, ID: newID("tpc"),
		ParentID: turn.ID, BranchID: turn.BranchID, Ts: turn.Ts, TurnID: turn.ID,
		ProviderContinuation: cloneProviderContinuation(continuation),
	}
}

func newModelContextProviderContinuationEvents(
	ownerID, branchID, timestamp string,
	messages []ModelContextMessage,
) ([]any, error) {
	result := make([]any, 0)
	for index, message := range messages {
		continuation, err := normalizeProviderContinuation(message.ProviderContinuation)
		if err != nil {
			return nil, err
		}
		if len(continuation) == 0 {
			continue
		}
		result = append(result, modelContextProviderContinuationEvent{
			V: schemaVersion, Type: StoryEventTypeModelContextProviderContinuation,
			ID: deterministicModelContextProviderContinuationID(ownerID, index), ParentID: ownerID,
			BranchID: strings.TrimSpace(branchID), Ts: strings.TrimSpace(timestamp), OwnerID: ownerID,
			MessageIndex: index, ProviderContinuation: continuation,
		})
	}
	return result, nil
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

func normalizeModelContextProviderContinuationEvent(
	event modelContextProviderContinuationEvent,
) (modelContextProviderContinuationEvent, error) {
	event.OwnerID = strings.TrimSpace(event.OwnerID)
	event.ParentID = strings.TrimSpace(event.ParentID)
	event.BranchID = strings.TrimSpace(event.BranchID)
	event.Ts = strings.TrimSpace(event.Ts)
	if event.V <= 0 || event.V > schemaVersion || event.Type != StoryEventTypeModelContextProviderContinuation ||
		event.OwnerID == "" || event.ParentID != event.OwnerID || event.BranchID == "" || event.Ts == "" || event.MessageIndex < 0 ||
		strings.TrimSpace(event.ID) != deterministicModelContextProviderContinuationID(event.OwnerID, event.MessageIndex) {
		return modelContextProviderContinuationEvent{}, fmt.Errorf("Game model-context provider continuation event is invalid")
	}
	continuation, err := normalizeProviderContinuation(event.ProviderContinuation)
	if err != nil {
		return modelContextProviderContinuationEvent{}, err
	}
	if len(continuation) == 0 {
		return modelContextProviderContinuationEvent{}, fmt.Errorf("Game model-context provider continuation event is empty")
	}
	event.ProviderContinuation = continuation
	return event, nil
}

func modelContextProviderContinuationsByOwner(
	lines []StoryEventRecord,
) (map[string]map[int]map[string]any, error) {
	result := make(map[string]map[int]map[string]any)
	for _, record := range lines {
		if record.Envelope.Type != StoryEventTypeModelContextProviderContinuation {
			continue
		}
		var event modelContextProviderContinuationEvent
		if err := mapToStruct(record.Raw, &event); err != nil {
			return nil, fmt.Errorf("decode Game model-context provider continuation event: %w", err)
		}
		event, err := normalizeModelContextProviderContinuationEvent(event)
		if err != nil {
			return nil, err
		}
		byIndex := result[event.OwnerID]
		if byIndex == nil {
			byIndex = make(map[int]map[string]any)
			result[event.OwnerID] = byIndex
		}
		if _, duplicate := byIndex[event.MessageIndex]; duplicate {
			return nil, fmt.Errorf("Game model-context owner %s has duplicate provider continuation for message %d", event.OwnerID, event.MessageIndex)
		}
		byIndex[event.MessageIndex] = event.ProviderContinuation
	}
	return result, nil
}

func hydrateModelContextProviderContinuations(
	messages []ModelContextMessage,
	ownerID string,
	continuations map[string]map[int]map[string]any,
) ([]ModelContextMessage, error) {
	result := sanitizeModelContextMessages(messages)
	for index, continuation := range continuations[strings.TrimSpace(ownerID)] {
		if index < 0 || index >= len(result) || result[index].Role != "assistant" {
			return nil, fmt.Errorf("Game model-context owner %s has provider continuation for invalid message %d", ownerID, index)
		}
		result[index].ProviderContinuation = cloneProviderContinuation(continuation)
	}
	return result, nil
}

func deterministicModelContextProviderContinuationID(ownerID string, messageIndex int) string {
	digest := sha256.Sum256([]byte(strings.TrimSpace(ownerID) + "\x00" + fmt.Sprint(messageIndex)))
	return "model-provider-continuation-" + hex.EncodeToString(digest[:16])
}
