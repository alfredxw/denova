package compaction

// Event is a bounded context-maintenance signal. Runtime adapters translate it
// into their transport event without coupling this package to chat delivery.
type Event struct {
	Type string
	Data any
}

func emitContextCompactionEvent(emit func(Event), phase, status string, result Result) {
	if emit == nil {
		return
	}
	cacheHitRatio := float64(0)
	if result.CacheExpectedPrefixTokens > 0 {
		cacheHitRatio = float64(result.CacheReadTokens) / float64(result.CacheExpectedPrefixTokens)
	}
	// Summary is transient UI data. RunLedger applies its own durable whitelist
	// and deliberately excludes checkpoint text from the trace file.
	emit(Event{Type: "context_compaction", Data: map[string]any{
		"phase":                        phase,
		"status":                       status,
		"estimated_tokens_before":      result.EstimatedTokensBefore,
		"observed_prompt_tokens":       result.ObservedPromptTokens,
		"observed_estimate_tokens":     result.ObservedEstimateTokens,
		"tokens_before":                result.TokensBefore,
		"projected_tokens_before":      result.ProjectedTokensBefore,
		"reserved_completion_tokens":   result.ReservedCompletionTokens,
		"reserved_tool_result_tokens":  result.ReservedToolResultTokens,
		"tokens_after":                 result.TokensAfter,
		"projected_tokens_after":       result.ProjectedTokensAfter,
		"context_window_tokens":        result.ContextWindowTokens,
		"strategy":                     result.Strategy,
		"threshold":                    result.Threshold,
		"trigger_reason":               result.TriggerReason,
		"recovery_band":                result.RecoveryBand,
		"recovery_target_tokens":       result.RecoveryTargetTokens,
		"recovery_band_met":            result.RecoveryBandMet,
		"degraded":                     result.Degraded,
		"target_ratio":                 result.TargetRatio,
		"epoch":                        result.Epoch,
		"source_message_count":         result.SourceMessageCount,
		"message_count_before":         result.MessageCountBefore,
		"message_count_after":          result.MessageCountAfter,
		"skipped_reason":               result.SkippedReason,
		"execution_mode":               result.ExecutionMode,
		"fallback_reason":              result.FallbackReason,
		"compaction_input_tokens":      result.CompactionInputTokens,
		"compaction_prompt_tokens":     result.CompactionPromptTokens,
		"checkpoint_output_reserve":    result.CheckpointOutputReserve,
		"safety_margin_tokens":         result.SafetyMarginTokens,
		"cache_expected_prefix_tokens": result.CacheExpectedPrefixTokens,
		"cache_read_tokens":            result.CacheReadTokens,
		"cache_write_tokens":           result.CacheWriteTokens,
		"cache_write_tokens_known":     result.CacheWriteTokensKnown,
		"cache_identity_status":        result.CacheIdentityStatus,
		"cache_usage_status":           result.CacheUsageStatus,
		"cache_miss_reason":            result.CacheMissReason,
		"cache_hit_ratio":              cacheHitRatio,
		"layer_count":                  result.LayerCount,
		"consecutive_failures":         result.ConsecutiveFailures,
		"failure_fuse_open":            result.FailureFuseOpen,
		"summary":                      result.Summary,
	}})
}

func emitContextCompactionDeltaEvent(emit func(Event), phase string, result Result, attempt int, delta string) {
	if emit == nil || delta == "" {
		return
	}
	// Delta events remain available to the live UI but are skipped entirely by
	// durable telemetry so partial checkpoint text can never reach RunLedger.
	emit(Event{Type: "context_compaction", Data: map[string]any{
		"phase":                       phase,
		"status":                      "delta",
		"attempt":                     attempt,
		"delta":                       delta,
		"estimated_tokens_before":     result.EstimatedTokensBefore,
		"observed_prompt_tokens":      result.ObservedPromptTokens,
		"observed_estimate_tokens":    result.ObservedEstimateTokens,
		"tokens_before":               result.TokensBefore,
		"projected_tokens_before":     result.ProjectedTokensBefore,
		"reserved_completion_tokens":  result.ReservedCompletionTokens,
		"reserved_tool_result_tokens": result.ReservedToolResultTokens,
		"context_window_tokens":       result.ContextWindowTokens,
		"strategy":                    result.Strategy,
		"threshold":                   result.Threshold,
		"message_count_before":        result.MessageCountBefore,
	}})
}
