package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	runstate "github.com/alfredxw/denova/agent/internal/runtime"
)

const transcriptSyncCapability = "agent.transcript_sync"

var ErrTranscriptSyncConflict = errors.New("Agent transcript source revision conflict")

// TranscriptSyncRequest replaces the raw conversation generation with one
// canonical product projection. Source is the stable identity of that
// canonical projection (for example one Story branch), and SourceRevision
// must be monotonic within the Session lane. SourceHash, when supplied, must
// equal TranscriptHash(Messages). This makes exact retries cheap and turns a
// reused identity/revision with changed content into an explicit conflict
// instead of silent history drift.
type TranscriptSyncRequest struct {
	Source         CapabilityIdentity
	SourceRevision uint64
	SourceHash     string
	Messages       []*Message
}

// TranscriptSyncState is the durable provenance of the currently imported
// raw conversation generation. Revision is Agent-owned; SourceRevision and
// SourceHash are product-owned canonical identity.
type TranscriptSyncState struct {
	Revision       uint64             `json:"revision"`
	Source         CapabilityIdentity `json:"source"`
	SourceRevision uint64             `json:"source_revision"`
	SourceHash     string             `json:"source_hash"`
	MessageCount   int                `json:"message_count"`
	SyncedAt       time.Time          `json:"synced_at"`
}

type TranscriptSyncResult struct {
	State   TranscriptSyncState
	Applied bool
	// Rebuilt distinguishes a canonical history replacement from a provenance
	// advance over an already identical Agent transcript. A forward provenance
	// advance preserves conversation-scoped maintenance capabilities.
	Rebuilt bool
}

// TranscriptHash validates a complete provider transcript and returns its
// stable content identity. Tool calls and their results must form contiguous,
// complete batches so a product import can never persist a half protocol turn.
func TranscriptHash(messages []*Message) (string, error) {
	if err := validateImportedTranscript(messages); err != nil {
		return "", err
	}
	return hashCanonical(cloneMessages(messages))
}

