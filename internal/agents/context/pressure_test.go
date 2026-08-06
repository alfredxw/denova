package context

import (
	"fmt"
	"strings"
	"testing"

	agent "github.com/alfredxw/denova/agent"
	agentcontext "github.com/alfredxw/denova/agent/context"

	"denova/internal/agents/toolresult"
)

func TestContextPressureKeepsRichResultsBelowThreshold(t *testing.T) {
	messages := pressureHistory(6, 1800, agent.ToolResultDeferred, agent.ToolResultContextNormal)
	decision := PlanContextPressure(messages, nil, pressureTestPolicy(100_000))
	if decision.Action != ContextMaintenanceNone || decision.Reason != "below_cleanup_threshold" {
		t.Fatalf("decision = %#v", decision)
	}
	if got := messages[3].Content; !strings.Contains(got, "rich-result-0") {
		t.Fatalf("planner mutated rich history: %q", got)
	}
}

func TestProviderCacheStateDoesNotTreatMissingCacheUsageAsCold(t *testing.T) {
	message := agent.AssistantMessage("ok", nil)
	message.ResponseMeta = &agent.ResponseMeta{Usage: &agent.TokenUsage{PromptTokens: 10_000}}
	if got := ProviderCacheStateFromMessages([]*agent.Message{message}); got != ProviderCacheUnknown {
		t.Fatalf("cache state = %q, want conservative unknown", got)
	}
	message.ResponseMeta.Usage.PromptTokenDetails.CachedTokens = 8_000
	if got := ProviderCacheStateFromMessages([]*agent.Message{message}); got != ProviderCacheWarm {
		t.Fatalf("cache state = %q, want warm", got)
	}
}

func TestContextPressurePolicyMergesDurableProviderUsageConservatively(t *testing.T) {
	policy := pressureTestPolicy(100_000)
	policy.ObservedPromptTokens = 10_000
	policy.ProviderCacheState = ProviderCacheUnknown
	policy = policy.ObservePromptUsage(75_000, 50_000)
	if policy.ObservedPromptTokens != 75_000 || policy.ProviderCacheState != ProviderCacheWarm {
		t.Fatalf("observed policy = %#v", policy)
	}
	policy = policy.ObservePromptUsage(5_000, 0)
	if policy.ObservedPromptTokens != 75_000 || policy.ProviderCacheState != ProviderCacheWarm {
		t.Fatalf("smaller ambiguous usage replaced calibration: %#v", policy)
	}
}

func TestContextPressureProtectsErrorsAndRecentGroups(t *testing.T) {
	messages := pressureHistory(8, 7000, agent.ToolResultDeferred, agent.ToolResultContextNormal)
	messages[3].ToolResult.Status = agent.ToolResultError
	policy := pressureTestPolicy(120_000)
	policy.ProviderCacheState = ProviderCacheCold
	decision := PlanContextPressure(messages, nil, policy)
	if decision.ProtectedResultCount == 0 {
		t.Fatalf("error result was not protected: %#v", decision)
	}
	if decision.Action != ContextMaintenanceCleanup {
		t.Fatalf("normal pressure should clean only eligible older results: %#v", decision)
	}
	for _, replacement := range decision.Cleanup.Replacements {
		if replacement.MessageIndex == 3 || replacement.MessageIndex >= len(messages)-11 {
			t.Fatalf("protected/recent result selected: %#v", replacement)
		}
	}
}

