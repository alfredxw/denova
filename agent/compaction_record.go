package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

type compactionRecord struct {
	// Version identifies the journal schema. Version 2 supports complete
	// assistant steps inside an unfinished Run.
	Version uint16 `json:"version,omitempty"`
	ID      string `json:"id"`
	// Revision increases when a checkpoint is committed or removed. It fences
	// stale requests and keeps checkpoints from before Clear inactive.
	Revision uint64 `json:"revision"`
	// SourceRevision labels the source snapshot for diagnostics; it is not
	// the checkpoint revision or a concurrency check.
	SourceRevision string `json:"source_revision"`
	SourceHash     string `json:"source_hash"`
	Summary        string `json:"summary"`
	// TokenEstimate is the exact post-checkpoint provider-input estimate after
	// stable Context fragments, tool schemas, and reserves are re-applied.
	TokenEstimate int `json:"token_estimate,omitempty"`
	// SummaryTokenEstimate measures only the generated checkpoint body.
	SummaryTokenEstimate int               `json:"summary_token_estimate,omitempty"`
	Metrics              CompactionMetrics `json:"metrics,omitempty"`
	ReplacementFrom      int               `json:"replacement_from"`
	ReplacementTo        int               `json:"replacement_to"`
	// RetainedUserFrom preserves verbatim user instructions from the active Run
	// inside the replaced prefix. Coordinates always refer to the raw journal;
	// completed tool groups may be summarized without erasing the task itself.
	RetainedUserFrom *int      `json:"retained_user_from,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
	Removed          bool      `json:"removed,omitempty"`
	// ContextData is optional, product-neutral metadata used by a custom
	// ContextSource to apply this checkpoint to host-owned context. Agent keeps
	// it durable and opaque; it is never injected into the model automatically.
	ContextData *HostData `json:"context_data,omitempty"`
}

func compactionStatePointer(state compactionRecord, present bool) *CompactionState {
	if !present || state.Removed {
		return nil
	}
	copy := state
	copy.ContextData = cloneHostData(state.ContextData)
	if state.RetainedUserFrom != nil {
		index := *state.RetainedUserFrom
		copy.RetainedUserFrom = &index
	}
	return &CompactionState{
		ID: state.ID, Revision: state.Revision, Summary: state.Summary, CreatedAt: state.CreatedAt,
		SourceMessageCount: state.ReplacementTo - state.ReplacementFrom,
		TokensBefore:       state.Metrics.ProjectedTokensBefore, TokensAfter: state.TokenEstimate,
		ContextData: cloneHostData(state.ContextData), projection: &copy,
	}
}

func cloneCompactionState(state *CompactionState) *CompactionState {
	if state == nil {
		return nil
	}
	copy := *state
	copy.ContextData = cloneHostData(state.ContextData)
	// The private projection is immutable and never exposed to callers.
	return &copy
}

func compactionStateFrom(states map[string]json.RawMessage) (compactionRecord, bool, error) {
	raw, present := states[compactionCapability]
	if !present {
		return compactionRecord{}, false, nil
	}
	state, err := decodeCompactionState(raw)
	return state, true, err
}

func decodeCompactionState(raw json.RawMessage) (compactionRecord, error) {
	var state compactionRecord
	if err := json.Unmarshal(raw, &state); err != nil {
		return compactionRecord{}, fmt.Errorf("decode Compaction state: %w", err)
	}
	if strings.TrimSpace(state.ID) == "" || state.Revision == 0 || state.ReplacementFrom < 0 ||
		state.ReplacementTo <= state.ReplacementFrom || strings.TrimSpace(state.Summary) == "" {
		return compactionRecord{}, errors.New("durable Compaction state is invalid")
	}
	if state.Version != 0 && state.Version != 2 {
		return compactionRecord{}, fmt.Errorf("unsupported Compaction state version %d", state.Version)
	}
	if state.RetainedUserFrom != nil && (*state.RetainedUserFrom < state.ReplacementFrom || *state.RetainedUserFrom >= state.ReplacementTo) {
		return compactionRecord{}, errors.New("durable Compaction retained user boundary is invalid")
	}
	if err := validateCompactionContextData(state.ContextData); err != nil {
		return compactionRecord{}, err
	}
	return state, nil
}

func compactionModelRequest(
	prepared preparedDefinition,
	messages []*Message,
	currentInput string,
	current compactionRecord,
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
		result = append(result, leadingContextMessages(prepared.fragments)...)
		result = append(result, effective...)
		return result, nil
	}
	cycle, _, err := assembleCycleMessages(effective, currentInput, nil, prepared.fragments, prepared.definition.AttachmentRoot)
	if err != nil {
		return nil, err
	}
	result = append(result, cycle...)
	return result, nil
}

func effectiveCompactionMessages(messages []*Message, state compactionRecord, present bool, summaryLimit int) ([]*Message, error) {
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
	result = append(result, compactionCheckpointMessage(state, summaryLimit))
	for index := state.ReplacementFrom; index < state.ReplacementTo; index++ {
		if compactionRetainsUser(messages, state, index) {
			result = append(result, messages[index].Clone())
		}
	}
	result = append(result, cloneMessages(messages[state.ReplacementTo:])...)
	return result, nil
}

func compactionRetainsUser(messages []*Message, state compactionRecord, index int) bool {
	return state.RetainedUserFrom != nil && index >= *state.RetainedUserFrom &&
		messages[index] != nil && messages[index].Role == User && !IsContextStateMessage(messages[index])
}

// compactionMessageIndex maps a surviving raw journal message to the effective
// transcript, including verbatim instructions preserved inside a checkpoint.
func compactionMessageIndex(messages []*Message, state compactionRecord, present bool, index int) int {
	if !present || state.Removed || index < state.ReplacementFrom {
		return index
	}
	projected := state.ReplacementFrom + 1
	for cursor := state.ReplacementFrom; cursor < min(index, state.ReplacementTo); cursor++ {
		if compactionRetainsUser(messages, state, cursor) {
			projected++
		}
	}
	if index < state.ReplacementTo {
		if !compactionRetainsUser(messages, state, index) {
			return -1
		}
		return projected
	}
	return projected + index - state.ReplacementTo
}

func compactionIncrementalSource(
	messages []*Message,
	plan compactionExecutionPlan,
	current compactionRecord,
	present bool,
	summaryLimit int,
) []*Message {
	if present && !current.Removed && current.ReplacementFrom == plan.SourceFrom &&
		current.ReplacementTo >= plan.SourceFrom && current.ReplacementTo <= plan.SourceTo {
		result := []*Message{compactionCheckpointMessage(current, summaryLimit)}
		return append(result, cloneMessages(messages[current.ReplacementTo:plan.SourceTo])...)
	}
	return cloneMessages(messages[plan.SourceFrom:plan.SourceTo])
}

func compactionCheckpointMessage(state compactionRecord, summaryLimit int) *Message {
	return SystemMessage(renderContextFragment(ContextFragment{
		Source: "agent.compaction", Purpose: "replace compacted conversation history",
		Resource: state.ID, Revision: fmt.Sprintf("%d", state.Revision),
		Stability: ContextCheckpoint, Placement: ContextCompactionCheckpoint, Content: state.Summary, HardLimit: summaryLimit,
	}))
}
