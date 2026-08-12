package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	runstate "github.com/alfredxw/denova/agent/runtime"
)

const (
	maxCompactionContextDataBytes = 8 << 20
)

type compactionCommandEnvelope struct {
	Version uint16                   `json:"version"`
	Manager CapabilityIdentity       `json:"manager"`
	Compact *CompactionRequest       `json:"compact,omitempty"`
	Remove  *CompactionRemoveRequest `json:"remove,omitempty"`
}

func (engine *definitionEngine) RunStructural(
	ctx context.Context,
	request runstate.StructuralEngineRequest,
	emit runstate.EngineEventSink,
) (runstate.EngineResult, error) {
	if engine == nil || engine.source == nil || emit == nil {
		return runstate.EngineResult{}, ErrDefinitionUnavailable
	}
	transcript, err := decodeEngineTranscript(request.State)
	if err != nil {
		return runstate.EngineResult{}, err
	}
	clearState, clearPresent, err := applyClearToTranscript(&transcript, request.Capabilities)
	if err != nil {
		return runstate.EngineResult{}, err
	}
	current, present, raw, err := compactionStateFrom(request.Capabilities)
	if err != nil {
		return runstate.EngineResult{}, err
	}
	current, present = clearCompaction(current, present, clearState, clearPresent)
	prepared, err := prepareDefinition(ctx, engine.source, PrepareRequest{
		Session: SessionView{Key: engine.key, Revision: uint64(request.Snapshot.ContextCursor)},
		Run:     runViewForStructural(request.Snapshot),
		Reason:  TurnReasonRecovery, DefinitionKey: transcript.DefinitionKey, RestoreKey: transcript.RestoreKey,
		HostData:   cloneHostData(transcript.HostData),
		Compaction: compactionStatePointer(current, present),
	})
	if err != nil {
		return runstate.EngineResult{}, err
	}
	if engine.persistent {
		if err := validatePersistentDefinition(prepared.definition); err != nil {
			return runstate.EngineResult{}, err
		}
	}
	if prepared.definition.Compaction == nil {
		return runstate.EngineResult{}, ErrCapabilityUnsupported
	}
	if transcript.DefinitionKey != "" && transcript.DefinitionKey != prepared.definitionKey ||
		transcript.RestoreKey != "" && transcript.RestoreKey != prepared.restoreKey {
		return runstate.EngineResult{}, ErrDefinitionMismatch
	}
	var envelope compactionCommandEnvelope
	if err := json.Unmarshal(request.Snapshot.Ref.RestoreDescriptor, &envelope); err != nil {
		return runstate.EngineResult{}, fmt.Errorf("decode Compaction command: %w", err)
	}
	if envelope.Version != 1 || envelope.Manager != prepared.definition.Compaction.Identity() {
		return runstate.EngineResult{}, fmt.Errorf("%w: Compaction Manager changed", ErrDefinitionMismatch)
	}
	session := SessionView{Key: engine.key, Revision: uint64(request.Snapshot.ContextCursor)}
	run := runViewForStructural(request.Snapshot)
	switch request.Snapshot.Kind {
	case runstate.StructuralCompactContext:
		if envelope.Compact == nil || envelope.Remove != nil {
			return runstate.EngineResult{}, errors.New("Compaction command envelope does not match compact operation")
		}
		if current.ID == compactionID(request.Snapshot.OperationID) && !current.Removed {
			return runstate.EngineResult{Status: runstate.EngineCompleted}, nil
		}
		next, changed, compactErr := executeCompaction(ctx, prepared, session, run, transcript.Messages, "", current, present, *envelope.Compact, compactionID(request.Snapshot.OperationID))
		if compactErr != nil {
			return runstate.EngineResult{}, compactErr
		}
		if changed {
			encoded, encodeErr := json.Marshal(next)
			if encodeErr != nil {
				return runstate.EngineResult{}, encodeErr
			}
			if err := emit(runstate.EngineCapabilityState{
				Capability: compactionCapability, Expected: describeCapabilityState(raw), State: encoded,
			}); err != nil {
				return runstate.EngineResult{}, err
			}
		}
	case runstate.StructuralRemoveCompaction:
		if envelope.Remove == nil || envelope.Compact != nil {
			return runstate.EngineResult{}, errors.New("Compaction command envelope does not match remove operation")
		}
		remove := *envelope.Remove
		if !present || current.Removed {
			return runstate.EngineResult{Status: runstate.EngineCompleted}, nil
		}
		if current.ID != remove.ID || remove.ExpectedRevision != 0 && current.Revision != remove.ExpectedRevision {
			return runstate.EngineResult{}, ErrDefinitionMismatch
		}
		current.Revision++
		current.Removed = true
		encoded, encodeErr := json.Marshal(current)
		if encodeErr != nil {
			return runstate.EngineResult{}, encodeErr
		}
		if err := emit(runstate.EngineCapabilityState{
			Capability: compactionCapability, Expected: describeCapabilityState(raw), State: encoded,
		}); err != nil {
			return runstate.EngineResult{}, err
		}
	default:
		return runstate.EngineResult{}, fmt.Errorf("unsupported structural operation %q", request.Snapshot.Kind)
	}
	return runstate.EngineResult{Status: runstate.EngineCompleted}, nil
}

