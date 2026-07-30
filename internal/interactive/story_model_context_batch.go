package interactive

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

var ErrModelContextBatchIdentityConflict = errors.New("model context batch identity conflict")

// ModelContextBatchIntent is one complete assistant tool-call batch staged
// after its player input and before the final narrative exists. Ordinal is
// scoped to the durable Agent cycle and makes retries deterministic.
type ModelContextBatchIntent struct {
	Identity      DomainCommitIdentity  `json:"identity"`
	BranchID      string                `json:"branch_id"`
	PlayerInputID string                `json:"player_input_id"`
	Ordinal       int                   `json:"ordinal"`
	Messages      []ModelContextMessage `json:"messages"`
	Hash          string                `json:"hash"`
}

// ModelContextBatchEvent is an append-only side event. It deliberately does
// not advance branch.Head: a tool call is durable model evidence, but it is
// not a completed story Turn.
type ModelContextBatchEvent struct {
	V                int                   `json:"v"`
	Type             string                `json:"type"`
	ID               string                `json:"id"`
	ParentID         string                `json:"parent_id,omitempty"`
	BranchID         string                `json:"branch_id"`
	Ts               string                `json:"ts"`
	PlayerInputID    string                `json:"player_input_id"`
	AgentCommandID   string                `json:"agent_command_id"`
	AgentOperationID string                `json:"agent_operation_id"`
	AgentCycle       int                   `json:"agent_cycle"`
	BatchOrdinal     int                   `json:"batch_ordinal"`
	BatchHash        string                `json:"batch_hash"`
	Messages         []ModelContextMessage `json:"messages"`
}

type ModelContextBatchReceipt struct {
	Identity DomainCommitIdentity   `json:"identity"`
	Hash     string                 `json:"hash"`
	Revision string                 `json:"revision"`
	Event    ModelContextBatchEvent `json:"event"`
}

// NewModelContextBatchIntents splits a model-context sequence into complete
// assistant+tool-result batches and assigns consecutive durable ordinals.
func NewModelContextBatchIntents(
	identity DomainCommitIdentity,
	branchID string,
	startOrdinal int,
	messages []ModelContextMessage,
) ([]ModelContextBatchIntent, error) {
	identity = normalizeDomainCommitIdentity(identity)
	branchID = strings.TrimSpace(branchID)
	if identity.CommandID == "" || identity.OperationID == "" || identity.Cycle <= 0 || branchID == "" || startOrdinal < 0 {
		return nil, fmt.Errorf("%w: command_id, operation_id, positive cycle, branch_id, and non-negative ordinal are required", ErrModelContextBatchIdentityConflict)
	}
	batches, err := splitCanonicalModelContextBatches(messages)
	if err != nil {
		return nil, err
	}
	intents := make([]ModelContextBatchIntent, 0, len(batches))
	for index, batch := range batches {
		ordinal := startOrdinal + index
		intent, err := newCanonicalModelContextBatchIntent(identity, branchID, ordinal, batch)
		if err != nil {
			return nil, err
		}
		intents = append(intents, intent)
	}
	return intents, nil
}

func newCanonicalModelContextBatchIntent(
	identity DomainCommitIdentity,
	branchID string,
	ordinal int,
	messages []ModelContextMessage,
) (ModelContextBatchIntent, error) {
	playerInputID := deterministicPlayerInputID(identity)
	payload, err := json.Marshal(struct {
		BranchID      string                `json:"branch_id"`
		PlayerInputID string                `json:"player_input_id"`
		Ordinal       int                   `json:"ordinal"`
		Messages      []ModelContextMessage `json:"messages"`
	}{BranchID: branchID, PlayerInputID: playerInputID, Ordinal: ordinal, Messages: messages})
	if err != nil {
		return ModelContextBatchIntent{}, err
	}
	sum := sha256.Sum256(payload)
	return ModelContextBatchIntent{
		Identity: identity, BranchID: branchID, PlayerInputID: playerInputID,
		Ordinal: ordinal, Messages: messages, Hash: "sha256:" + hex.EncodeToString(sum[:]),
	}, nil
}