func TestContextPressureCleansConsumedLargeResultWithoutRewritingCurrentUserInput(t *testing.T) {
	large := pressureToolResult(
		"large", strings.Repeat("large recoverable output ", 20_000),
		agent.ToolResultDeferred, agent.ToolResultContextNormal,
	)
	small := pressureToolResult(
		"follow-up", "small unconsumed result",
		agent.ToolResultDeferred, agent.ToolResultContextNormal,
	)
	messages := []*agent.Message{
		agent.UserMessage("current user request must remain exact"),
		agent.AssistantMessage("", []agent.ToolCall{pressureCall("large", 0)}),
		large,
		agent.AssistantMessage("result consumed; inspect one follow-up", []agent.ToolCall{pressureCall("follow-up", 1)}),
		small,
	}
	policy := pressureTestPolicy(80_000)
	policy.ProviderCacheState = ProviderCacheCold

	decision := PlanContextPressure(messages, nil, policy)
	if decision.Action != ContextMaintenanceCleanup {
		t.Fatalf("consumed recoverable result should be cleaned before whole-turn compaction: %#v", decision)
	}
	if len(decision.Cleanup.Replacements) != 1 || decision.Cleanup.Replacements[0].ToolCallID != "large" {
		t.Fatalf("cleanup targets = %#v, want only consumed large result", decision.Cleanup.Replacements)
	}
	projected := toolresult.ApplyCleanupPlan(messages, decision.Cleanup)
	if projected[0].Content != messages[0].Content || projected[3].Content != messages[3].Content {
		t.Fatal("tool-result cleanup rewrote user or assistant content")
	}
	if projected[2].Content == messages[2].Content || projected[4].Content != messages[4].Content {
		t.Fatal("cleanup did not isolate the consumed tool-result body")
	}
}

func TestContextPressureKeepsLargeResultUntilOneLaterAssistantStepConsumesIt(t *testing.T) {
	result := pressureToolResult(
		"pending", strings.Repeat("pending evidence ", 30_000),
		agent.ToolResultDeferred, agent.ToolResultContextNormal,
	)
	messages := []*agent.Message{
		agent.UserMessage("analyze this"),
		agent.AssistantMessage("", []agent.ToolCall{pressureCall("pending", 0)}),
		result,
	}
	policy := pressureTestPolicy(80_000)
	policy.ProviderCacheState = ProviderCacheCold

	decision := PlanContextPressure(messages, nil, policy)
	if decision.Action == ContextMaintenanceCleanup || len(decision.Cleanup.Replacements) != 0 {
		t.Fatalf("unconsumed tool result must stay rich: %#v", decision)
	}
}

func TestContextPressureCleanupRequiresCacheGateAndRecoveryTarget(t *testing.T) {
	messages := pressureHistory(10, 9000, agent.ToolResultDeferred, agent.ToolResultContextDiscardable)
	messages = append(messages, agent.UserMessage(strings.Repeat("warm-suffix ", 12_000)))
	policy := pressureTestPolicy(240_000)
	policy.ProviderCacheState = ProviderCacheUnknown
	decision := PlanContextPressure(messages, nil, policy)
	if decision.Action != ContextMaintenanceCompaction || decision.Reason != "cleanup_cache_gate_failed" {
		t.Fatalf("warm deep-prefix edit should route hard pressure to compaction: %#v", decision)
	}
	if decision.CleanupSkippedWarmSuffixCount == 0 || decision.CacheViableCandidateTokens != 0 {
		t.Fatalf("warm-suffix skip attribution is missing: %#v", decision)
	}

	policy.ProviderCacheState = ProviderCacheCold
	decision = PlanContextPressure(messages, nil, policy)
	if decision.Action != ContextMaintenanceCleanup {
		t.Fatalf("cold cache should permit one batch cleanup: %#v", decision)
	}
	if decision.Cleanup.ReclaimedTokens < decision.MinimumCleanupTokens || decision.Cleanup.PressureAfter > policy.CleanupTarget {
		t.Fatalf("cleanup did not establish recovery waterline: %#v", decision.Cleanup)
	}
	if decision.Cleanup.EarliestChanged != decision.Cleanup.Replacements[0].MessageIndex {
		t.Fatalf("cache mutation boundary must start at the first changed result body: %#v", decision.Cleanup)
	}
	projected := toolresult.ApplyCleanupPlan(messages, decision.Cleanup)
	for _, replacement := range decision.Cleanup.Replacements {
		got := projected[replacement.MessageIndex]
		if got.Content != replacement.Placeholder || !strings.Contains(got.Content, "Older tool result removed") {
			t.Fatalf("replacement %d not applied deterministically: %#v", replacement.MessageIndex, got)
		}
		if messages[replacement.MessageIndex].Content == got.Content {
			t.Fatalf("canonical input was mutated at %d", replacement.MessageIndex)
		}
	}
}

