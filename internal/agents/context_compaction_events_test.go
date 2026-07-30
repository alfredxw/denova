package agents

import "testing"

func TestContextCompactionEventIncludesProjectedTokensAfter(t *testing.T) {
	result := ContextCompactionResult{
		TokensAfter:          120,
		ProjectedTokensAfter: 180,
		CacheIdentityStatus:  contextCompactionCacheIdentityExact,
		CacheUsageStatus:     contextCompactionCacheUsageZero,
		CacheMissReason:      contextCompactionCacheMissZero,
	}
	var emitted Event
	emitContextCompactionEvent(func(event Event) { emitted = event }, contextCompactionPhaseModelStep, "completed", result)
	data, ok := emitted.Data.(map[string]any)
	if !ok || data["projected_tokens_after"] != 180 {
		t.Fatalf("compaction metrics = %#v", emitted.Data)
	}
	if data["cache_identity_status"] != contextCompactionCacheIdentityExact ||
		data["cache_usage_status"] != contextCompactionCacheUsageZero ||
		data["cache_miss_reason"] != contextCompactionCacheMissZero {
		t.Fatalf("compaction cache attribution = %#v", emitted.Data)
	}
}