func splitCanonicalModelContextBatches(messages []ModelContextMessage) ([][]ModelContextMessage, error) {
	sanitized := sanitizeModelContextMessages(messages)
	if len(sanitized) != len(messages) || len(sanitized) == 0 {
		return nil, fmt.Errorf("%w: context must contain one or more complete tool batches", ErrModelContextBatchIdentityConflict)
	}
	batches := make([][]ModelContextMessage, 0)
	for offset := 0; offset < len(sanitized); {
		assistant := sanitized[offset]
		if assistant.Role != "assistant" || len(assistant.ToolCalls) == 0 {
			return nil, fmt.Errorf("%w: batch %d must start with an assistant tool call", ErrModelContextBatchIdentityConflict, len(batches))
		}
		callIDs := make(map[string]bool, len(assistant.ToolCalls))
		for _, call := range assistant.ToolCalls {
			callID := strings.TrimSpace(call.ID)
			if callID == "" || callIDs[callID] {
				return nil, fmt.Errorf("%w: batch %d has an empty or duplicate tool call id", ErrModelContextBatchIdentityConflict, len(batches))
			}
			callIDs[callID] = true
			arguments := strings.TrimSpace(call.Function.Arguments)
			if arguments != "" && !json.Valid([]byte(arguments)) {
				return nil, fmt.Errorf("%w: tool call %q arguments are not valid JSON", ErrModelContextBatchIdentityConflict, callID)
			}
		}
		end := offset + 1 + len(assistant.ToolCalls)
		if end > len(sanitized) {
			return nil, fmt.Errorf("%w: batch %d is missing tool results", ErrModelContextBatchIdentityConflict, len(batches))
		}
		for index, call := range assistant.ToolCalls {
			result := sanitized[offset+1+index]
			if result.Role != "tool" || strings.TrimSpace(result.ToolCallID) != strings.TrimSpace(call.ID) {
				return nil, fmt.Errorf("%w: batch %d result %d does not match tool call %q", ErrModelContextBatchIdentityConflict, len(batches), index, call.ID)
			}
		}
		batch := make([]ModelContextMessage, end-offset)
		copy(batch, sanitized[offset:end])
		batches = append(batches, batch)
		offset = end
	}
	return batches, nil
}