// SyncTranscript atomically rebuilds the idle Session's raw transcript from a
// canonical product history. Goal deliberately survives; Todo, Cleanup,
// Compaction, and Compaction health belong to the replaced conversation and
// are removed in the same journal append. Their prior raw journal records stay
// available for audit but can no longer reappear through remove/replay.
func (session *Session) SyncTranscript(ctx context.Context, request TranscriptSyncRequest) (TranscriptSyncResult, error) {
	if err := session.usable(); err != nil {
		return TranscriptSyncResult{}, err
	}
	if err := request.Source.validate("Transcript Sync source"); err != nil {
		return TranscriptSyncResult{}, err
	}
	hash, err := TranscriptHash(request.Messages)
	if err != nil {
		return TranscriptSyncResult{}, err
	}
	if supplied := strings.TrimSpace(request.SourceHash); supplied != "" && supplied != hash {
		return TranscriptSyncResult{}, fmt.Errorf("%w: supplied source hash does not match Messages", ErrTranscriptSyncConflict)
	}
	request.SourceHash = hash

	checkpoint, err := session.harness.EngineCheckpoint(ctx)
	if err != nil {
		return TranscriptSyncResult{}, mapRuntimeError(err)
	}
	current, present, err := transcriptSyncStateFrom(checkpoint.Capabilities)
	if err != nil {
		return TranscriptSyncResult{}, err
	}
	if present {
		switch {
		case request.Source != current.Source:
			return TranscriptSyncResult{}, fmt.Errorf(
				"%w: source identity changed from %q to %q",
				ErrTranscriptSyncConflict, current.Source.Kind, request.Source.Kind,
			)
		case request.SourceRevision < current.SourceRevision:
			return TranscriptSyncResult{}, fmt.Errorf(
				"%w: source revision moved backwards from %d to %d",
				ErrTranscriptSyncConflict, current.SourceRevision, request.SourceRevision,
			)
		case request.SourceRevision == current.SourceRevision:
			if request.SourceHash != current.SourceHash || len(request.Messages) != current.MessageCount {
				return TranscriptSyncResult{}, fmt.Errorf(
					"%w: source revision %d has different content",
					ErrTranscriptSyncConflict, request.SourceRevision,
				)
			}
			// Verify the idle fence and exact sync generation inside the actor.
			// Do not overwrite messages appended after this imported base.
			err = session.harness.ReplaceEngineCheckpoint(ctx, runstate.EngineCheckpointUpdate{
				ExpectedState: checkpoint.StateDescriptor,
				CapabilityGuards: map[string]runstate.PayloadDescriptor{
					transcriptSyncCapability: checkpoint.CapabilityDescriptor(transcriptSyncCapability),
				},
			})
			return TranscriptSyncResult{State: current}, mapRuntimeError(err)
		}
	}
	currentTranscript, err := decodeEngineTranscript(checkpoint.State)
	if err != nil {
		return TranscriptSyncResult{}, err
	}
	if _, _, err := applyClearToTranscript(&currentTranscript, checkpoint.Capabilities); err != nil {
		return TranscriptSyncResult{}, err
	}
	sourceEquivalent, err := transcriptSourceEquivalent(currentTranscript.Messages, request.Messages)
	if err != nil {
		return TranscriptSyncResult{}, fmt.Errorf("compare current Agent transcript with canonical source: %w", err)
	}
	// The first import always establishes a clean product-owned generation.
	// Later revisions that already equal the complete raw Agent transcript only
	// advance provenance: normal product settlement must not discard a useful
	// Cleanup/Compaction/Todo projection on every turn.
	rebuild := !present || !sourceEquivalent

	transcript := engineTranscript{Version: engineTranscriptVersion, Messages: cloneMessages(request.Messages)}
	encodedTranscript, err := json.Marshal(transcript)
	if err != nil {
		return TranscriptSyncResult{}, fmt.Errorf("encode synchronized Agent transcript: %w", err)
	}
	next := TranscriptSyncState{
		Revision: 1, Source: request.Source, SourceRevision: request.SourceRevision, SourceHash: request.SourceHash,
		MessageCount: len(request.Messages), SyncedAt: time.Now().UTC(),
	}
	if present {
		next.Revision = current.Revision + 1
	}
	encodedState, err := json.Marshal(next)
	if err != nil {
		return TranscriptSyncResult{}, fmt.Errorf("encode Agent transcript sync state: %w", err)
	}

	guards := map[string]runstate.PayloadDescriptor{
		transcriptSyncCapability: checkpoint.CapabilityDescriptor(transcriptSyncCapability),
	}
	changes := make([]runstate.EngineCapabilityState, 0, 6)
	if rebuild {
		for _, capability := range []string{clearCapability, TodoCapability, cleanupCapability, compactionCapability, compactionHealthCapability} {
			if _, exists := checkpoint.Capabilities[capability]; !exists {
				continue
			}
			changes = append(changes, runstate.EngineCapabilityState{
				Capability: capability, Expected: checkpoint.CapabilityDescriptor(capability), Delete: true,
			})
		}
	}
	changes = append(changes, runstate.EngineCapabilityState{
		Capability: transcriptSyncCapability,
		Expected:   checkpoint.CapabilityDescriptor(transcriptSyncCapability),
		State:      encodedState,
	})
	update := runstate.EngineCheckpointUpdate{
		ExpectedState:    checkpoint.StateDescriptor,
		CapabilityGuards: guards, CapabilityChanges: changes,
	}
	if rebuild {
		update.State = encodedTranscript
	}
	err = session.harness.ReplaceEngineCheckpoint(ctx, update)
	if err != nil {
		return TranscriptSyncResult{}, mapRuntimeError(err)
	}
	return TranscriptSyncResult{State: next, Applied: true, Rebuilt: rebuild}, nil
}

