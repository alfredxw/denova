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

	agent "github.com/alfredxw/denova/agent"
)

var ErrModelContextBatchIdentityConflict = errors.New("model context batch identity conflict")

const (
	ModelContextBatchKindTool           = "tool_batch"
	ModelContextBatchKindState          = "context_state"
	ModelContextBatchKindTaskCompletion = "task_completion"
)

// ModelContextBatchIntent is one complete assistant tool-call batch staged
// after its player input and before the final narrative exists. Ordinal is
// scoped to the durable Agent cycle and makes retries deterministic.
type ModelContextBatchIntent struct {
	Identity      DomainCommitIdentity  `json:"identity"`
	BranchID      string                `json:"branch_id"`
	PlayerInputID string                `json:"player_input_id"`
	Ordinal       int                   `json:"ordinal"`
	Kind          string                `json:"kind,omitempty"`
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
	Kind             string                `json:"kind,omitempty"`
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
	return newAgentContextBatchIntent(identity, branchID, ModelContextBatchKindTool, ordinal, messages)
}

// NewAgentContextBatchIntent creates one exact canonical batch for context
// state, tool protocol, or delegated task completion messages.
func NewAgentContextBatchIntent(
	identity DomainCommitIdentity,
	branchID string,
	kind string,
	ordinal int,
	messages []ModelContextMessage,
) (ModelContextBatchIntent, error) {
	return newAgentContextBatchIntent(identity, branchID, kind, ordinal, messages)
}

func newAgentContextBatchIntent(
	identity DomainCommitIdentity,
	branchID string,
	kind string,
	ordinal int,
	messages []ModelContextMessage,
) (ModelContextBatchIntent, error) {
	identity = normalizeDomainCommitIdentity(identity)
	branchID, kind = strings.TrimSpace(branchID), strings.TrimSpace(kind)
	messages = sanitizeModelContextMessages(messages)
	if identity.CommandID == "" || identity.OperationID == "" || identity.Cycle <= 0 || branchID == "" || ordinal < 0 || len(messages) == 0 {
		return ModelContextBatchIntent{}, fmt.Errorf("%w: invalid Agent context batch identity", ErrModelContextBatchIdentityConflict)
	}
	if err := validateAgentContextMessages(kind, messages); err != nil {
		return ModelContextBatchIntent{}, err
	}
	playerInputID := deterministicPlayerInputID(identity)
	payloadValue := struct {
		BranchID      string                `json:"branch_id"`
		PlayerInputID string                `json:"player_input_id"`
		Ordinal       int                   `json:"ordinal"`
		Kind          string                `json:"kind,omitempty"`
		Messages      []ModelContextMessage `json:"messages"`
	}{BranchID: branchID, PlayerInputID: playerInputID, Ordinal: ordinal, Messages: messages}
	// Preserve the released tool-batch hash shape. New kinds include their kind
	// so the same ordinal cannot be reinterpreted after a crash.
	if kind != ModelContextBatchKindTool {
		payloadValue.Kind = kind
	}
	payload, err := json.Marshal(payloadValue)
	if err != nil {
		return ModelContextBatchIntent{}, err
	}
	sum := sha256.Sum256(payload)
	return ModelContextBatchIntent{
		Identity: identity, BranchID: branchID, PlayerInputID: playerInputID,
		Ordinal: ordinal, Kind: kind, Messages: messages, Hash: "sha256:" + hex.EncodeToString(sum[:]),
	}, nil
}