func TestContextPressureAtHardThresholdCompactsWhenSavingsInsufficient(t *testing.T) {
	messages := []*agent.Message{
		agent.SystemMessage(strings.Repeat("stable ", 3000)),
		agent.UserMessage(strings.Repeat("unrecoverable ", 12_000)),
	}
	policy := pressureTestPolicy(30_000)
	decision := PlanContextPressure(messages, nil, policy)
	if decision.Action != ContextMaintenanceCompaction {
		t.Fatalf("85%% pressure must compact even without clearable results: %#v", decision)
	}
	if decision.CleanupSkippedBelowMinimumCount != 1 || decision.CacheViableCandidateTokens != 0 {
		t.Fatalf("insufficient cleanup savings were not attributed: %#v", decision)
	}
}

func TestContextPressureBodyCleanupCannotBypassFullWindowHardCap(t *testing.T) {
	messages := pressureHistory(20, 1000, agent.ToolResultDeferred, agent.ToolResultContextDiscardable)
	messages[0] = agent.SystemMessage(strings.Repeat("stable-prefix ", 20_000))
	prefix := stableModelPrefixTokens(messages, nil)
	policy := pressureTestPolicy(max(1, int(float64(prefix)/0.68)))
	policy.Scope = ContextPressureBodyAfterPrefix
	policy.KeepRecentGroups = 0
	policy.KeepRecentTokens = 0
	policy.ProviderCacheState = ProviderCacheCold

	decision := PlanContextPressure(messages, nil, policy)
	if decision.Pressure < policy.CleanupThreshold || decision.FullPressure < policy.CompactionThreshold {
		t.Fatalf("test setup did not cross both pressure thresholds: %#v", decision)
	}
	if decision.Cleanup.PressureAfter > policy.CleanupTarget ||
		decision.Cleanup.FullPressureAfter < policy.CompactionThreshold {
		t.Fatalf("test setup does not isolate the full-window guard: %#v", decision.Cleanup)
	}
	if decision.Action != ContextMaintenanceCompaction || decision.Reason != "cleanup_full_pressure_remains_high" {
		t.Fatalf("body-only recovery incorrectly consumed the structural turn: %#v", decision)
	}
}

func TestContextPressureCompactsBeforeThresholdWhenForkReserveWouldNotFit(t *testing.T) {
	messages := []*agent.Message{agent.UserMessage(strings.Repeat("context ", 13_000))}
	policy := pressureTestPolicy(30_000)
	policy.CleanupThreshold = 0.90
	policy.CompactionThreshold = 0.95
	policy.CompactionPromptTokens = 1500
	policy.CheckpointOutputReserve = 2500
	policy.SafetyMarginTokens = 1000
	decision := PlanContextPressure(messages, nil, policy)
	if decision.FullPressure >= policy.CompactionThreshold {
		t.Fatalf("test setup already crossed ratio threshold: %#v", decision)
	}
	if decision.Action != ContextMaintenanceCompaction || decision.Reason != "compaction_capacity_reserve" {
		t.Fatalf("capacity reserve did not trigger early compaction: %#v", decision)
	}
}

func TestContextPressurePreservesExplicitZeroPolicyLimits(t *testing.T) {
	policy := pressureTestPolicy(100_000)
	policy.CleanupMinTokens = 0
	policy.KeepRecentGroups = 0
	policy.KeepRecentTokens = 0
	policy.WarmSuffixTokens = 0
	policy.EagerMinTokens = 0

	normalized := policy.normalized()
	if normalized.CleanupMinTokens != 0 || normalized.KeepRecentGroups != 0 || normalized.KeepRecentTokens != 0 ||
		normalized.WarmSuffixTokens != 0 || normalized.EagerMinTokens != 0 {
		t.Fatalf("explicit zero limits were replaced: %#v", normalized)
	}
}

func TestContextPressureBodyScopeIncludesStableLeadingContextAndCheckpoint(t *testing.T) {
	leading := agent.UserMessage(strings.Repeat("resident ", 2000))
	leading.Extra = map[string]any{agentcontext.MessageExtraPlacement: string(agentcontext.PlacementLeadingMessage)}
	messages := []*agent.Message{
		agent.SystemMessage(strings.Repeat("system ", 500)),
		leading,
		NewCompactionSummaryMessage(2, strings.Repeat("checkpoint ", 1000)),
		agent.UserMessage(strings.Repeat("mutable ", 1000)),
	}
	withPrefix := stableModelPrefixTokens(messages, nil)
	withoutPrefix := stableModelPrefixTokens(messages[3:], nil)
	if withPrefix <= withoutPrefix || withPrefix <= EstimateMessageTokens(messages[0]) {
		t.Fatalf("stable prefix did not include leading/checkpoint context: with=%d without=%d", withPrefix, withoutPrefix)
	}
}

