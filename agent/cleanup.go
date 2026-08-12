package agent

import (
	"context"
	"time"
)

const cleanupCapability = "agent.cleanup"

// CleanupAction is the planner's recommendation at one exact model seam.
// CleanupProject selects a reversible tool-result projection. CleanupCompact
// asks the fixed Agent lifecycle to try checkpoint compaction instead; an
// optional projection may still be applied transiently to recover a request
// that already exceeds the provider window.
type CleanupAction string

const (
	CleanupNone    CleanupAction = "none"
	CleanupProject CleanupAction = "project"
	CleanupCompact CleanupAction = "compact"
)

// CleanupReplacement is one deterministic substitution in the exact model
// request supplied to CleanupManager.Plan. Agent resolves it back to the raw
// transcript by occurrence, because provider tool-call IDs are only
// batch-local and may be reused.
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

// CleanupState is the Agent-owned durable projection over an immutable raw
// transcript prefix. SourceHash authenticates Messages[:SourceEnd]; later
// messages may be appended without invalidating the projection.
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

type CleanupPlanRequest struct {
	Session SessionView
	Run     RunView
	// Messages is the raw durable transcript at the beginning of this cycle.
	Messages []*Message
	// ModelRequest is the exact provider-visible request after caller
	// middleware and any already-active Cleanup/Compaction projection.
	ModelRequest []*Message
	// ModelInspection is the detached, non-executable metadata for that exact
	// request. Cleanup is a pure planner and must not receive provider authority
	// merely to inspect tool schemas or the authenticated stable-prefix bound.
	ModelInspection ModelRequestInspection
	Current         CleanupState
	Present         bool
	// CompactionAvailable lets one coordinated planner select the only durable
	// maintenance mutation for this run without learning about persistence.
	CompactionAvailable bool
}

type CleanupPlan struct {
	Action       CleanupAction
	Reason       string
	Replacements []CleanupReplacement
	Renderer     string
	Metrics      CleanupMetrics
	// FallbackToCompaction preserves the pressure decision even when planning
	// or projection fails. Agent may then checkpoint at the same model seam;
	// callers do not need persistence access to coordinate the fallback.
	FallbackToCompaction bool
}

// CleanupManager is deliberately a pure planning boundary. Agent owns raw
// history, target resolution, CAS, atomic final settlement, replay, and event
// publication; custom managers only choose safe replacements and wording.
type CleanupManager interface {
	Identity() CapabilityIdentity
	Plan(context.Context, CleanupPlanRequest) (CleanupPlan, error)
}

func cloneCleanupState(state *CleanupState) *CleanupState {
	if state == nil {
		return nil
	}
	cloned := *state
	cloned.Replacements = append([]CleanupReplacement(nil), state.Replacements...)
	return &cloned
}