func executeCompaction(
	ctx context.Context,
	prepared preparedDefinition,
	session SessionView,
	run RunView,
	messages []*Message,
	currentInput string,
	current CompactionState,
	present bool,
	request CompactionRequest,
	checkpointID string,
) (CompactionState, bool, error) {
	if present && current.ID == checkpointID && !current.Removed {
		return current, false, nil
	}
	if request.ExpectedID != "" && (!present || current.ID != request.ExpectedID) ||
		request.ExpectedRevision != 0 && (!present || current.Revision != request.ExpectedRevision) {
		return CompactionState{}, false, ErrDefinitionMismatch
	}
	summaryLimit := prepared.definition.Compaction.SummaryLimitBytes()
	modelRequest, err := compactionModelRequest(prepared, messages, currentInput, current, present)
	if err != nil {
		return CompactionState{}, false, err
	}
	plan, err := prepared.definition.Compaction.Plan(ctx, CompactionPlanRequest{
		Session: session, Run: run, Messages: cloneMessages(messages), ModelRequest: modelRequest,
		Force:   request.Force || present && len(current.Summary) > summaryLimit,
		Current: current, Present: present,
	})
	if err != nil {
		return CompactionState{}, false, err
	}
	if plan.Action == CompactionNone {
		return current, false, nil
	}
	if plan.Action != CompactionCreate || plan.SourceFrom < 0 || plan.SourceTo <= plan.SourceFrom || plan.SourceTo > len(messages) {
		return CompactionState{}, false, errors.New("Compaction Manager returned an invalid source range")
	}
	wantHash, err := hashCanonical(messages[plan.SourceFrom:plan.SourceTo])
	if err != nil {
		return CompactionState{}, false, err
	}
	if strings.TrimSpace(plan.SourceHash) == "" {
		plan.SourceHash = wantHash
	} else if plan.SourceHash != wantHash {
		return CompactionState{}, false, errors.New("Compaction Manager source hash does not match the selected messages")
	}
	if strings.TrimSpace(plan.SourceRevision) == "" {
		plan.SourceRevision = fmt.Sprintf("session:%d", session.Revision)
	}
	checkpoint, err := prepared.definition.Compaction.Compact(ctx, CompactionCompactRequest{
		Session: session, Run: run, Messages: cloneMessages(messages), ModelRequest: modelRequest,
		Plan: plan, Current: current, Present: present,
	})
	if err != nil {
		return CompactionState{}, false, err
	}
	checkpoint.Summary = strings.TrimSpace(checkpoint.Summary)
	if checkpoint.Summary == "" || checkpoint.TokenEstimate < 0 {
		return CompactionState{}, false, errors.New("Compaction Manager returned an invalid checkpoint")
	}
	if len(checkpoint.Summary) > summaryLimit {
		return CompactionState{}, false, fmt.Errorf("%w: Compaction checkpoint is %d bytes and exceeds the target Agent summary limit %d", ErrContextLimit, len(checkpoint.Summary), summaryLimit)
	}
	if err := validateCompactionContextData(checkpoint.ContextData); err != nil {
		return CompactionState{}, false, err
	}
	revision := uint64(1)
	if present {
		revision = current.Revision + 1
	}
	return CompactionState{
		ID: checkpointID, Revision: revision,
		SourceRevision: plan.SourceRevision, SourceHash: plan.SourceHash,
		Summary: checkpoint.Summary, TokenEstimate: checkpoint.TokenEstimate,
		ReplacementFrom: plan.SourceFrom, ReplacementTo: plan.SourceTo,
		CreatedAt:   time.Now().UTC(),
		ContextData: cloneHostData(checkpoint.ContextData),
	}, true, nil
}

func validateCompactionContextData(data *HostData) error {
	if data == nil {
		return nil
	}
	if strings.TrimSpace(data.Type) == "" || data.Version == 0 || !json.Valid(data.Data) {
		return errors.New("Compaction ContextData requires Type, Version, and valid JSON Data")
	}
	if len(data.Data) > maxCompactionContextDataBytes {
		return fmt.Errorf("Compaction ContextData exceeds %d bytes", maxCompactionContextDataBytes)
	}
	return nil
}

func compactionStatePointer(state CompactionState, present bool) *CompactionState {
	if !present || state.Removed {
		return nil
	}
	cloned := state
	cloned.ContextData = cloneHostData(state.ContextData)
	return &cloned
}

func cloneCompactionState(state *CompactionState) *CompactionState {
	if state == nil {
		return nil
	}
	cloned := *state
	cloned.ContextData = cloneHostData(state.ContextData)
	return &cloned
}

