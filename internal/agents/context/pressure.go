package context

import (
	"denova/internal/agents/toolresult"
	agent "github.com/alfredxw/denova/agent"
	basecontext "github.com/alfredxw/denova/agent/context"

	"denova/config"
)

type ContextPressureScope string

const (
	ContextPressureTotal                 ContextPressureScope = "total"
	ContextPressureBodyAfterPrefix       ContextPressureScope = "body_after_prefix"
	contextPressureOrderingFallbackRatio                      = 0.85
)

type ProviderCacheState string

const (
	ProviderCacheUnknown ProviderCacheState = "unknown"
	ProviderCacheWarm    ProviderCacheState = "warm"
	ProviderCacheCold    ProviderCacheState = "cold"
)

type ContextMaintenanceAction string

const (
	ContextMaintenanceNone       ContextMaintenanceAction = "none"
	ContextMaintenanceCleanup    ContextMaintenanceAction = "cleanup"
	ContextMaintenanceCompaction ContextMaintenanceAction = "compaction"
)

// ContextPressurePolicy is the provider-neutral policy shared by writing and
// game conversations. Provider adapters expose cache capability/state but do
// not decide which result has semantic value.
type ContextPressurePolicy struct {
	AgentKind               string
	Enabled                 bool
	CompactionEnabled       bool
	CleanupEnabled          bool
	Scope                   ContextPressureScope
	ContextWindowTokens     int
	ReservedTokens          int
	CleanupThreshold        float64
	CleanupTarget           float64
	CleanupMinTokens        int
	KeepRecentGroups        int
	KeepRecentTokens        int
	WarmSuffixTokens        int
	EagerMinTokens          int
	EagerMinContextRatio    float64
	CompactionThreshold     float64
	CompactionRecoveryBand  float64
	CompactionPromptTokens  int
	CheckpointOutputReserve int
	SafetyMarginTokens      int
	ProviderCacheState      ProviderCacheState
	ObservedPromptTokens    int
}

// ObservePromptUsage merges durable provider telemetry into a newly assembled
// request policy. A zero cached-token count remains ambiguous for compatible
// providers, while a positive count proves that the relevant provider cache
// was warm. Local projection still wins whenever it is larger.
func (policy ContextPressurePolicy) ObservePromptUsage(promptTokens, cachedTokens int) ContextPressurePolicy {
	if promptTokens > policy.ObservedPromptTokens {
		policy.ObservedPromptTokens = promptTokens
	}
	if promptTokens > 0 && cachedTokens > 0 {
		policy.ProviderCacheState = ProviderCacheWarm
	}
	return policy.normalized()
}

func ProviderCacheStateFromMessages(messages []*agent.Message) ProviderCacheState {
	for index := len(messages) - 1; index >= 0; index-- {
		message := messages[index]
		if message == nil || message.ResponseMeta == nil || message.ResponseMeta.Usage == nil || message.ResponseMeta.Usage.PromptTokens <= 0 {
			continue
		}
		if message.ResponseMeta.Usage.PromptTokenDetails.CachedTokens > 0 {
			return ProviderCacheWarm
		}
		// A zero value is ambiguous: many compatible providers omit cache usage
		// entirely. Treat it as unknown (and therefore warm for mutation gating)
		// until an adapter can prove that the relevant prefix is cold.
		return ProviderCacheUnknown
	}
	return ProviderCacheUnknown
}

func (policy ContextPressurePolicy) normalized() ContextPressurePolicy {
	if policy.Scope != ContextPressureTotal && policy.Scope != ContextPressureBodyAfterPrefix {
		policy.Scope = ContextPressureBodyAfterPrefix
	}
	if policy.CleanupThreshold <= 0 || policy.CleanupThreshold >= 1 {
		policy.CleanupThreshold = 0.70
	}
	if policy.CompactionThreshold <= 0 || policy.CompactionThreshold >= 1 {
		policy.CompactionThreshold = config.DefaultContextCompactionThreshold
	}
	if policy.CleanupThreshold >= policy.CompactionThreshold {
		policy.CleanupThreshold = policy.CompactionThreshold * contextPressureOrderingFallbackRatio
	}
	if policy.CleanupTarget <= 0 || policy.CleanupTarget >= policy.CleanupThreshold {
		policy.CleanupTarget = policy.CleanupThreshold * contextPressureOrderingFallbackRatio
	}
	if policy.CleanupMinTokens < 0 {
		policy.CleanupMinTokens = 0
	}
	if policy.KeepRecentGroups < 0 {
		policy.KeepRecentGroups = 0
	}
	if policy.KeepRecentTokens < 0 {
		policy.KeepRecentTokens = 0
	}
	if policy.WarmSuffixTokens < 0 {
		policy.WarmSuffixTokens = 0
	}
	if policy.EagerMinTokens < 0 {
		policy.EagerMinTokens = 0
	}
	if policy.EagerMinContextRatio <= 0 || policy.EagerMinContextRatio >= 1 {
		policy.EagerMinContextRatio = 0.15
	}
	if policy.CompactionRecoveryBand <= 0 || policy.CompactionRecoveryBand > 1 {
		policy.CompactionRecoveryBand = 0.80
	}
	if policy.ProviderCacheState == "" {
		policy.ProviderCacheState = ProviderCacheUnknown
	}
	return policy
}

