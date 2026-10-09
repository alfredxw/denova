package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/alfredxw/denova/agent/schema"
	"github.com/alfredxw/denova/agent/session"
	"github.com/alfredxw/denova/agent/session/canonical"
)

// ProjectCheckpoint returns a complete durable projection. Lifecycle treats
// State and Metadata as opaque bytes and commits them with capability changes.
func ProjectCheckpoint(raw json.RawMessage, capabilities map[string]json.RawMessage) (json.RawMessage, PersistedMessageCheckpoint, error) {
	state, err := decodeEngineTranscript(raw)
	if err != nil {
		return nil, PersistedMessageCheckpoint{}, err
	}
	state, err = projectCanonicalWindow(state, capabilities)
	if err != nil {
		return nil, PersistedMessageCheckpoint{}, err
	}
	checkpoint, err := canonicalMessageCheckpoint(state)
	if err != nil {
		return nil, PersistedMessageCheckpoint{}, err
	}
	encoded, err := json.Marshal(state)
	return encoded, checkpoint, err
}

// AlignCheckpoint preserves the committed history coordinate while execution
// finishes an output whose product transaction may already have committed.
func AlignCheckpoint(raw json.RawMessage, previous PersistedMessageCheckpoint, output *DomainCommitState) (json.RawMessage, error) {
	state, err := decodeEngineTranscript(raw)
	if err != nil {
		return nil, err
	}
	if len(previous.Metadata) != 0 {
		var committed engineTranscript
		if err := json.Unmarshal(previous.Metadata, &committed); err != nil {
			return nil, err
		}
		state.HistoryHead = committed.HistoryHead
	}
	if output != nil {
		state.HistoryHead.Revision = ""
		if state.ActiveModelUser == nil {
			state.HistoryHead.Revision = output.Revision
		}
	}
	return json.Marshal(state)
}

// AlignedCanonicalState answers whether an in-memory checkpoint already represents
// the current canonical source, without exposing its transcript to lifecycle.
func AlignedCanonicalState(raw json.RawMessage, head canonical.CanonicalHistoryHead) (json.RawMessage, bool, error) {
	if len(raw) == 0 {
		return nil, false, nil
	}
	state, err := decodeEngineTranscript(raw)
	if err != nil {
		return nil, false, err
	}
	return append(json.RawMessage(nil), raw...), state.HistoryHead == head, nil
}

// CanonicalCommit is a candidate transaction, not committed state. Its caller
// publishes it only after the product and Agent journal CAS both succeed.
type CanonicalCommit struct {
	State         json.RawMessage
	Snapshot      TurnSnapshot
	Checkpoint    PersistedMessageCheckpoint
	CompletionIDs []string
}

func PrepareCanonicalCommit(key session.Key, current json.RawMessage, capabilities map[string]json.RawMessage, update CanonicalUpdate, receipt canonical.CommitReceipt) (CanonicalCommit, error) {
	result := CanonicalCommit{Snapshot: update.Snapshot}
	raw := append(json.RawMessage(nil), update.State...)
	switch update.Stage {
	case canonical.CommitInput:
		input, err := DecodeInput(result.Snapshot.Input)
		if err != nil {
			return CanonicalCommit{}, err
		}
		state, err := decodeEngineTranscript(current)
		if err != nil {
			return CanonicalCommit{}, err
		}
		state.DefinitionKey, state.BehaviorKey, state.MaterializedFingerprint, state.PreparationStage = "", "", "", ""
		state.PreparedContext = nil
		state.DefinitionOperationID, state.DefinitionCommandID, state.DefinitionCycle = string(result.Snapshot.OperationID), string(result.Snapshot.CommandID), result.Snapshot.Cycle
		state.ContextSequence, state.LastResponseOrdinal = 0, 0
		state.ActiveUserIndex, state.ActiveModelUser = state.Archive.Count(state.Messages), schema.UserMessageWithAttachments(input.Text, input.Attachments)
		state.Messages = append(state.Messages, state.ActiveModelUser.Clone())
		state.HostData = schema.CloneHostData(input.HostData)
		raw, err = json.Marshal(state)
		if err != nil {
			return CanonicalCommit{}, err
		}
		result.Snapshot.InputCommit = &DomainCommitState{Identity: engineCommitIdentity(canonicalCommitIdentity(key, result.Snapshot, canonical.CommitInput)), Hash: update.Hash, Revision: receipt.Revision}
	case canonical.CommitContext:
	case canonical.CommitOutput:
		result.Snapshot.OutputCommit = &DomainCommitState{Identity: engineCommitIdentity(canonicalCommitIdentity(key, result.Snapshot, canonical.CommitOutput)), Hash: update.Hash, Revision: receipt.Revision}
		if len(raw) == 0 {
			raw = append(json.RawMessage(nil), current...)
		}
	default:
		return CanonicalCommit{}, fmt.Errorf("unsupported canonical checkpoint stage %q", update.Stage)
	}
	state, err := decodeEngineTranscript(raw)
	if err != nil {
		return CanonicalCommit{}, err
	}
	// Output acceptance precedes the final transcript projection. A crash at
	// this boundary must reload the source instead of trusting an older window.
	state.HistoryHead.Revision = receipt.Revision
	if update.Stage == canonical.CommitOutput {
		state.HistoryHead.Revision = ""
	}
	states := schema.CloneRawStateMap(capabilities)
	for key, value := range update.CapabilityStates {
		states[key] = value
	}
	result.State, err = encodeCanonicalWindow(state, states)
	if err != nil {
		return CanonicalCommit{}, err
	}
	result.Checkpoint, err = CanonicalMessageCheckpoint(result.State)
	if err != nil {
		return CanonicalCommit{}, err
	}
	projected, err := decodeEngineTranscript(result.State)
	if err != nil {
		return CanonicalCommit{}, err
	}
	for _, message := range projected.Messages {
		if message.TaskCompletion != nil {
			result.CompletionIDs = append(result.CompletionIDs, message.TaskCompletion.CompletionID)
		}
	}
	return result, nil
}

// expandCanonicalArchive is used only for explicit removal/rebuild. Normal
// model steps retain the bounded archive projection.
func expandCanonicalArchive(ctx context.Context, source canonical.CanonicalHistorySource, state engineTranscript) (engineTranscript, error) {
	if source == nil {
		return engineTranscript{}, errors.New("canonical history source is unavailable for compaction removal")
	}
	head, err := source.CanonicalHistoryHead(ctx)
	if err != nil {
		return engineTranscript{}, err
	}
	if head != state.HistoryHead {
		return engineTranscript{}, schema.ErrDefinitionMismatch
	}
	messages, err := source.CanonicalMessages(ctx)
	if err != nil {
		return engineTranscript{}, err
	}
	after, err := source.CanonicalHistoryHead(ctx)
	if err != nil {
		return engineTranscript{}, err
	}
	if head != after {
		return engineTranscript{}, schema.ErrSessionBusy
	}
	ordered := canonical.CanonicalContextStateOrder(messages)
	if err := canonical.ValidateImportedTranscript(ordered); err != nil {
		return engineTranscript{}, err
	}
	if len(ordered) != state.Archive.Count(state.Messages) {
		return engineTranscript{}, schema.ErrDefinitionMismatch
	}
	state.Messages, state.Archive = ordered, nil
	state.Version = engineTranscriptVersion
	return state, nil
}
