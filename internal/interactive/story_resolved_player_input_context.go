package interactive

import (
	"fmt"
	"strings"
)

// ResolvedPlayerInputContext is the durable model-history record for an older
// accepted input closed by a later successful Turn. Keeping the canonical
// input event and its complete ordered tool batches together lets projections
// restore the input at its acceptance boundary after reload. It is deliberately
// separate from Turn.ModelContextMessages, which belongs only to that Turn's
// own player input.
type ResolvedPlayerInputContext struct {
	Input               PlayerInputAcceptedEvent `json:"input"`
	ModelContextBatches []ModelContextBatchEvent `json:"model_context_batches,omitempty"`
}

func resolveHistoricalPlayerInputContexts(
	lines []StoryEventRecord,
	branchID string,
	pendingInputs []PlayerInputAcceptedEvent,
	currentPlayerInputID string,
) ([]ResolvedPlayerInputContext, error) {
	currentPlayerInputID = strings.TrimSpace(currentPlayerInputID)
	contexts := make([]ResolvedPlayerInputContext, 0, len(pendingInputs))
	consumedIDs := make([]string, 0, len(pendingInputs))
	for _, input := range pendingInputs {
		consumedIDs = append(consumedIDs, input.ID)
		if strings.TrimSpace(input.ID) == currentPlayerInputID {
			continue
		}
		batches, err := modelContextBatchesForPlayerInput(lines, branchID, input.ID)
		if err != nil {
			return nil, err
		}
		contexts = append(contexts, ResolvedPlayerInputContext{
			Input:               input,
			ModelContextBatches: batches,
		})
	}
	return normalizeResolvedPlayerInputContexts(contexts, branchID, currentPlayerInputID, consumedIDs)
}

func normalizeResolvedPlayerInputContexts(
	contexts []ResolvedPlayerInputContext,
	branchID string,
	currentPlayerInputID string,
	consumedPlayerInputIDs []string,
) ([]ResolvedPlayerInputContext, error) {
	if len(contexts) == 0 {
		return nil, nil
	}
	branchID = strings.TrimSpace(branchID)
	currentPlayerInputID = strings.TrimSpace(currentPlayerInputID)
	consumed := make(map[string]bool, len(consumedPlayerInputIDs))
	for _, playerInputID := range consumedPlayerInputIDs {
		if playerInputID = strings.TrimSpace(playerInputID); playerInputID != "" {
			consumed[playerInputID] = true
		}
	}
	seen := make(map[string]bool, len(contexts))
	result := make([]ResolvedPlayerInputContext, 0, len(contexts))
	for _, context := range contexts {
		input, err := normalizePlayerInputAcceptedEvent(context.Input)
		if err != nil {
			return nil, fmt.Errorf("normalize resolved player input: %w", err)
		}
		if branchID != "" && input.BranchID != branchID {
			return nil, fmt.Errorf("%w: resolved player input %s belongs to branch %s, want %s", ErrPlayerInputIdentityConflict, input.ID, input.BranchID, branchID)
		}
		if input.ID == currentPlayerInputID {
			return nil, fmt.Errorf("%w: current player input %s cannot also be a resolved historical context", ErrPlayerInputIdentityConflict, input.ID)
		}
		if !consumed[input.ID] {
			return nil, fmt.Errorf("%w: resolved player input %s is not listed as consumed", ErrPlayerInputIdentityConflict, input.ID)
		}
		if seen[input.ID] {
			return nil, fmt.Errorf("%w: duplicate resolved player input %s", ErrPlayerInputIdentityConflict, input.ID)
		}
		seen[input.ID] = true

		batches := make([]ModelContextBatchEvent, 0, len(context.ModelContextBatches))
		for index, batch := range context.ModelContextBatches {
			normalized, err := normalizeModelContextBatchEvent(batch)
			if err != nil {
				return nil, err
			}
			if normalized.PlayerInputID != input.ID || normalized.BranchID != input.BranchID ||
				normalized.AgentCommandID != input.AgentCommandID || normalized.AgentOperationID != input.AgentOperationID ||
				normalized.AgentCycle != input.AgentCycle {
				return nil, fmt.Errorf("%w: resolved batch does not match player input %s", ErrModelContextBatchIdentityConflict, input.ID)
			}
			if normalized.BatchOrdinal != index {
				return nil, fmt.Errorf("%w: resolved player input %s has a missing or duplicate batch ordinal", ErrModelContextBatchIdentityConflict, input.ID)
			}
			batches = append(batches, normalized)
		}
		result = append(result, ResolvedPlayerInputContext{Input: input, ModelContextBatches: batches})
	}
	return result, nil
}

func normalizePlayerInputAcceptedEvent(event PlayerInputAcceptedEvent) (PlayerInputAcceptedEvent, error) {
	if event.V <= 0 || event.V > schemaVersion || strings.TrimSpace(event.Type) != StoryEventTypePlayerInput || strings.TrimSpace(event.Ts) == "" {
		return PlayerInputAcceptedEvent{}, fmt.Errorf("%w: persisted player input envelope is invalid", ErrPlayerInputIdentityConflict)
	}
	identity := normalizeDomainCommitIdentity(DomainCommitIdentity{
		CommandID: event.AgentCommandID, OperationID: event.AgentOperationID, Cycle: event.AgentCycle,
	})
	canonical, err := NewPlayerInputIntent(identity, event.BranchID, event.Text)
	if err != nil {
		return PlayerInputAcceptedEvent{}, err
	}
	if strings.TrimSpace(event.ID) != deterministicPlayerInputID(identity) ||
		strings.TrimSpace(event.AgentCommitHash) != canonical.Hash {
		return PlayerInputAcceptedEvent{}, fmt.Errorf("%w: persisted player input identity or hash changed", ErrPlayerInputIdentityConflict)
	}
	if event.AcceptedTurnCount < 0 {
		return PlayerInputAcceptedEvent{}, fmt.Errorf("%w: accepted turn boundary is negative", ErrPlayerInputIdentityConflict)
	}
	return PlayerInputAcceptedEvent{
		V: event.V, Type: StoryEventTypePlayerInput,
		ID: deterministicPlayerInputID(identity), ParentID: strings.TrimSpace(event.ParentID),
		BranchID: canonical.BranchID, Ts: strings.TrimSpace(event.Ts), Text: canonical.Text,
		AcceptedTurnCount: event.AcceptedTurnCount,
		AgentCommandID:    identity.CommandID, AgentOperationID: identity.OperationID,
		AgentCycle: identity.Cycle, AgentCommitHash: canonical.Hash,
	}, nil
}

func cloneResolvedPlayerInputContexts(contexts []ResolvedPlayerInputContext) []ResolvedPlayerInputContext {
	if len(contexts) == 0 {
		return nil
	}
	result := make([]ResolvedPlayerInputContext, 0, len(contexts))
	for _, context := range contexts {
		input := context.Input
		batches := make([]ModelContextBatchEvent, 0, len(context.ModelContextBatches))
		for _, batch := range context.ModelContextBatches {
			batch.Messages = sanitizeModelContextMessages(batch.Messages)
			batches = append(batches, batch)
		}
		result = append(result, ResolvedPlayerInputContext{Input: input, ModelContextBatches: batches})
	}
	return result
}

// CloneResolvedPlayerInputContexts returns an independently mutable copy of a
// canonical resolved-input projection for model-history callers.
func CloneResolvedPlayerInputContexts(contexts []ResolvedPlayerInputContext) []ResolvedPlayerInputContext {
	return cloneResolvedPlayerInputContexts(contexts)
}
