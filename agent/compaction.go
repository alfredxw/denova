package agent

import (
	"context"
	"math"
	"time"
)

const (
	compactionCapability       = "agent.compaction"
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

type CompactionState struct {
	ID             string `json:"id"`
	Revision       uint64 `json:"revision"`
	SourceRevision string `json:"source_revision"`
	SourceHash     string `json:"source_hash"`
	Summary        string `json:"summary"`
	// TokenEstimate is the exact post-checkpoint provider-input estimate after
	// stable Context fragments, tool schemas, and reserves are re-applied.
	TokenEstimate int `json:"token_estimate,omitempty"`
	// SummaryTokenEstimate measures only the generated checkpoint body.
	SummaryTokenEstimate int               `json:"summary_token_estimate,omitempty"`
	Metrics              CompactionMetrics `json:"metrics,omitempty"`
	// CleanupRevisionAtCompaction is the newest Cleanup projection absorbed by
	// this structural boundary. It remains authoritative after removal so raw
	// rich history is restored without resurrecting an older placeholder view.
	CleanupRevisionAtCompaction uint64    `json:"cleanup_revision_at_compaction,omitempty"`
	ReplacementFrom             int       `json:"replacement_from"`
	ReplacementTo               int       `json:"replacement_to"`
	CreatedAt                   time.Time `json:"created_at"`
	Removed                     bool      `json:"removed,omitempty"`
	// ContextData is optional, product-neutral metadata used by a custom
	// ContextSource to apply this checkpoint to host-owned context. Agent keeps
	// it durable and opaque; it is never injected into the model automatically.
	ContextData *HostData `json:"context_data,omitempty"`
}

type CompactionPlanRequest struct {
	Session SessionView
	Run     RunView
	// Messages is the complete raw transcript. A CompactionManager is a trusted
	// structural capability, not a model-visibility sandbox: it needs raw
	// coordinates to choose a reversible replacement range.
	Messages []*Message
	// ModelRequest is the exact provider-visible request after caller
	// middleware. Managers must use it for pressure and provider projection;
	// they must not assume that every raw Message is model-visible.
	ModelRequest []*Message
	// ModelSnapshot is the exact final request for both automatic model-step and
	// explicit structural planning. Agent freezes it before Manager code runs.
	ModelSnapshot *ModelRequestSnapshot
	// LifecycleReservedTokens is the active capability reserve that must be
	// added to manager-owned reserves for trigger and validation calculations.
	LifecycleReservedTokens int
	Force                   bool
	Current                 CompactionState
	Present                 bool
}

type CompactionPlan struct {
	Action         CompactionAction
	SkippedReason  string
	SourceFrom     int
	SourceTo       int
	SourceRevision string
	SourceHash     string
	// Validation freezes the manager's provider-neutral health policy for this
	// exact plan. Agent owns measurement against the final reassembled request.
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

type CompactionCompactRequest struct {
	Session SessionView
	Run     RunView
	// Messages is the complete raw transcript retained by Agent for durable
	// range hashing and future checkpoint removal.
	Messages     []*Message
	ModelRequest []*Message
	// SourceMessages is the incremental semantic source supplied to the
	// checkpoint generator. Agent defaults it to the selected raw range; after
	// an earlier checkpoint it contains that checkpoint plus only the newly
	// selected raw tail. Caller middleware is intentionally not reverse-mapped
	// onto raw coordinates. A host that applies model-only visibility policy or
	// owns a rendered domain history must wrap the Manager and explicitly
	// project this field before invoking its Summarizer.
	SourceMessages []*Message
	// ModelSnapshot is the exact request after caller model middleware. Custom
	// managers may fork it to preserve provider/model/options/cache identity.
	// Agent supplies it on both automatic and explicit structural paths.
	ModelSnapshot *ModelRequestSnapshot
	Plan          CompactionPlan
	Current       CompactionState
	Present       bool
}

type CompactionCheckpoint struct {
	Summary       string
	TokenEstimate int
	ContextData   *HostData
}

// CompactionManager plans and generates checkpoints. It is a trusted context
// capability with deliberate access to the raw transcript; model visibility
// is represented separately by ModelRequest and, where required, a host-owned
// SourceMessages projection Adapter. Agent owns durable CAS, raw-history
// retention, effective markers, recovery, and Event publication.
type CompactionManager interface {
	Identity() CapabilityIdentity
	// SummaryLimitBytes is the maximum checkpoint size that this Agent can
	// inject as one context fragment. It must already be aligned with the
	// target Agent's fragment, aggregate-context, and provider-input limits.
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