// transcriptSourceEquivalent compares only fields owned by the canonical
// product source. AgentMeta, ResponseMeta, and provider reasoning are generated
// by the Agent loop; asking a product store to duplicate them would create a
// second execution-metadata authority. Denova's model-history middleware drops
// settled reasoning from later provider requests, while raw Agent history keeps
// it for recovery and audit. A product revision that settles the same visible
// response can therefore advance without discarding maintenance capabilities.
func transcriptSourceEquivalent(current, canonical []*Message) (bool, error) {
	current = productOwnedTranscriptMessages(current)
	if len(current) != len(canonical) {
		return false, nil
	}
	project := func(messages []*Message) ([]*Message, error) {
		if err := validateImportedTranscript(messages); err != nil {
			return nil, err
		}
		projected := cloneMessages(messages)
		for _, message := range projected {
			if message == nil {
				continue
			}
			message.AgentMeta = nil
			message.ResponseMeta = nil
			message.ReasoningContent = ""
		}
		return projected, nil
	}
	left, err := project(current)
	if err != nil {
		return false, err
	}
	right, err := project(canonical)
	if err != nil {
		return false, err
	}
	leftHash, err := hashCanonical(left)
	if err != nil {
		return false, err
	}
	rightHash, err := hashCanonical(right)
	if err != nil {
		return false, err
	}
	return leftHash == rightHash, nil
}

func productOwnedTranscriptMessages(messages []*Message) []*Message {
	projected := make([]*Message, 0, len(messages))
	for _, message := range messages {
		if IsContextStateMessage(message) {
			continue
		}
		projected = append(projected, message)
	}
	return projected
}

func transcriptSyncStateFrom(capabilities map[string]json.RawMessage) (TranscriptSyncState, bool, error) {
	raw, present := capabilities[transcriptSyncCapability]
	if !present {
		return TranscriptSyncState{}, false, nil
	}
	var state TranscriptSyncState
	if err := json.Unmarshal(raw, &state); err != nil {
		return TranscriptSyncState{}, false, fmt.Errorf("decode Agent transcript sync state: %w", err)
	}
	if state.Revision == 0 || state.Source.validate("durable Transcript Sync source") != nil ||
		len(state.SourceHash) != 64 || state.MessageCount < 0 || state.SyncedAt.IsZero() {
		return TranscriptSyncState{}, false, errors.New("durable Agent transcript sync state is invalid")
	}
	return state, true, nil
}

func validateImportedTranscript(messages []*Message) error {
	pending := make(map[string]struct{})
	for index, message := range messages {
		if message == nil {
			return fmt.Errorf("Agent transcript message %d is nil", index)
		}
		if len(pending) > 0 && message.Role != ToolRole {
			return fmt.Errorf("Agent transcript message %d splits an incomplete tool-result batch", index)
		}
		switch message.Role {
		case User:
			if len(message.ToolCalls) != 0 || strings.TrimSpace(message.ToolCallID) != "" {
				return fmt.Errorf("Agent transcript user message %d contains tool protocol fields", index)
			}
		case Assistant:
			if strings.TrimSpace(message.ToolCallID) != "" {
				return fmt.Errorf("Agent transcript assistant message %d contains a tool result ID", index)
			}
			for _, call := range message.ToolCalls {
				id := strings.TrimSpace(call.ID)
				if id == "" || strings.TrimSpace(call.Function.Name) == "" {
					return fmt.Errorf("Agent transcript assistant message %d has an invalid tool call", index)
				}
				if _, duplicate := pending[id]; duplicate {
					return fmt.Errorf("Agent transcript assistant message %d repeats tool call %q", index, id)
				}
				pending[id] = struct{}{}
			}
		case ToolRole:
			if len(message.ToolCalls) != 0 {
				return fmt.Errorf("Agent transcript tool message %d contains nested tool calls", index)
			}
			id := strings.TrimSpace(message.ToolCallID)
			if _, ok := pending[id]; !ok || id == "" {
				return fmt.Errorf("Agent transcript tool message %d has no matching pending call", index)
			}
			delete(pending, id)
		case System:
			return fmt.Errorf("Agent transcript message %d is system-owned and cannot be product-imported", index)
		default:
			return fmt.Errorf("Agent transcript message %d has unsupported role %q", index, message.Role)
		}
	}
	if len(pending) > 0 {
		return errors.New("Agent transcript ends with an incomplete tool-result batch")
	}
	return nil
}
