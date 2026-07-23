package agent

func emitContextCompactionEvent(emit func(Event), phase, status string, result ContextCompactionResult) {
	if emit == nil {
		return
	}
	emit(Event{Type: "context_compaction", Data: map[string]any{
		"phase":                       phase,
		"status":                      status,
		"tokens_before":               result.TokensBefore,
		"projected_tokens_before":     result.ProjectedTokensBefore,
		"reserved_completion_tokens":  result.ReservedCompletionTokens,
		"reserved_tool_result_tokens": result.ReservedToolResultTokens,
		"tokens_after":                result.TokensAfter,
		"context_window_tokens":       result.ContextWindowTokens,
		"strategy":                    result.Strategy,
		"threshold":                   result.Threshold,
		"target_ratio":                result.TargetRatio,
		"epoch":                       result.Epoch,
		"source_message_count":        result.SourceMessageCount,
		"message_count_before":        result.MessageCountBefore,
		"message_count_after":         result.MessageCountAfter,
		"skipped_reason":              result.SkippedReason,
		"summary":                     result.Summary,
	}})
}

func emitContextCompactionDeltaEvent(emit func(Event), phase string, result ContextCompactionResult, attempt int, delta string) {
	if emit == nil || delta == "" {
		return
	}
	emit(Event{Type: "context_compaction", Data: map[string]any{
		"phase":                       phase,
		"status":                      "delta",
		"attempt":                     attempt,
		"delta":                       delta,
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