type ContextPressureDecision struct {
	Action               ContextMaintenanceAction
	Reason               string
	Scope                ContextPressureScope
	ProviderCacheState   ProviderCacheState
	LocalProjectedTokens int
	ObservedPromptTokens int
	EffectiveTokens      int
	StablePrefixTokens   int
	Pressure             float64
	FullPressure         float64
	MinimumCleanupTokens int
	ProtectedResultCount int
	CandidateTokens      int
	// CacheViableCandidateTokens is the amount that can be reclaimed without
	// rewriting more of a warm suffix than policy permits.
	CacheViableCandidateTokens      int
	CleanupSkippedBelowMinimumCount int
	CleanupSkippedWarmSuffixCount   int
	EagerCandidateCount             int
	SupersededCount                 int
	DiscardableCount                int
	Cleanup                         toolresult.CleanupPlan
}

// PlanContextPressure performs a dry run against the exact model-visible
// messages. It never mutates messages or the canonical journal.
func PlanContextPressure(messages []*agent.Message, tools []*agent.ToolInfo, policy ContextPressurePolicy) ContextPressureDecision {
	policy = policy.normalized()
	decision := ContextPressureDecision{
		Action: ContextMaintenanceNone, Reason: "disabled", Scope: policy.Scope,
		ProviderCacheState: policy.ProviderCacheState,
	}
	if !policy.Enabled || policy.ContextWindowTokens <= 0 {
		return decision
	}

	local := EstimateTokens(messages, tools) + max(0, policy.ReservedTokens)
	effective := max(local, policy.ObservedPromptTokens+max(0, policy.ReservedTokens))
	prefix := stableModelPrefixTokens(messages, tools)
	pressure, fullPressure, budget := contextPressureRatios(effective, prefix, policy)
	cleanupPolicy := policy
	if fullPressure >= 1 {
		// A request that already exceeds the model window cannot preserve a warm
		// mutable suffix or recently consumed recoverable results. Pending,
		// unsuccessful, and otherwise non-recoverable tool groups remain protected
		// by collectToolInteractionGroups; only cache/recency preferences yield.
		cleanupPolicy.ProviderCacheState = ProviderCacheCold
		cleanupPolicy.KeepRecentGroups = 0
		cleanupPolicy.KeepRecentTokens = 0
	}
	minimum := max(policy.CleanupMinTokens, budget/10)
	decision.LocalProjectedTokens = local
	decision.ObservedPromptTokens = policy.ObservedPromptTokens
	decision.EffectiveTokens = effective
	decision.StablePrefixTokens = prefix
	decision.Pressure = pressure
	decision.FullPressure = fullPressure
	decision.MinimumCleanupTokens = minimum
	capacityAtRisk := effective+max(0, policy.CompactionPromptTokens)+max(0, policy.CheckpointOutputReserve)+max(0, policy.SafetyMarginTokens) > policy.ContextWindowTokens

	var groups []*toolInteractionGroup
	protected := 0
	if policy.CleanupEnabled {
		groups, protected = collectToolInteractionGroups(messages, cleanupPolicy)
	}
	decision.ProtectedResultCount = protected
	markSupersededGroups(messages, groups)
	for _, group := range groups {
		if group.eager {
			decision.EagerCandidateCount++
		}
		if group.superseded {
			decision.SupersededCount++
		}
		if group.discardable {
			decision.DiscardableCount++
		}
	}
	protectRecentToolGroups(groups, cleanupPolicy)
	prepareCleanupReplacements(messages, groups)

	var eagerGroups, ordinaryGroups []*toolInteractionGroup
	for _, group := range groups {
		if group.protected || group.reclaimed <= 0 {
			continue
		}
		decision.CandidateTokens += group.reclaimed
		if group.eager {
			eagerGroups = append(eagerGroups, group)
		}
		ordinaryGroups = append(ordinaryGroups, group)
	}
	cacheViableGroups := cacheViableCleanupGroups(messages, ordinaryGroups, cleanupPolicy)
	decision.CleanupSkippedWarmSuffixCount = max(0, len(ordinaryGroups)-len(cacheViableGroups))
	for _, group := range cacheViableGroups {
		decision.CacheViableCandidateTokens += group.reclaimed
	}
	if (pressure >= policy.CleanupThreshold || fullPressure >= policy.CleanupThreshold || capacityAtRisk) &&
		decision.CacheViableCandidateTokens < minimum {
		decision.CleanupSkippedBelowMinimumCount = 1
	}

	// An eager transition may happen below the general pressure threshold, but
	// still uses the same cache mutation gate and one structural operation.
	if pressure < policy.CleanupThreshold && fullPressure < policy.CompactionThreshold && !capacityAtRisk {
		plan, ok := buildCleanupPlan(messages, eagerGroups, effective, prefix, budget, cleanupPolicy, true)
		if !ok {
			decision.Reason = "below_cleanup_threshold"
			return decision
		}
		decision.Action = ContextMaintenanceCleanup
		decision.Reason = "eager_recoverable_result"
		decision.Cleanup = plan
		return decision
	}

	plan, cleanupOK := buildCleanupPlan(messages, ordinaryGroups, effective, prefix, budget, cleanupPolicy, false)
	if cleanupOK {
		decision.Cleanup = plan
	}
	capacityAtRiskAfterCleanup := cleanupOK &&
		plan.ProjectedTokensAfter+max(0, policy.CompactionPromptTokens)+max(0, policy.CheckpointOutputReserve)+max(0, policy.SafetyMarginTokens) > policy.ContextWindowTokens
	cleanupRestoresFullWindow := cleanupOK && plan.FullPressureAfter < policy.CompactionThreshold && !capacityAtRiskAfterCleanup
	if cleanupOK && plan.ReclaimedTokens >= minimum && plan.PressureAfter <= policy.CleanupTarget && cleanupRestoresFullWindow {
		decision.Action = ContextMaintenanceCleanup
		decision.Reason = "cleanup_recovery_target_met"
		return decision
	}

	if policy.CompactionEnabled && (pressure >= policy.CompactionThreshold || fullPressure >= policy.CompactionThreshold || capacityAtRisk) {
		decision.Action = ContextMaintenanceCompaction
		switch {
		case capacityAtRisk && pressure < policy.CompactionThreshold && fullPressure < policy.CompactionThreshold:
			decision.Reason = "compaction_capacity_reserve"
		case decision.CleanupSkippedWarmSuffixCount > 0 && decision.CacheViableCandidateTokens == 0:
			decision.Reason = "cleanup_cache_gate_failed"
		case decision.CacheViableCandidateTokens < minimum:
			decision.Reason = "cleanup_savings_below_minimum"
		case !cleanupOK:
			decision.Reason = "cleanup_cache_gate_failed"
		case plan.FullPressureAfter >= policy.CompactionThreshold:
			decision.Reason = "cleanup_full_pressure_remains_high"
		case capacityAtRiskAfterCleanup:
			decision.Reason = "cleanup_capacity_reserve_not_restored"
		default:
			decision.Reason = "cleanup_cannot_reach_target"
		}
		return decision
	}

	if !policy.CompactionEnabled && (pressure >= policy.CompactionThreshold || fullPressure >= policy.CompactionThreshold || capacityAtRisk) {
		decision.Reason = "compaction_disabled"
	} else if decision.CleanupSkippedWarmSuffixCount > 0 && decision.CacheViableCandidateTokens == 0 {
		decision.Reason = "cleanup_cache_gate_failed"
	} else if decision.CleanupSkippedBelowMinimumCount > 0 {
		decision.Reason = "cleanup_savings_below_minimum"
	} else {
		decision.Reason = "cleanup_not_cost_effective"
	}
	return decision
}