// AppendModelContextBatch durably publishes one complete tool batch without
// advancing the story branch. Exact retries return the original receipt.
func (s *Store) AppendModelContextBatch(storyID string, intent ModelContextBatchIntent) (ModelContextBatchReceipt, error) {
	canonicalIntents, err := NewModelContextBatchIntents(intent.Identity, intent.BranchID, intent.Ordinal, intent.Messages)
	if err != nil || len(canonicalIntents) != 1 {
		if err == nil {
			err = fmt.Errorf("%w: one append must contain exactly one batch", ErrModelContextBatchIdentityConflict)
		}
		return ModelContextBatchReceipt{}, err
	}
	canonical := canonicalIntents[0]
	if canonical.PlayerInputID != strings.TrimSpace(intent.PlayerInputID) || canonical.Hash != strings.TrimSpace(intent.Hash) {
		return ModelContextBatchReceipt{}, fmt.Errorf("%w: staged model context batch changed", ErrModelContextBatchIdentityConflict)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	releaseStory, err := s.acquireStoryMutationLeaseLocked(storyID)
	if err != nil {
		return ModelContextBatchReceipt{}, err
	}
	defer releaseStory()
	meta, lines, err := s.readStoryRecentLocked(storyID, canonical.BranchID)
	if err != nil {
		return ModelContextBatchReceipt{}, err
	}
	branch, ok := meta.Branches[canonical.BranchID]
	if !ok {
		return ModelContextBatchReceipt{}, fmt.Errorf("分支不存在: %s", canonical.BranchID)
	}
	if receipt, found, err := findModelContextBatchInLines(lines, canonical); err != nil || found {
		return receipt, err
	}
	playerInput, found, err := modelContextBatchPlayerInput(lines, canonical)
	if err != nil {
		return ModelContextBatchReceipt{}, err
	}
	if !found {
		return ModelContextBatchReceipt{}, fmt.Errorf("%w: canonical player input is missing", ErrPlayerInputIdentityConflict)
	}
	if modelContextPlayerInputConsumed(lines, canonical.PlayerInputID) {
		return ModelContextBatchReceipt{}, fmt.Errorf("%w: player input %s already belongs to a completed turn", ErrModelContextBatchIdentityConflict, canonical.PlayerInputID)
	}
	_, activeAncestry := eventPath(branch.Head, eventsByID(lines))
	if parentID := strings.TrimSpace(playerInput.ParentID); parentID != "" && !activeAncestry[parentID] {
		return ModelContextBatchReceipt{}, fmt.Errorf("%w: player input is no longer on the active branch ancestry", ErrStoryContextRevisionConflict)
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	event := ModelContextBatchEvent{
		V: schemaVersion, Type: StoryEventTypeModelContextBatch,
		ID: deterministicModelContextBatchID(canonical.Identity, canonical.Ordinal), ParentID: playerInput.ParentID,
		BranchID: canonical.BranchID, Ts: now, PlayerInputID: canonical.PlayerInputID,
		AgentCommandID: canonical.Identity.CommandID, AgentOperationID: canonical.Identity.OperationID,
		AgentCycle: canonical.Identity.Cycle, BatchOrdinal: canonical.Ordinal,
		BatchHash: canonical.Hash, Messages: canonical.Messages,
	}
	meta.UpdatedAt = now
	if err := s.appendStoryTransactionLocked(storyID, meta, event); err != nil {
		return ModelContextBatchReceipt{}, err
	}
	s.syncStoryIndexProjectionLocked(storyID, meta, len(lines)+1)
	return modelContextBatchReceipt(canonical.Identity, event), nil
}

func findModelContextBatchInLines(lines []StoryEventRecord, intent ModelContextBatchIntent) (ModelContextBatchReceipt, bool, error) {
	expectedID := deterministicModelContextBatchID(intent.Identity, intent.Ordinal)
	for _, record := range lines {
		if record.Envelope.Type != StoryEventTypeModelContextBatch || record.Envelope.ID != expectedID {
			continue
		}
		var event ModelContextBatchEvent
		if err := mapToStruct(record.Raw, &event); err != nil {
			return ModelContextBatchReceipt{}, false, err
		}
		normalized, err := normalizeModelContextBatchEvent(event)
		if err != nil {
			return ModelContextBatchReceipt{}, false, err
		}
		if normalized.BranchID != intent.BranchID || normalized.PlayerInputID != intent.PlayerInputID || normalized.BatchHash != intent.Hash {
			return ModelContextBatchReceipt{}, false, fmt.Errorf("%w: batch ordinal %d has different content", ErrModelContextBatchIdentityConflict, intent.Ordinal)
		}
		return modelContextBatchReceipt(intent.Identity, normalized), true, nil
	}
	return ModelContextBatchReceipt{}, false, nil
}

func modelContextBatchPlayerInput(lines []StoryEventRecord, intent ModelContextBatchIntent) (PlayerInputAcceptedEvent, bool, error) {
	for _, record := range lines {
		if record.Envelope.Type != StoryEventTypePlayerInput || record.Envelope.ID != intent.PlayerInputID {
			continue
		}
		var event PlayerInputAcceptedEvent
		if err := mapToStruct(record.Raw, &event); err != nil {
			return PlayerInputAcceptedEvent{}, false, err
		}
		if event.AgentCommandID != intent.Identity.CommandID || event.AgentOperationID != intent.Identity.OperationID ||
			event.AgentCycle != intent.Identity.Cycle || event.BranchID != intent.BranchID {
			return PlayerInputAcceptedEvent{}, false, fmt.Errorf("%w: model context batch does not match accepted player input", ErrModelContextBatchIdentityConflict)
		}
		return event, true, nil
	}
	return PlayerInputAcceptedEvent{}, false, nil
}

func modelContextPlayerInputConsumed(lines []StoryEventRecord, playerInputID string) bool {
	for _, record := range lines {
		if record.Envelope.Type != StoryEventTypeTurn {
			continue
		}
		var turn TurnEvent
		if mapToStruct(record.Raw, &turn) == nil {
			if strings.TrimSpace(turn.PlayerInputID) == playerInputID {
				return true
			}
			for _, consumed := range turn.ConsumedPlayerInputIDs {
				if strings.TrimSpace(consumed) == playerInputID {
					return true
				}
			}
		}
	}
	return false
}

func normalizeModelContextBatchEvent(event ModelContextBatchEvent) (ModelContextBatchEvent, error) {
	if event.V <= 0 || event.V > schemaVersion || strings.TrimSpace(event.Type) != StoryEventTypeModelContextBatch || strings.TrimSpace(event.Ts) == "" {
		return ModelContextBatchEvent{}, fmt.Errorf("%w: persisted batch envelope is invalid", ErrModelContextBatchIdentityConflict)
	}
	identity := normalizeDomainCommitIdentity(DomainCommitIdentity{
		CommandID: event.AgentCommandID, OperationID: event.AgentOperationID, Cycle: event.AgentCycle,
	})
	intents, err := NewModelContextBatchIntents(identity, event.BranchID, event.BatchOrdinal, event.Messages)
	if err != nil || len(intents) != 1 {
		if err == nil {
			err = fmt.Errorf("%w: persisted event contains more than one batch", ErrModelContextBatchIdentityConflict)
		}
		return ModelContextBatchEvent{}, err
	}
	intent := intents[0]
	if strings.TrimSpace(event.ID) != deterministicModelContextBatchID(identity, event.BatchOrdinal) ||
		strings.TrimSpace(event.PlayerInputID) != intent.PlayerInputID || strings.TrimSpace(event.BatchHash) != intent.Hash {
		return ModelContextBatchEvent{}, fmt.Errorf("%w: persisted batch identity or hash changed", ErrModelContextBatchIdentityConflict)
	}
	event.Type = StoryEventTypeModelContextBatch
	event.ID = strings.TrimSpace(event.ID)
	event.ParentID = strings.TrimSpace(event.ParentID)
	event.BranchID = intent.BranchID
	event.PlayerInputID = intent.PlayerInputID
	event.AgentCommandID = identity.CommandID
	event.AgentOperationID = identity.OperationID
	event.BatchHash = intent.Hash
	event.Messages = intent.Messages
	return event, nil
}

func normalizeDomainCommitIdentity(identity DomainCommitIdentity) DomainCommitIdentity {
	identity.CommandID = strings.TrimSpace(identity.CommandID)
	identity.OperationID = strings.TrimSpace(identity.OperationID)
	return identity
}

func deterministicModelContextBatchID(identity DomainCommitIdentity, ordinal int) string {
	identity = normalizeDomainCommitIdentity(identity)
	sum := sha256.Sum256([]byte(identity.CommandID + "\x00" + identity.OperationID + "\x00" + fmt.Sprint(identity.Cycle) + "\x00" + fmt.Sprint(ordinal)))
	return "model-context-batch-" + hex.EncodeToString(sum[:16])
}

func modelContextBatchReceipt(identity DomainCommitIdentity, event ModelContextBatchEvent) ModelContextBatchReceipt {
	return ModelContextBatchReceipt{Identity: identity, Hash: event.BatchHash, Revision: event.ID, Event: event}
}

func modelContextBatchesForPlayerInput(lines []StoryEventRecord, branchID, playerInputID string) ([]ModelContextBatchEvent, error) {
	batches := make([]ModelContextBatchEvent, 0)
	for _, record := range lines {
		if record.Envelope.Type != StoryEventTypeModelContextBatch || record.Envelope.BranchID != branchID {
			continue
		}
		var event ModelContextBatchEvent
		if err := mapToStruct(record.Raw, &event); err != nil {
			return nil, err
		}
		normalized, err := normalizeModelContextBatchEvent(event)
		if err != nil {
			return nil, err
		}
		if normalized.PlayerInputID == playerInputID {
			batches = append(batches, normalized)
		}
	}
	sort.SliceStable(batches, func(i, j int) bool { return batches[i].BatchOrdinal < batches[j].BatchOrdinal })
	for index, batch := range batches {
		if batch.BatchOrdinal != index {
			return nil, fmt.Errorf("%w: player input %s has a missing or duplicate batch ordinal", ErrModelContextBatchIdentityConflict, playerInputID)
		}
	}
	return batches, nil
}

func mergeModelContextBatchesForPlayerInput(
	lines []StoryEventRecord,
	branchID, playerInputID string,
	requested []ModelContextMessage,
) ([]ModelContextMessage, error) {
	durable, err := modelContextBatchesForPlayerInput(lines, branchID, playerInputID)
	if err != nil {
		return nil, err
	}
	if len(durable) == 0 {
		return sanitizeModelContextMessages(requested), nil
	}
	result := make([]ModelContextMessage, 0, len(requested))
	durableCounts := make(map[string]int, len(durable))
	for _, batch := range durable {
		result = append(result, batch.Messages...)
		durableCounts[modelContextBatchMessagesKey(batch.Messages)]++
	}
	if len(requested) == 0 {
		return result, nil
	}
	requestedBatches, err := splitCanonicalModelContextBatches(requested)
	if err != nil {
		return nil, err
	}
	for _, batch := range requestedBatches {
		key := modelContextBatchMessagesKey(batch)
		if durableCounts[key] > 0 {
			durableCounts[key]--
			continue
		}
		result = append(result, batch...)
	}
	return result, nil
}

func modelContextBatchMessagesKey(messages []ModelContextMessage) string {
	data, _ := json.Marshal(messages)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