func TestEagerResultWaitsForSettlementThenUsesSharedCacheGate(t *testing.T) {
	result := pressureToolResult("call-eager", strings.Repeat("large-log ", 16_000), agent.ToolResultEagerCandidate, agent.ToolResultContextDiscardable)
	base := []*agent.Message{
		agent.UserMessage("run analysis"),
		agent.AssistantMessage("", []agent.ToolCall{pressureCall("call-eager", 0)}),
		result,
	}
	policy := pressureTestPolicy(160_000)
	policy.KeepRecentGroups = 3
	policy.ProviderCacheState = ProviderCacheCold
	if decision := PlanContextPressure(base, nil, policy); decision.Action != ContextMaintenanceNone {
		t.Fatalf("unsettled eager result must stay rich: %#v", decision)
	}

	settled := append(append([]*agent.Message(nil), base...), agent.AssistantMessage("conclusion preserved", nil), agent.UserMessage("continue"))
	decision := PlanContextPressure(settled, nil, policy)
	if decision.Action != ContextMaintenanceCleanup || !decision.Cleanup.EagerOnly {
		t.Fatalf("settled recoverable eager result should transition once: %#v", decision)
	}
	if len(decision.Cleanup.Replacements) != 1 || !strings.Contains(decision.Cleanup.Replacements[0].Placeholder, `"path":".denova/artifacts/session/call-eager.log"`) {
		t.Fatalf("eager receipt lacks bounded recovery hint: %#v", decision.Cleanup)
	}
}

func TestEagerCleanupBatchesEveryCacheViableSettledResult(t *testing.T) {
	messages := []*agent.Message{
		agent.UserMessage("first analysis"),
		agent.AssistantMessage("", []agent.ToolCall{pressureCall("eager-one", 1)}),
		pressureToolResult("eager-one", strings.Repeat("large-one ", 10_000), agent.ToolResultEagerCandidate, agent.ToolResultContextDiscardable),
		agent.AssistantMessage("first conclusion preserved", nil),
		agent.UserMessage("second analysis"),
		agent.AssistantMessage("", []agent.ToolCall{pressureCall("eager-two", 2)}),
		pressureToolResult("eager-two", strings.Repeat("large-two ", 10_000), agent.ToolResultEagerCandidate, agent.ToolResultContextDiscardable),
		agent.AssistantMessage("second conclusion preserved", nil),
		agent.UserMessage("next turn"),
	}
	policy := pressureTestPolicy(160_000)
	policy.ProviderCacheState = ProviderCacheCold
	decision := PlanContextPressure(messages, nil, policy)
	if decision.Action != ContextMaintenanceCleanup || !decision.Cleanup.EagerOnly {
		t.Fatalf("eager batch decision = %#v", decision)
	}
	if len(decision.Cleanup.Replacements) != 2 ||
		decision.Cleanup.Replacements[0].ToolCallID != "eager-one" ||
		decision.Cleanup.Replacements[1].ToolCallID != "eager-two" {
		t.Fatalf("eager cleanup must mutate the warm prefix once for every eligible result: %#v", decision.Cleanup)
	}
}

func TestToolResultPlaceholderRenderingIsDeterministicAndArgumentIndependent(t *testing.T) {
	call := pressureCall("call-1", 1)
	call.Function.Arguments = `{"authorization":"secret","path":"never-copy-this"}`
	message := pressureToolResult("call-1", strings.Repeat("body", 2000), agent.ToolResultDeferred, agent.ToolResultContextNormal)
	first, ok := renderToolResultPlaceholder(call, message, "call-2")
	if !ok {
		t.Fatal("render placeholder")
	}
	second, ok := renderToolResultPlaceholder(call, message, "call-2")
	if !ok || first != second {
		t.Fatalf("placeholder is not deterministic: first=%#v second=%#v", first, second)
	}
	if strings.Contains(first.Content, "secret") || strings.Contains(first.Content, "never-copy-this") {
		t.Fatalf("renderer copied raw tool arguments: %s", first.Content)
	}
	if first.Version != ToolResultPlaceholderRendererVersion || !strings.Contains(first.Content, "Superseded by tool call: call-2") {
		t.Fatalf("placeholder metadata missing: %#v", first)
	}
}

