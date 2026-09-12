package agent

import (
	"context"
	"errors"
	"math"
	"time"
)

const (
	// CompactionCapability identifies the durable Compaction projection in a
	// Session journal. It is exported for hosts that must migrate a released
	// canonical journal into Agent's current capability schema.
	CompactionCapability       = "agent.compaction"
	compactionCapability       = CompactionCapability
	compactionHealthCapability = "agent.compaction_health"

	defaultMaxAutomaticCompactionFailures = 3
)

type CompactionAction string

const (
	CompactionNone   CompactionAction = "none"
	CompactionCreate CompactionAction = "compact"
)

type CompactionRequest struct {
	Force            bool
	IdempotencyKey   string
	ExpectedID       string
	ExpectedRevision uint64
}

type CompactionRemoveRequest struct {
	ID               string
	ExpectedRevision uint64
	IdempotencyKey   string
}

// CompactionState is a detached view of the active checkpoint. Coverage and
// journal schema are private. Obtain it from a Session or a Context request;
// JSON round trips preserve display data, but not history projection authority.
type CompactionState struct {
	ID                 string    `json:"id"`
	Revision           uint64    `json:"revision"`
	Summary            string    `json:"summary"`
	CreatedAt          time.Time `json:"created_at"`
	SourceMessageCount int       `json:"source_message_count"`
	TokensBefore       int       `json:"tokens_before"`
	TokensAfter        int       `json:"tokens_after"`
	ContextData        *HostData `json:"context_data,omitempty"`
	projection         *compactionRecord
}

// Project returns the checkpoint and surviving messages without changing raw
// history. Only a runtime-issued view carries authenticated coverage.
func (state CompactionState) Project(messages []*Message, summaryLimit int) ([]*Message, error) {
	if state.projection == nil {
		return nil, errors.New("Compaction projection requires a runtime-issued state")
	}
	return effectiveCompactionMessages(messages, *state.projection, true, summaryLimit)
}

// CompactionGroup is one completed assistant step with all of its tool results.
// Agent supplies detached, runtime-resolved messages in journal order. The newest
// complete step and any incomplete suffix are never offered for replacement.
type CompactionGroup struct {
	Messages []*Message
}

type CompactionPlanRequest struct {
	Session SessionView
	Run     RunView
	Groups  []CompactionGroup
	// RetainedBytes measures the protected suffix, for policies retaining more history.
	RetainedBytes           int
	ModelSnapshot           *ModelRequestSnapshot
	LifecycleReservedTokens int
	Force                   bool
	Current                 *CompactionState
}

type CompactionPlan struct {
	Action        CompactionAction
	SkippedReason string
	// GroupCount selects a nonempty prefix of the offered Groups. Agent maps
	// it to journal positions; a policy cannot split a step or rewrite history.
	GroupCount int
	Validation CompactionValidationPolicy
	Metrics    CompactionMetrics
}

// CompactionValidationPolicy defines the fixed post-checkpoint safety band.
// Managers choose policy; Agent measures and enforces it only after rebuilding
// the exact provider request. Zero ContextWindowTokens keeps only the mandatory
// no-progress and byte-limit checks, which is useful for custom providers that
// do not expose a token window.
type CompactionValidationPolicy struct {
	ContextWindowTokens int     `json:"context_window_tokens,omitempty"`
	ReservedTokens      int     `json:"reserved_tokens,omitempty"`
	Threshold           float64 `json:"threshold,omitempty"`
	RecoveryBand        float64 `json:"recovery_band,omitempty"`
	MinimumChangeTokens int     `json:"minimum_change_tokens,omitempty"`
	HardLimitBytes      int     `json:"hard_limit_bytes,omitempty"`
}