// EffectiveToolResultCleanupMinimum returns the same cache-aware cleanup
// savings floor used by the planner. Health latches use it as well so a large
// context window cannot retrigger checkpoint work after less growth than the
// planner itself considers worth one prefix mutation.
func EffectiveToolResultCleanupMinimum(messages []*agent.Message, tools []*agent.ToolInfo, policy ContextPressurePolicy) int {
	policy = policy.normalized()
	prefix := stableModelPrefixTokens(messages, tools)
	_, _, budget := contextPressureRatios(max(0, EstimateTokens(messages, tools)+policy.ReservedTokens), prefix, policy)
	return max(policy.CleanupMinTokens, budget/10)
}

func stableModelPrefixTokens(messages []*agent.Message, tools []*agent.ToolInfo) int {
	prefix := EstimateTokens(nil, tools)
	for index, message := range messages {
		if !isStablePrefixMessage(message, index) {
			break
		}
		prefix += EstimateMessageTokens(message)
	}
	return prefix
}

func isStablePrefixMessage(message *agent.Message, index int) bool {
	if message == nil {
		return false
	}
	if message.Role == agent.System {
		return true
	}
	if placement, ok := message.Extra[basecontext.MessageExtraPlacement].(string); ok && placement == string(basecontext.PlacementLeadingMessage) {
		return true
	}
	// The checkpoint is stable between compactions and immediately follows the
	// stable leading context. Counting it keeps body pressure independent from a
	// large but cacheable checkpoint. It is never accepted as the first message.
	return index > 0 && IsCompactionSummaryMessage(message)
}

func contextPressureRatios(tokens, prefix int, policy ContextPressurePolicy) (pressure, full float64, budget int) {
	window := max(1, policy.ContextWindowTokens)
	full = float64(tokens) / float64(window)
	if policy.Scope == ContextPressureTotal {
		return full, full, window
	}
	budget = max(1, window-prefix)
	body := max(0, tokens-prefix)
	return float64(body) / float64(budget), full, budget
}