func TestToolResultCleanupRejectsRedactedOnlyRecoveryIdentity(t *testing.T) {
	message := pressureToolResult("call-secret", strings.Repeat("body", 2000), agent.ToolResultEagerCandidate, agent.ToolResultContextDiscardable)
	message.ToolResult.ContextHints.Recovery.ArtifactPath = ""
	message.ToolResult.ContextHints.Recovery.Reference = map[string]any{"url": "[REDACTED]"}
	if _, ok := renderToolResultPlaceholder(pressureCall("call-secret", 0), message, ""); ok {
		t.Fatal("redacted-only recovery metadata must not authorize rich-result cleanup")
	}
	if !ToolResultProtected(message) {
		t.Fatal("redacted-only recovery metadata must remain protected")
	}
}

func TestUsableToolResultRecoveryRequiresKindSpecificExecutableIdentity(t *testing.T) {
	tests := []struct {
		name     string
		recovery agent.ToolResultRecoveryHint
		usable   bool
	}{
		{name: "read path", recovery: agent.ToolResultRecoveryHint{
			Kind: agent.ToolResultRecoveryRead, Reference: map[string]any{"path": "chapters/one.md", "line": float64(20)},
		}, usable: true},
		{name: "read line and limit only", recovery: agent.ToolResultRecoveryHint{
			Kind: agent.ToolResultRecoveryRead, Reference: map[string]any{"line": float64(20), "limit": float64(100)},
		}},
		{name: "read nested reference with pagination only", recovery: agent.ToolResultRecoveryHint{
			Kind: agent.ToolResultRecoveryRead, Reference: map[string]any{"reference": map[string]any{"line": float64(20), "limit": float64(100)}},
		}},
		{name: "read cannot borrow artifact path", recovery: agent.ToolResultRecoveryHint{
			Kind: agent.ToolResultRecoveryRead, ArtifactPath: ".denova/artifacts/output.log",
		}},
		{name: "refetch absolute URL", recovery: agent.ToolResultRecoveryHint{
			Kind: agent.ToolResultRecoveryRefetch, Reference: map[string]any{"url": "https://example.com/page", "start_index": float64(100)},
		}, usable: true},
		{name: "refetch relative URL", recovery: agent.ToolResultRecoveryHint{
			Kind: agent.ToolResultRecoveryRefetch, Reference: map[string]any{"url": "/page", "limit": float64(100)},
		}},
		{name: "refetch query without scope", recovery: agent.ToolResultRecoveryHint{
			Kind: agent.ToolResultRecoveryRefetch, Reference: map[string]any{"query": "release notes"},
		}},
		{name: "refetch scoped query", recovery: agent.ToolResultRecoveryHint{
			Kind: agent.ToolResultRecoveryRefetch, Reference: map[string]any{"query": "release notes", "scope": "docs.example.com"},
		}, usable: true},
		{name: "rerun query", recovery: agent.ToolResultRecoveryHint{
			Kind: agent.ToolResultRecoveryRerun, Reference: map[string]any{"query": "stable invocation", "limit": float64(20)},
		}, usable: true},
		{name: "rerun pagination only", recovery: agent.ToolResultRecoveryHint{
			Kind: agent.ToolResultRecoveryRerun, Reference: map[string]any{"line": float64(20), "limit": float64(100), "cursor": "page-2"},
		}},
		{name: "artifact readable path", recovery: agent.ToolResultRecoveryHint{
			Kind: agent.ToolResultRecoveryArtifact, ArtifactPath: ".denova/artifacts/output.log",
		}, usable: true},
		{name: "artifact reference without readable path", recovery: agent.ToolResultRecoveryHint{
			Kind: agent.ToolResultRecoveryArtifact, Reference: map[string]any{"path": ".denova/artifacts/output.log"},
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := usableToolResultRecovery(test.recovery); got != test.usable {
				t.Fatalf("usableToolResultRecovery(%#v) = %t, want %t", test.recovery, got, test.usable)
			}
		})
	}
}

