package agent

import (
	"time"
)

// These types decode historical events from the latest release. Cleanup no
// longer participates in Definition, model preparation, or Session recovery.
const cleanupCapability = "agent.cleanup"

// CleanupReplacement describes a substitution recorded by v0.4.5.
type CleanupReplacement struct {
	MessageIndex      int    `json:"message_index"`
	ToolCallID        string `json:"tool_call_id"`
	Placeholder       string `json:"placeholder"`
	OriginalTokens    int    `json:"original_tokens,omitempty"`
	PlaceholderTokens int    `json:"placeholder_tokens,omitempty"`
}

// CleanupMetrics are provider-neutral planning and projection measurements.
// They are durable diagnostics, never inputs to replay.
type CleanupMetrics struct {
	EstimatedTokensBefore      int     `json:"estimated_tokens_before,omitempty"`
	LocalProjectedTokens       int     `json:"local_projected_tokens,omitempty"`
	ObservedPromptTokens       int     `json:"observed_prompt_tokens,omitempty"`
	EffectiveTokens            int     `json:"effective_tokens,omitempty"`
	EstimatedTokensAfter       int     `json:"estimated_tokens_after,omitempty"`
	ReclaimedTokens            int     `json:"reclaimed_tokens,omitempty"`
	ContextWindowTokens        int     `json:"context_window_tokens,omitempty"`
	PressureBefore             float64 `json:"pressure_before,omitempty"`
	PressureAfter              float64 `json:"pressure_after,omitempty"`
	BodyPressureBefore         float64 `json:"body_pressure_before,omitempty"`
	BodyPressureAfter          float64 `json:"body_pressure_after,omitempty"`
	StablePrefixTokens         int     `json:"stable_prefix_tokens,omitempty"`
	CandidateTokens            int     `json:"candidate_tokens,omitempty"`
	CacheViableCandidateTokens int     `json:"cache_viable_candidate_tokens,omitempty"`
	SkippedBelowMinimumCount   int     `json:"skipped_below_minimum_count,omitempty"`
	SkippedWarmSuffixCount     int     `json:"skipped_warm_suffix_count,omitempty"`
	EagerCandidateCount        int     `json:"eager_candidate_count,omitempty"`
	EagerSelectedCount         int     `json:"eager_selected_count,omitempty"`
	SupersededCandidateCount   int     `json:"superseded_candidate_count,omitempty"`
	DiscardableCandidateCount  int     `json:"discardable_candidate_count,omitempty"`
	MinimumCleanupTokens       int     `json:"minimum_cleanup_tokens,omitempty"`
	ProtectedResults           int     `json:"protected_results,omitempty"`
	EarliestChanged            int     `json:"earliest_changed,omitempty"`
	WarmSuffixTokens           int     `json:"warm_suffix_tokens,omitempty"`
	PlaceholderTokens          int     `json:"placeholder_tokens,omitempty"`
	ReplacementCount           int     `json:"replacement_count,omitempty"`
	EagerOnly                  bool    `json:"eager_only,omitempty"`
	PressureScope              string  `json:"pressure_scope,omitempty"`
	ProviderCacheState         string  `json:"provider_cache_state,omitempty"`
	ExecutionMode              string  `json:"execution_mode,omitempty"`
	RendererVersion            string  `json:"renderer_version,omitempty"`
}

// CleanupState is retained only to decode and display historical events.
// New execution ignores this former independent projection.
type CleanupState struct {
	ID             string               `json:"id"`
	Revision       uint64               `json:"revision"`
	SourceRevision string               `json:"source_revision"`
	SourceHash     string               `json:"source_hash"`
	SourceStart    int                  `json:"source_start"`
	SourceEnd      int                  `json:"source_end"`
	Replacements   []CleanupReplacement `json:"replacements"`
	Renderer       string               `json:"renderer"`
	Metrics        CleanupMetrics       `json:"metrics,omitempty"`
	CreatedAt      time.Time            `json:"created_at"`
	UpdatedAt      time.Time            `json:"updated_at"`
	Removed        bool                 `json:"removed,omitempty"`
}

func cloneCleanupState(state *CleanupState) *CleanupState {
	if state == nil {
		return nil
	}
	cloned := *state
	cloned.Replacements = append([]CleanupReplacement(nil), state.Replacements...)
	return &cloned
}
