// Package context owns Denova's model-context assembly and compaction
// projection vocabulary. Durable maintenance state belongs to public Agent
// Sessions; product stores may only retain read-only display projections.
package context

// CompactionCheckpoint is Denova's read-only projection of the public Agent
// compaction state. Product adapters may render it, but the Agent Session is
// the sole authority for mutation, recovery, and durable identity.
type CompactionCheckpoint struct {
	AgentKind              string  `json:"agent_kind,omitempty"`
	Epoch                  int     `json:"epoch"`
	Summary                string  `json:"summary"`
	RetainedTurns          int     `json:"retained_turns"`
	EstimatedTokensBefore  int     `json:"estimated_tokens_before,omitempty"`
	ObservedPromptTokens   int     `json:"observed_prompt_tokens,omitempty"`
	ObservedEstimateTokens int     `json:"observed_estimate_tokens,omitempty"`
	TokensBefore           int     `json:"tokens_before"`
	TokensAfter            int     `json:"tokens_after"`
	TargetRatio            float64 `json:"target_ratio,omitempty"`
	ContextWindowTokens    int     `json:"context_window_tokens"`
	Strategy               string  `json:"strategy,omitempty"`
	Threshold              float64 `json:"threshold"`
	TriggerReason          string  `json:"reason,omitempty"`
	Phase                  string  `json:"phase,omitempty"`
	RecoveryBand           float64 `json:"recovery_band,omitempty"`
	CandidateFingerprint   string  `json:"candidate_fingerprint,omitempty"`
	CandidateGeneration    uint64  `json:"candidate_generation,omitempty"`
}