func TestToolResultPlaceholderRejectsNonIdentityPaginationFields(t *testing.T) {
	for _, recovery := range []agent.ToolResultRecoveryHint{
		{Kind: agent.ToolResultRecoveryRead, Reference: map[string]any{"line": float64(12), "limit": float64(50)}},
		{Kind: agent.ToolResultRecoveryRefetch, Reference: map[string]any{"start_index": float64(500), "max_chars": float64(1000)}},
		{Kind: agent.ToolResultRecoveryRerun, Reference: map[string]any{"cursor": "next", "offset": float64(20), "limit": float64(10)}},
	} {
		message := pressureToolResult("call-pagination", strings.Repeat("body", 2000), agent.ToolResultDeferred, agent.ToolResultContextNormal)
		message.ToolResult.ContextHints.Recovery = recovery
		if _, ok := renderToolResultPlaceholder(pressureCall("call-pagination", 0), message, ""); ok {
			t.Fatalf("pagination-only recovery authorized cleanup: %#v", recovery)
		}
		if !ToolResultProtected(message) {
			t.Fatalf("pagination-only recovery was not protected: %#v", recovery)
		}
	}
}

func TestAttachmentArtifactCannotAuthorizeToolResultCleanup(t *testing.T) {
	message := pressureToolResult("call-attachment", strings.Repeat("body", 2000), agent.ToolResultDeferred, agent.ToolResultContextNormal)
	path := ".denova/artifacts/attachment.png"
	message.ToolResult.ContextHints.Recovery = agent.ToolResultRecoveryHint{
		Kind: agent.ToolResultRecoveryArtifact, ArtifactPath: path,
	}
	message.ToolResult.Artifacts = []agent.ToolArtifactRef{{
		ID: "attachment", Purpose: agent.ToolArtifactPurposeAttachment,
		ReadablePath: path, ContentType: "image/png", Complete: true,
	}}
	if _, ok := renderToolResultPlaceholder(pressureCall("call-attachment", 0), message, ""); ok {
		t.Fatal("attachment artifact authorized a cleanup placeholder")
	}
	if !ToolResultProtected(message) {
		t.Fatal("attachment-backed result was not protected")
	}

	message.ToolResult.Artifacts[0].Purpose = agent.ToolArtifactPurposeCompleteModelOutput
	if _, ok := renderToolResultPlaceholder(pressureCall("call-attachment", 0), message, ""); !ok {
		t.Fatal("explicit complete-model-output artifact did not authorize cleanup")
	}
}

func TestCleanupPlanPrefersSupersededResultBeforeNewerOrdinaryEvidence(t *testing.T) {
	oldCall := pressureCall("old", 0)
	newCall := pressureCall("new", 1)
	ordinaryCall := pressureCall("ordinary", 2)
	oldResult := pressureToolResult("old", strings.Repeat("obsolete ", 5000), agent.ToolResultDeferred, agent.ToolResultContextNormal)
	newResult := pressureToolResult("new", strings.Repeat("current ", 5000), agent.ToolResultDeferred, agent.ToolResultContextNormal)
	ordinaryResult := pressureToolResult("ordinary", strings.Repeat("evidence ", 5000), agent.ToolResultDeferred, agent.ToolResultContextNormal)
	oldResult.ToolResult.ContextHints.SupersessionKey = "read:same-target"
	newResult.ToolResult.ContextHints.SupersessionKey = "read:same-target"
	ordinaryResult.ToolResult.ContextHints.SupersessionKey = "read:other-target"
	messages := []*agent.Message{
		agent.UserMessage("old"), agent.AssistantMessage("", []agent.ToolCall{oldCall}), oldResult, agent.AssistantMessage("used old", nil),
		agent.UserMessage("new"), agent.AssistantMessage("", []agent.ToolCall{newCall}), newResult, agent.AssistantMessage("used new", nil),
		agent.UserMessage("ordinary"), agent.AssistantMessage("", []agent.ToolCall{ordinaryCall}), ordinaryResult, agent.AssistantMessage("used ordinary", nil),
		agent.UserMessage("current turn"),
	}
	policy := pressureTestPolicy(200_000)
	policy.KeepRecentGroups = 0
	policy.KeepRecentTokens = 0
	policy.ProviderCacheState = ProviderCacheCold
	groups, _ := collectToolInteractionGroups(messages, policy)
	markSupersededGroups(messages, groups)
	prepareCleanupReplacements(messages, groups)
	plan, ok := buildCleanupPlan(messages, groups, 12_000, 0, 12_000, policy, false)
	if !ok || len(plan.Replacements) == 0 {
		t.Fatalf("cleanup plan missing: %#v", plan)
	}
	if got := plan.Replacements[0].ToolCallID; got != "old" {
		t.Fatalf("first cleanup target = %q, want superseded old result", got)
	}
}

