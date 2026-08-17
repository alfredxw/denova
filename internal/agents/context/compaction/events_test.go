package compaction

import "testing"

func TestContextCompactionEventIncludesProjectedTokensAfter(t *testing.T) {
	result := Result{
		TokensAfter:          120,
		ProjectedTokensAfter: 180,
		CacheIdentityStatus:  CacheIdentityExact,
		CacheUsageStatus:     CacheUsageZero,
		CacheMissReason:      CacheMissZero,
	}
	var emitted Event
	emitContextCompactionEvent(func(event Event) { emitted = event }, PhaseModelStep, "completed", result)
	data, ok := emitted.Data.(map[string]any)
	if !ok || data["projected_tokens_after"] != 180 {
		t.Fatalf("compaction metrics = %#v", emitted.Data)
	}
	if data["cache_identity_status"] != CacheIdentityExact ||
		data["cache_usage_status"] != CacheUsageZero ||
		data["cache_miss_reason"] != CacheMissZero {
		t.Fatalf("compaction cache attribution = %#v", emitted.Data)
	}
}