// CompactionMetrics is a provider-neutral lifecycle vocabulary. It contains
// no model/provider implementation types and no checkpoint or tool-result
// bodies, so it is safe for public events and durable diagnostics.
type CompactionMetrics struct {
	EstimatedTokensBefore int `json:"estimated_tokens_before,omitempty"`
	ObservedPromptTokens  int `json:"observed_prompt_tokens,omitempty"`
	// ObservedEstimateTokens is the local estimate for the exact request whose
	// provider usage produced ObservedPromptTokens. The pair calibrates current
	// before/after projections without treating stale usage as current context.
	ObservedEstimateTokens    int     `json:"observed_estimate_tokens,omitempty"`
	EstimatedTokensAfter      int     `json:"estimated_tokens_after,omitempty"`
	ProjectedTokensBefore     int     `json:"projected_tokens_before,omitempty"`
	ProjectedTokensAfter      int     `json:"projected_tokens_after,omitempty"`
	ReservedTokens            int     `json:"reserved_tokens,omitempty"`
	ContextWindowTokens       int     `json:"context_window_tokens,omitempty"`
	Threshold                 float64 `json:"threshold,omitempty"`
	RecoveryBand              float64 `json:"recovery_band,omitempty"`
	RecoveryTargetTokens      int     `json:"recovery_target_tokens,omitempty"`
	RecoveryBandMet           bool    `json:"recovery_band_met,omitempty"`
	Degraded                  bool    `json:"degraded,omitempty"`
	StablePrefixTokens        int     `json:"stable_prefix_tokens,omitempty"`
	SourceMessageCount        int     `json:"source_message_count,omitempty"`
	MessageCountBefore        int     `json:"message_count_before,omitempty"`
	MessageCountAfter         int     `json:"message_count_after,omitempty"`
	CacheExpectedPrefixTokens int     `json:"cache_expected_prefix_tokens,omitempty"`
	CacheReadTokens           int     `json:"cache_read_tokens,omitempty"`
	CandidateFingerprint      string  `json:"candidate_fingerprint,omitempty"`
	CandidateGeneration       uint64  `json:"candidate_generation,omitempty"`
}

// CalibratedTokens applies provider/local calibration measured on the exact
// previous request. The provider ratio may raise a conservative local
// estimate, but never lower it. Outlier ratios fail closed to the local
// estimate.
func (metrics CompactionMetrics) CalibratedTokens(estimated int) int {
	estimated = max(1, estimated)
	if metrics.ObservedPromptTokens <= 0 || metrics.ObservedEstimateTokens <= 0 {
		return estimated
	}
	ratio := float64(metrics.ObservedPromptTokens) / float64(metrics.ObservedEstimateTokens)
	if ratio < .25 || ratio > 4 {
		return estimated
	}
	return max(estimated, int(math.Round(float64(estimated)*ratio)))
}

// CompactionCompactRequest contains only the selected incremental source: the
// previous checkpoint followed by newly covered messages. Hosts with model-only
// visibility policies must apply the same policy to Messages before delegation.
type CompactionCompactRequest struct {
	Session       SessionView
	Run           RunView
	Messages      []*Message
	ModelSnapshot *ModelRequestSnapshot
	Current       *CompactionState
}

// CompactionCheckpoint is the semantic result of either extension point. Agent
// estimates its size and atomically persists ContextData with the checkpoint.
// ContextData requires a type, version, valid JSON and at most 8 MiB; it is never
// automatically injected into the model.
type CompactionCheckpoint struct {
	Summary     string
	ContextData *HostData
}

// CompactionManager chooses when and how much to compact, then generates the
// checkpoint. Agent owns grouping, active intent, final request validation,
// journal coverage, revision, cancellation and recovery for every implementation.
type CompactionManager interface {
	Identity() CapabilityIdentity
	SummaryLimitBytes() int
	Plan(context.Context, CompactionPlanRequest) (CompactionPlan, error)
	Compact(context.Context, CompactionCompactRequest) (CompactionCheckpoint, error)
}

type CompactionResult struct {
	Changed bool
	State   CompactionState
}

type compactionHealthState struct {
	Fingerprint         string `json:"fingerprint"`
	ConsecutiveFailures int    `json:"consecutive_failures"`
	FailureCode         string `json:"failure_code,omitempty"`
}

func normalizedAutomaticCompactionFailureLimit(policy ExecutionPolicy) int {
	if policy.MaxAutomaticCompactionFailures > 0 {
		return policy.MaxAutomaticCompactionFailures
	}
	return defaultMaxAutomaticCompactionFailures
}