func compactionID(operationID runstate.OperationID) string {
	return "compaction-" + string(operationID)
}

func (engine *definitionEngine) applyAutomaticCompaction(
	ctx context.Context,
	request runstate.EngineRequest,
	prepared preparedDefinition,
	messages []*Message,
	emit runstate.EngineEventSink,
) (CompactionState, bool, bool, error) {
	current, present, raw, err := compactionStateFrom(request.Snapshot.Capabilities)
	if err != nil || prepared.definition.Compaction == nil {
		return current, present, false, err
	}
	next, changed, err := executeCompaction(
		ctx, prepared,
		SessionView{Key: engine.key, Revision: uint64(request.Snapshot.ContextCursor)},
		runViewForTurn(request.Snapshot),
		messages, request.Snapshot.Input.Text, current, present, CompactionRequest{},
		fmt.Sprintf("compaction-%s-%d", request.Snapshot.OperationID, request.Snapshot.Cycle),
	)
	if err != nil {
		return CompactionState{}, false, false, err
	}
	if !changed {
		return current, present, false, nil
	}
	encoded, err := json.Marshal(next)
	if err != nil {
		return CompactionState{}, false, false, err
	}
	if err := emit(runstate.EngineCapabilityState{
		Capability: compactionCapability, Expected: describeCapabilityState(raw), State: encoded,
	}); err != nil {
		return CompactionState{}, false, false, err
	}
	return next, true, true, nil
}

func compactionStateFrom(states map[string]json.RawMessage) (CompactionState, bool, json.RawMessage, error) {
	raw, present := states[compactionCapability]
	if !present {
		return CompactionState{}, false, nil, nil
	}
	state, err := decodeCompactionState(raw)
	return state, true, append(json.RawMessage(nil), raw...), err
}

func decodeCompactionState(raw json.RawMessage) (CompactionState, error) {
	var state CompactionState
	if err := json.Unmarshal(raw, &state); err != nil {
		return CompactionState{}, fmt.Errorf("decode Compaction state: %w", err)
	}
	if strings.TrimSpace(state.ID) == "" || state.Revision == 0 || state.ReplacementFrom < 0 ||
		state.ReplacementTo <= state.ReplacementFrom || strings.TrimSpace(state.Summary) == "" {
		return CompactionState{}, errors.New("durable Compaction state is invalid")
	}
	return state, nil
}

func compactionModelRequest(
	prepared preparedDefinition,
	messages []*Message,
	currentInput string,
	current CompactionState,
	present bool,
) ([]*Message, error) {
	result := make([]*Message, 0, len(messages)+len(prepared.fragments)+2)
	if prepared.definition.Instructions != "" {
		result = append(result, SystemMessage(prepared.definition.Instructions))
	}
	effective, err := effectiveCompactionMessages(messages, current, present, prepared.definition.Compaction.SummaryLimitBytes())
	if err != nil {
		// Raw history is retained specifically so an oversized checkpoint can be
		// regenerated after the target Agent's configured limits are lowered.
		if !errors.Is(err, ErrContextLimit) {
			return nil, err
		}
		effective = cloneMessages(messages)
	}
	if strings.TrimSpace(currentInput) == "" {
		for _, fragment := range prepared.fragments {
			if fragment.Placement == ContextLeadingMessage {
				result = append(result, SystemMessage(renderContextFragment(fragment)))
			}
		}
		result = append(result, effective...)
		return result, nil
	}
	cycle, _, err := assembleCycleMessages(effective, currentInput, prepared.fragments)
	if err != nil {
		return nil, err
	}
	result = append(result, cycle...)
	return result, nil
}

func effectiveCompactionMessages(messages []*Message, state CompactionState, present bool, summaryLimit int) ([]*Message, error) {
	if !present || state.Removed || state.ReplacementFrom < 0 || state.ReplacementTo > len(messages) || state.ReplacementTo <= state.ReplacementFrom {
		return cloneMessages(messages), nil
	}
	if summaryLimit <= 0 {
		return nil, errors.New("Compaction summary limit must be positive")
	}
	if len(state.Summary) > summaryLimit {
		return nil, fmt.Errorf("%w: durable Compaction checkpoint is %d bytes and exceeds the target Agent summary limit %d", ErrContextLimit, len(state.Summary), summaryLimit)
	}
	result := make([]*Message, 0, len(messages)-(state.ReplacementTo-state.ReplacementFrom)+1)
	result = append(result, cloneMessages(messages[:state.ReplacementFrom])...)
	result = append(result, SystemMessage(renderContextFragment(ContextFragment{
		Source: "agent.compaction", Purpose: "replace compacted conversation history",
		Resource: state.ID, Revision: fmt.Sprintf("%d", state.Revision),
		Placement: ContextCompactionCheckpoint, Content: state.Summary, HardLimit: summaryLimit,
	})))
	result = append(result, cloneMessages(messages[state.ReplacementTo:])...)
	return result, nil
}

var _ runstate.StructuralEngine = (*definitionEngine)(nil)