func TestCleanupPlanPrefersLargestResultWithinSameSemanticTier(t *testing.T) {
	largeCall := pressureCall("large", 0)
	smallCall := pressureCall("small", 1)
	largeResult := pressureToolResult("large", strings.Repeat("large ", 6000), agent.ToolResultDeferred, agent.ToolResultContextNormal)
	smallResult := pressureToolResult("small", strings.Repeat("small ", 800), agent.ToolResultDeferred, agent.ToolResultContextNormal)
	largeResult.ToolResult.ContextHints.SupersessionKey = "read:large"
	smallResult.ToolResult.ContextHints.SupersessionKey = "read:small"
	messages := []*agent.Message{
		agent.UserMessage("large"), agent.AssistantMessage("", []agent.ToolCall{largeCall}), largeResult, agent.AssistantMessage("used large", nil),
		agent.UserMessage("small"), agent.AssistantMessage("", []agent.ToolCall{smallCall}), smallResult, agent.AssistantMessage("used small", nil),
		agent.UserMessage("current turn"),
	}
	policy := pressureTestPolicy(100_000)
	policy.KeepRecentGroups = 0
	policy.KeepRecentTokens = 0
	policy.ProviderCacheState = ProviderCacheCold
	groups, _ := collectToolInteractionGroups(messages, policy)
	markSupersededGroups(messages, groups)
	prepareCleanupReplacements(messages, groups)
	plan, ok := buildCleanupPlan(messages, groups, 6000, 0, 6000, policy, false)
	if !ok || len(plan.Replacements) == 0 {
		t.Fatalf("cleanup plan missing: %#v", plan)
	}
	if got := plan.Replacements[0].ToolCallID; got != "large" {
		t.Fatalf("first cleanup target = %q, want largest result", got)
	}
}

func TestContextPressureHardOverflowReclaimsRecentRecoverableResult(t *testing.T) {
	large := pressureToolResult(
		"large", strings.Repeat("recoverable overflow ", 20_000),
		agent.ToolResultDeferred, agent.ToolResultContextNormal,
	)
	messages := []*agent.Message{
		agent.UserMessage("inspect"),
		agent.AssistantMessage("", []agent.ToolCall{pressureCall("large", 0)}),
		large,
		agent.AssistantMessage("result consumed", nil),
		agent.UserMessage("current turn"),
	}
	policy := pressureTestPolicy(20_000)
	policy.ProviderCacheState = ProviderCacheWarm
	decision := PlanContextPressure(messages, nil, policy)
	if decision.FullPressure < 1 {
		t.Fatalf("fixture is not over the hard window: %#v", decision)
	}
	if decision.Action != ContextMaintenanceCleanup {
		t.Fatalf("recoverable result should resolve hard overflow before compaction: %#v", decision)
	}
	if len(decision.Cleanup.Replacements) != 1 || decision.Cleanup.Replacements[0].ToolCallID != "large" {
		t.Fatalf("hard overflow cleanup = %#v, want consumed recent result", decision.Cleanup)
	}
}

