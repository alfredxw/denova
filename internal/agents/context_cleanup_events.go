package agents

func emitContextCleanupEvent(emit func(Event), status string, decision ContextPressureDecision, actualReclaimed int, err error) {
	if emit == nil {
		return
	}
	eagerApplied := 0
	if status == "completed" && decision.Action == ContextMaintenanceCleanup {
		eagerApplied = decision.Cleanup.EagerGroupCount
	}
	data := map[string]any{
		"phase":                               contextCompactionPhaseModelStep,
		"status":                              status,
		"action":                              decision.Action,
		"trigger_reason":                      decision.Reason,
		"pressure_scope":                      decision.Scope,
		"pressure":                            decision.Pressure,
		"full_pressure":                       decision.FullPressure,
		"local_projected_tokens":              decision.LocalProjectedTokens,
		"observed_prompt_tokens":              decision.ObservedPromptTokens,
		"effective_tokens":                    decision.EffectiveTokens,
		"stable_prefix_tokens":                decision.StablePrefixTokens,
		"candidate_tokens":                    decision.CandidateTokens,
		"cache_viable_candidate_tokens":       decision.CacheViableCandidateTokens,
		"cleanup_skipped_below_minimum_count": decision.CleanupSkippedBelowMinimumCount,
		"cleanup_skipped_warm_suffix_count":   decision.CleanupSkippedWarmSuffixCount,
		"eager_receipt_candidate_count":       decision.EagerCandidateCount,
		"eager_receipt_applied_count":         eagerApplied,
		"eager_receipt_fallback_count":        max(0, decision.EagerCandidateCount-eagerApplied),
		"superseded_candidate_count":          decision.SupersededCount,
		"discardable_candidate_count":         decision.DiscardableCount,
		"minimum_cleanup_tokens":              decision.MinimumCleanupTokens,
		"protected_result_count":              decision.ProtectedResultCount,
		"estimated_reclaimed_tokens":          decision.Cleanup.ReclaimedTokens,
		"actual_reclaimed_tokens":             actualReclaimed,
		"projected_tokens_after":              decision.Cleanup.ProjectedTokensAfter,
		"pressure_after":                      decision.Cleanup.PressureAfter,
		"full_pressure_after":                 decision.Cleanup.FullPressureAfter,
		"earliest_changed_index":              decision.Cleanup.EarliestChanged,
		"warm_suffix_tokens":                  decision.Cleanup.WarmSuffixTokens,
		"placeholder_tokens":                  decision.Cleanup.PlaceholderTokens,
		"replacement_count":                   len(decision.Cleanup.Replacements),
		"eager_only":                          decision.Cleanup.EagerOnly,
		"provider_cache_state":                decision.ProviderCacheState,
		"cleanup_execution_mode":              decision.CleanupExecutionMode,
		"placeholder_renderer_version":        decision.Cleanup.RendererVersion,
	}
	if err != nil {
		// The live UI receives the actionable diagnostic. RunLedger converts it
		// to error_class and never persists this raw text.
		data["error"] = err.Error()
	}
	emit(Event{Type: "context_cleanup", Data: data})
}
