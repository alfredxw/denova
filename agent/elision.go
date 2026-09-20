package agent

import (
	"encoding/json"
	"errors"
	"fmt"
)

const (
	elisionCapability      = "agent.elision"
	maxElisionReplacements = 4096
)

// ElisionPolicy enables deterministic removal of stale, recoverable tool bodies
// before semantic Compaction. Nil disables new elisions; committed projections
// remain in force until Clear or canonical history replacement. Agent owns the
// policy's journal, recovery and exact provider-request validation.
// The newest two complete steps and a 30% context-window tail stay verbatim.
type ElisionPolicy struct {
	ContextWindowTokens int
	ReservedTokens      int
	// TriggerRatio defaults to .60. Hosts should keep it below their summary threshold.
	TriggerRatio float64
	// MinimumSavingsTokens defaults to max(256, 1% of the window), batching
	// changes instead of invalidating the prompt cache for small savings.
	MinimumSavingsTokens int
}

func normalizeElisionPolicy(policy *ElisionPolicy) (*ElisionPolicy, error) {
	if policy == nil {
		return nil, nil
	}
	copy := *policy
	if copy.TriggerRatio == 0 {
		copy.TriggerRatio = .60
	}
	if copy.ContextWindowTokens <= 0 || copy.ReservedTokens < 0 || copy.MinimumSavingsTokens < 0 ||
		!(copy.TriggerRatio > 0 && copy.TriggerRatio < 1) {
		return nil, errors.New("Elision requires a positive context window, nonnegative reserves and savings, and a trigger ratio between zero and one")
	}
	if copy.MinimumSavingsTokens == 0 {
		copy.MinimumSavingsTokens = max(256, copy.ContextWindowTokens/100)
	}
	return &copy, nil
}

// ElisionMetrics reports the last committed projection without exposing bodies.
type ElisionMetrics struct {
	TokensBefore              int `json:"tokens_before"`
	TokensAfter               int `json:"tokens_after"`
	ResultsElided             int `json:"results_elided"`
	CacheExpectedPrefixTokens int `json:"cache_expected_prefix_tokens"`
}

// Only coordinates and source fingerprints are durable. Placeholders are
// rendered from the original portable result using the record's schema version;
// host absolute paths and copied result bodies never enter this recovery record.
type elisionReplacement struct {
	MessageIndex int    `json:"message_index"`
	SourceHash   string `json:"source_hash"`
}

type elisionRecord struct {
	Version       uint16               `json:"version"`
	Revision      uint64               `json:"revision"`
	ClearRevision uint64               `json:"clear_revision,omitempty"`
	Replacements  []elisionReplacement `json:"replacements"`
	Metrics       ElisionMetrics       `json:"metrics"`
}

func elisionStateFrom(states map[string]json.RawMessage) (elisionRecord, error) {
	raw, present := states[elisionCapability]
	if !present {
		return elisionRecord{}, nil
	}
	var state elisionRecord
	if err := json.Unmarshal(raw, &state); err != nil {
		return state, fmt.Errorf("decode Elision state: %w", err)
	}
	clear, present, err := clearStateFrom(states)
	if err != nil {
		return elisionRecord{}, err
	}
	if present && state.ClearRevision < clear.Revision {
		return elisionRecord{}, nil
	}
	if state.Version != 1 || state.Revision == 0 || len(state.Replacements) == 0 || len(state.Replacements) > maxElisionReplacements {
		return state, errors.New("durable Elision state is invalid")
	}
	previous := -1
	for _, replacement := range state.Replacements {
		if replacement.MessageIndex <= previous || len(replacement.SourceHash) != 64 {
			return state, errors.New("durable Elision source coordinates are invalid")
		}
		previous = replacement.MessageIndex
	}
	return state, nil
}

func (state elisionRecord) project(messages []*Message) ([]*Message, error) {
	projected := cloneMessages(messages)
	for _, replacement := range state.Replacements {
		index := replacement.MessageIndex
		if index < 0 || index >= len(messages) {
			return nil, errors.New("Elision source is outside canonical history")
		}
		message := messages[index]
		fingerprint, err := hashCanonical(message)
		if err != nil || fingerprint != replacement.SourceHash {
			return nil, fmt.Errorf("Elision source changed at message %d", index)
		}
		stub := elisionPlaceholder(message)
		if stub == "" {
			return nil, fmt.Errorf("Elision source is no longer recoverable at message %d", index)
		}
		projected[index].Content = stub
	}
	return projected, nil
}

// elisionForHistory drops projections whose raw coordinates are already inside
// a Compaction replacement. The raw journal remains authoritative, so keeping
// those coordinates in the diagnostic record is safe; filtering them here also
// prevents an explicit Compaction removal from resurrecting an older placeholder.
func elisionForHistory(state elisionRecord, compaction compactionRecord, present bool) elisionRecord {
	if !present || compaction.ReplacementTo <= 0 || len(state.Replacements) == 0 {
		return state
	}
	filtered := state
	filtered.Replacements = make([]elisionReplacement, 0, len(state.Replacements))
	for _, replacement := range state.Replacements {
		if replacement.MessageIndex >= compaction.ReplacementTo {
			filtered.Replacements = append(filtered.Replacements, replacement)
		}
	}
	if len(filtered.Replacements) == 0 {
		filtered.Metrics = ElisionMetrics{}
	}
	return filtered
}

func effectiveHistoryMessages(messages []*Message, elision elisionRecord, compaction compactionRecord, present bool, summaryLimit int) ([]*Message, error) {
	projected, err := elisionForHistory(elision, compaction, present).project(messages)
	if err != nil {
		return nil, err
	}
	return effectiveCompactionMessages(projected, compaction, present, summaryLimit)
}