func TestCleanupPlanDoesNotLetOldSemanticCandidatePoisonCacheViableBatch(t *testing.T) {
	oldCall := pressureCall("old", 0)
	newCall := pressureCall("new", 1)
	oldResult := pressureToolResult("old", strings.Repeat("obsolete ", 6000), agent.ToolResultDeferred, agent.ToolResultContextDiscardable)
	newResult := pressureToolResult("new", strings.Repeat("recoverable ", 6000), agent.ToolResultDeferred, agent.ToolResultContextNormal)
	oldResult.ToolResult.ContextHints.SupersessionKey = "read:obsolete"
	newResult.ToolResult.ContextHints.SupersessionKey = "read:current"
	messages := []*agent.Message{
		agent.UserMessage("old"), agent.AssistantMessage("", []agent.ToolCall{oldCall}), oldResult, agent.AssistantMessage("used old", nil),
		agent.UserMessage(strings.Repeat("warm middle ", 10_000)),
		agent.AssistantMessage("", []agent.ToolCall{newCall}), newResult, agent.AssistantMessage("used new", nil),
		agent.UserMessage("current turn"),
	}
	policy := pressureTestPolicy(200_000)
	policy.KeepRecentGroups = 0
	policy.KeepRecentTokens = 0
	policy.ProviderCacheState = ProviderCacheUnknown
	policy.WarmSuffixTokens = EstimateMessageTokens(messages[len(messages)-1]) + EstimateMessageTokens(messages[len(messages)-2]) + EstimateMessageTokens(newResult) + 100

	groups, _ := collectToolInteractionGroups(messages, policy)
	markSupersededGroups(messages, groups)
	prepareCleanupReplacements(messages, groups)
	plan, ok := buildCleanupPlan(messages, groups, 16_000, 0, 16_000, policy, false)
	if !ok || len(plan.Replacements) != 1 {
		t.Fatalf("cache-viable cleanup plan missing: %#v", plan)
	}
	if got := plan.Replacements[0].ToolCallID; got != "new" {
		t.Fatalf("cleanup target = %q, want newer cache-viable group", got)
	}
}

func pressureTestPolicy(window int) ContextPressurePolicy {
	return ContextPressurePolicy{
		Enabled: true, CompactionEnabled: true, CleanupEnabled: true, Scope: ContextPressureTotal, ContextWindowTokens: window,
		CleanupThreshold: 0.70, CleanupTarget: 0.60, CleanupMinTokens: 2000,
		KeepRecentGroups: 3, KeepRecentTokens: 1000, WarmSuffixTokens: 8000,
		EagerMinTokens: 20_000, EagerMinContextRatio: 0.10,
		CompactionThreshold: 0.85, CompactionRecoveryBand: 0.80,
	}
}

func pressureHistory(groups, resultWords int, retention agent.ToolResultRetentionMode, value agent.ToolResultContextValue) []*agent.Message {
	messages := []*agent.Message{agent.SystemMessage("stable system")}
	for index := 0; index < groups; index++ {
		callID := fmt.Sprintf("call-%d", index)
		messages = append(messages,
			agent.UserMessage(fmt.Sprintf("request %d", index)),
			agent.AssistantMessage("", []agent.ToolCall{pressureCall(callID, index)}),
			pressureToolResult(callID, fmt.Sprintf("rich-result-%d ", index)+strings.Repeat("payload ", resultWords), retention, value),
			agent.AssistantMessage(fmt.Sprintf("used result %d", index), nil),
		)
	}
	messages = append(messages, agent.UserMessage("current turn"))
	return messages
}

func pressureCall(callID string, index int) agent.ToolCall {
	return agent.ToolCall{
		ID: callID, Type: "function",
		Function: agent.FunctionCall{Name: "read", Arguments: fmt.Sprintf(`{"path":"chapter-%d.md"}`, index)},
	}
}

func pressureToolResult(callID, content string, retention agent.ToolResultRetentionMode, value agent.ToolResultContextValue) *agent.Message {
	result := agent.TextToolResult(content)
	result.ResultRetention = retention
	result.ContextHints = &agent.ToolResultContextHints{
		Recovery: agent.ToolResultRecoveryHint{
			Kind:           agent.ToolResultRecoveryRead,
			Reference:      map[string]any{"path": ".denova/artifacts/session/" + callID + ".log"},
			ArtifactPath:   ".denova/artifacts/session/" + callID + ".log",
			EstimatedBytes: int64(len(content)), EstimatedTokens: EstimateStringTokens(content),
		},
		ContextValue:    value,
		SupersessionKey: "read:chapter.md",
	}
	return agent.ToolMessage(result, callID, agent.WithToolName("read"))
}
