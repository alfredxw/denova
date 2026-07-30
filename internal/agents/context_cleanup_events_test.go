package agents

import "testing"

func TestCleanupEventCountsEagerInteractionGroupsInsteadOfResultMessages(t *testing.T) {
	decision := ContextPressureDecision{
		Action: ContextMaintenanceCleanup, EagerCandidateCount: 3,
		CacheViableCandidateTokens: 17, CleanupSkippedBelowMinimumCount: 1, CleanupSkippedWarmSuffixCount: 2,
		Cleanup: ToolResultCleanupPlan{
			EagerGroupCount: 2,
			Replacements:    []ToolResultCleanupReplacement{{MessageIndex: 1}, {MessageIndex: 2}, {MessageIndex: 3}},
		},
	}
	var emitted Event
	emitContextCleanupEvent(func(event Event) { emitted = event }, "completed", decision, 10, nil)
	data, ok := emitted.Data.(map[string]any)
	if !ok || data["eager_receipt_applied_count"] != 2 || data["eager_receipt_fallback_count"] != 1 ||
		data["cache_viable_candidate_tokens"] != 17 || data["cleanup_skipped_below_minimum_count"] != 1 ||
		data["cleanup_skipped_warm_suffix_count"] != 2 {
		t.Fatalf("eager cleanup metrics use incompatible units: %#v", emitted.Data)
	}
}