func validateAgentContextMessages(kind string, messages []ModelContextMessage) error {
	switch kind {
	case ModelContextBatchKindTool:
		batches, err := splitCanonicalModelContextBatches(messages)
		if err != nil || len(batches) != 1 {
			if err != nil {
				return err
			}
			return fmt.Errorf("%w: tool context commit must contain exactly one batch", ErrModelContextBatchIdentityConflict)
		}
	case ModelContextBatchKindState:
		for _, value := range messages {
			if message := AgentMessageFromModelContext(value); message == nil || !agent.IsContextStateMessage(message) {
				return fmt.Errorf("%w: context-state batch contains a non-state message", ErrModelContextBatchIdentityConflict)
			}
		}
	case ModelContextBatchKindTaskCompletion:
		for _, value := range messages {
			if message := AgentMessageFromModelContext(value); message == nil || message.Role != agent.User || message.TaskCompletion == nil {
				return fmt.Errorf("%w: task-completion batch contains an invalid message", ErrModelContextBatchIdentityConflict)
			}
		}
	default:
		return fmt.Errorf("%w: unsupported context batch kind %q", ErrModelContextBatchIdentityConflict, kind)
	}
	return nil
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
	kind := strings.TrimSpace(intent.Kind)
	if kind == "" {
		kind = ModelContextBatchKindTool
	}
	canonical, err := newAgentContextBatchIntent(intent.Identity, intent.BranchID, kind, intent.Ordinal, intent.Messages)
	if err != nil {
		return ModelContextBatchReceipt{}, err
	}
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
		AgentCycle: canonical.Identity.Cycle, BatchOrdinal: canonical.Ordinal, Kind: canonical.Kind,
		BatchHash: canonical.Hash, Messages: canonical.Messages,
	}
	if canonical.Kind == ModelContextBatchKindTool {
		event.Kind = ""
	}
	continuationEvents, err := newModelContextProviderContinuationEvents(event.ID, event.BranchID, event.Ts, event.Messages)
	if err != nil {
		return ModelContextBatchReceipt{}, err
	}
	meta.UpdatedAt = now
	newEvents := []any{event}
	newEvents = append(newEvents, continuationEvents...)
	if err := s.appendStoryTransactionLocked(storyID, meta, newEvents...); err != nil {
		return ModelContextBatchReceipt{}, err
	}
	s.syncStoryIndexProjectionLocked(storyID)
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
		continuations, err := modelContextProviderContinuationsByOwner(lines)
		if err != nil {
			return ModelContextBatchReceipt{}, false, err
		}
		normalized.Messages, err = hydrateModelContextProviderContinuations(normalized.Messages, normalized.ID, continuations)
		if err != nil {
			return ModelContextBatchReceipt{}, false, err
		}
		if normalized.BranchID != intent.BranchID || normalized.PlayerInputID != intent.PlayerInputID || normalized.BatchHash != intent.Hash {
			return ModelContextBatchReceipt{}, false, fmt.Errorf("%w: batch ordinal %d has different content", ErrModelContextBatchIdentityConflict, intent.Ordinal)
		}
		storedContinuationHash, err := modelContextProviderContinuationFingerprint(normalized.Messages)
		if err != nil {
			return ModelContextBatchReceipt{}, false, err
		}
		intentContinuationHash, err := modelContextProviderContinuationFingerprint(intent.Messages)
		if err != nil {
			return ModelContextBatchReceipt{}, false, err
		}
		if storedContinuationHash != intentContinuationHash {
			return ModelContextBatchReceipt{}, false, fmt.Errorf("%w: batch ordinal %d has different provider continuation", ErrModelContextBatchIdentityConflict, intent.Ordinal)
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
	kind := strings.TrimSpace(event.Kind)
	if kind == "" {
		kind = ModelContextBatchKindTool
	}
	intent, err := newAgentContextBatchIntent(identity, event.BranchID, kind, event.BatchOrdinal, event.Messages)
	if err != nil {
		return ModelContextBatchEvent{}, err
	}
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
	if intent.Kind == ModelContextBatchKindTool {
		event.Kind = ""
	} else {
		event.Kind = intent.Kind
	}
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
	continuations, err := modelContextProviderContinuationsByOwner(lines)
	if err != nil {
		return nil, err
	}
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
		normalized.Messages, err = hydrateModelContextProviderContinuations(normalized.Messages, normalized.ID, continuations)
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
	for _, batch := range durable {
		result = append(result, batch.Messages...)
	}
	if len(requested) == 0 {
		return result, nil
	}
	sanitized := sanitizeModelContextMessages(requested)
	if len(sanitized) != len(requested) || modelContextBatchMessagesKey(sanitized) != modelContextBatchMessagesKey(result) {
		return nil, fmt.Errorf("%w: completed Turn context differs from its durable batches", ErrModelContextBatchIdentityConflict)
	}
	return result, nil
}

func modelContextBatchMessagesKey(messages []ModelContextMessage) string {
	data, _ := json.Marshal(messages)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
