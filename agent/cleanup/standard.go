// Package cleanup provides the built-in, storage-free tool-result cleanup
// policy used by Agent.Definition.Cleanup.
package cleanup

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"sync"

	agent "github.com/alfredxw/denova/agent"
)

const RendererVersion = "tool_result.placeholder.v1"

type CacheState string

type PressureScope string

const (
	CacheUnknown            CacheState    = "unknown"
	CacheWarm               CacheState    = "warm"
	CacheCold               CacheState    = "cold"
	PressureTotal           PressureScope = "total"
	PressureBodyAfterPrefix PressureScope = "body_after_prefix"
)

// StandardConfig contains model-visible cleanup policy only. Persistence,
// settlement, and CAS intentionally are not configurable manager concerns.
type StandardConfig struct {
	Scope                   PressureScope
	ContextWindowTokens     int
	ReservedTokens          int
	ObservedPromptTokens    int
	CleanupThreshold        float64
	CleanupTarget           float64
	CleanupMinTokens        int
	KeepRecentGroups        int
	KeepRecentTokens        int
	WarmSuffixTokens        int
	EagerMinTokens          int
	EagerMinContextRatio    float64
	CompactionEnabled       bool
	CompactionThreshold     float64
	CompactionPromptTokens  int
	CheckpointOutputReserve int
	SafetyMarginTokens      int
	CacheState              CacheState
}

type standardManager struct {
	config   StandardConfig
	identity agent.CapabilityIdentity
}

type standardDefinition struct {
	config StandardConfig
	once   sync.Once
	value  *standardManager
	err    error
}

// Standard declares the built-in Cleanup policy. Agent validates and resolves
// it together with the rest of the Definition in agent.New.
func Standard(config StandardConfig) agent.CleanupManager {
	return &standardDefinition{config: config}
}

func newStandard(config StandardConfig) (*standardManager, error) {
	if config.CacheState != "" && config.CacheState != CacheUnknown &&
		config.CacheState != CacheWarm && config.CacheState != CacheCold {
		return nil, fmt.Errorf("standard Cleanup CacheState %q is invalid", config.CacheState)
	}
	config = normalizedConfig(config)
	if config.ContextWindowTokens <= 0 {
		return nil, errors.New("standard Cleanup ContextWindowTokens must be positive")
	}
	encoded, _ := json.Marshal(config)
	digest := sha256.Sum256(encoded)
	return &standardManager{config: config, identity: agent.CapabilityIdentity{
		Kind: "cleanup.standard", Version: 1, ConfigHash: hex.EncodeToString(digest[:]),
	}}, nil
}

func (definition *standardDefinition) InitializeDefinition(context.Context) error {
	if definition == nil {
		return errors.New("standard Cleanup Definition is nil")
	}
	definition.once.Do(func() {
		definition.value, definition.err = newStandard(definition.config)
	})
	return definition.err
}

func (definition *standardDefinition) Identity() agent.CapabilityIdentity {
	if err := definition.InitializeDefinition(context.Background()); err != nil {
		return agent.CapabilityIdentity{}
	}
	return definition.value.Identity()
}

func (definition *standardDefinition) Plan(
	ctx context.Context,
	request agent.CleanupPlanRequest,
) (agent.CleanupPlan, error) {
	if err := definition.InitializeDefinition(ctx); err != nil {
		return agent.CleanupPlan{}, err
	}
	return definition.value.Plan(ctx, request)
}

var _ agent.CleanupManager = (*standardDefinition)(nil)
var _ agent.DefinitionInitializer = (*standardDefinition)(nil)

func normalizedConfig(config StandardConfig) StandardConfig {
	if config.Scope != PressureTotal && config.Scope != PressureBodyAfterPrefix {
		config.Scope = PressureBodyAfterPrefix
	}
	if config.CleanupThreshold <= 0 || config.CleanupThreshold >= 1 {
		config.CleanupThreshold = .70
	}
	if config.CompactionThreshold <= 0 || config.CompactionThreshold >= 1 {
		config.CompactionThreshold = .85
	}
	if config.CleanupThreshold >= config.CompactionThreshold {
		config.CleanupThreshold = config.CompactionThreshold * .85
	}
	if config.CleanupTarget <= 0 || config.CleanupTarget >= config.CleanupThreshold {
		config.CleanupTarget = config.CleanupThreshold * .85
	}
	if config.CleanupMinTokens < 0 {
		config.CleanupMinTokens = 0
	}
	if config.KeepRecentGroups < 0 {
		config.KeepRecentGroups = 0
	}
	if config.KeepRecentTokens < 0 {
		config.KeepRecentTokens = 0
	}
	if config.WarmSuffixTokens < 0 {
		config.WarmSuffixTokens = 0
	}
	if config.EagerMinTokens < 0 {
		config.EagerMinTokens = 0
	}
	if config.EagerMinContextRatio <= 0 || config.EagerMinContextRatio >= 1 {
		config.EagerMinContextRatio = .15
	}
	if config.CacheState == "" {
		config.CacheState = CacheUnknown
	}
	return config
}

func (manager *standardManager) Identity() agent.CapabilityIdentity { return manager.identity }

func (manager *standardManager) Plan(_ context.Context, request agent.CleanupPlanRequest) (agent.CleanupPlan, error) {
	if manager == nil {
		return agent.CleanupPlan{}, errors.New("standard Cleanup Manager is nil")
	}
	messages := request.ModelRequest
	if len(messages) == 0 {
		messages = request.Messages
	}
	config := manager.config
	maxOutputTokens := 0
	if maxTokens := request.ModelInspection.Options.MaxTokens; maxTokens != nil {
		maxOutputTokens = *maxTokens
	}
	reservedTokens := agent.CapacityAwareTokenReserve(
		config.ReservedTokens, maxOutputTokens, config.ContextWindowTokens, config.CompactionThreshold,
	)
	local := EstimateInspectedTokens(messages, request.ModelInspection) + reservedTokens
	observed, observedCache := latestPromptUsage(messages)
	observed = max(config.ObservedPromptTokens, observed)
	effective := max(local, observed+reservedTokens)
	checkpointOutputReserve := max(0, config.CheckpointOutputReserve)
	stablePrefix := stablePrefixTokens(messages, request.ModelInspection)
	pressure, fullPressure, budget := pressureRatios(effective, stablePrefix, config)
	plan := agent.CleanupPlan{Action: agent.CleanupNone, Reason: "below_cleanup_threshold", Renderer: RendererVersion,
		Metrics: agent.CleanupMetrics{EstimatedTokensBefore: effective, LocalProjectedTokens: local,
			ObservedPromptTokens: observed, EffectiveTokens: effective, EstimatedTokensAfter: effective,
			ContextWindowTokens: config.ContextWindowTokens, PressureBefore: fullPressure, PressureAfter: fullPressure,
			BodyPressureBefore: pressure, BodyPressureAfter: pressure, StablePrefixTokens: stablePrefix,
			PressureScope: string(config.Scope), ExecutionMode: "agent_projection", RendererVersion: RendererVersion}}
	if config.CacheState == CacheUnknown && observedCache == CacheWarm {
		config.CacheState = CacheWarm
	}
	if fullPressure >= 1 {
		config.CacheState = CacheCold
		config.KeepRecentGroups = 0
		config.KeepRecentTokens = 0
	}
	plan.Metrics.ProviderCacheState = string(config.CacheState)
	groups, protected := collectGroups(messages, config)
	plan.Metrics.ProtectedResults = protected
	prepareReplacements(messages, groups)
	ordinary := make([]*interactionGroup, 0, len(groups))
	for _, group := range groups {
		if group.eager {
			plan.Metrics.EagerCandidateCount++
		}
		if group.superseded {
			plan.Metrics.SupersededCandidateCount++
		}
		if group.discardable {
			plan.Metrics.DiscardableCandidateCount++
		}
		if !group.protected {
			plan.Metrics.CandidateTokens += group.reclaimed
			if group.reclaimed > 0 {
				ordinary = append(ordinary, group)
			}
		}
	}
	cacheViable := cacheViableGroups(messages, ordinary, config, false)
	plan.Metrics.SkippedWarmSuffixCount = max(0, len(ordinary)-len(cacheViable))
	for _, group := range cacheViable {
		plan.Metrics.CacheViableCandidateTokens += group.reclaimed
	}

	capacityAtRisk := effective+max(0, config.CompactionPromptTokens)+checkpointOutputReserve+max(0, config.SafetyMarginTokens) > config.ContextWindowTokens
	eagerOnly := pressure < config.CleanupThreshold && fullPressure < config.CompactionThreshold && !capacityAtRisk
	minimum := max(max(1, config.CleanupMinTokens), budget/10)
	plan.Metrics.MinimumCleanupTokens = minimum
	if !eagerOnly && plan.Metrics.CacheViableCandidateTokens < minimum {
		plan.Metrics.SkippedBelowMinimumCount = 1
	}
	selected := selectGroups(messages, groups, effective, stablePrefix, budget, config, eagerOnly)
	if len(selected) != 0 {
		plan.Replacements = flattenReplacements(selected)
		applyProjectionMetrics(messages, plan.Replacements, &plan.Metrics)
		plan.Metrics.ReplacementCount = len(plan.Replacements)
		plan.Metrics.EagerOnly = eagerOnly && selectedOnlyEager(selected)
		for _, group := range selected {
			if group.eager {
				plan.Metrics.EagerSelectedCount++
			}
		}
		plan.Metrics.EstimatedTokensAfter = max(1, effective-plan.Metrics.ReclaimedTokens)
		plan.Metrics.BodyPressureAfter, plan.Metrics.PressureAfter, _ = pressureRatios(plan.Metrics.EstimatedTokensAfter, stablePrefix, config)
	}

	eager := eagerOnly && len(selected) != 0 && selectedOnlyEager(selected)
	cleanupRestores := len(plan.Replacements) != 0 && plan.Metrics.ReclaimedTokens >= minimum &&
		plan.Metrics.BodyPressureAfter <= config.CleanupTarget && plan.Metrics.PressureAfter < config.CompactionThreshold &&
		plan.Metrics.EstimatedTokensAfter+max(0, config.CompactionPromptTokens)+checkpointOutputReserve+max(0, config.SafetyMarginTokens) <= config.ContextWindowTokens
	plan.FallbackToCompaction = config.CompactionEnabled && request.CompactionAvailable &&
		(pressure >= config.CompactionThreshold || fullPressure >= config.CompactionThreshold || capacityAtRisk)
	if eager || cleanupRestores {
		plan.Action = agent.CleanupProject
		if eager {
			plan.Reason = "eager_recoverable_result"
		} else {
			plan.Reason = "cleanup_recovery_target_met"
		}
		return plan, validatePlan(messages, plan)
	}
	if plan.FallbackToCompaction {
		plan.Action = agent.CleanupCompact
		plan.Reason = "cleanup_cannot_restore_context"
		return plan, validatePlan(messages, plan)
	}
	if !config.CompactionEnabled && (pressure >= config.CompactionThreshold || fullPressure >= config.CompactionThreshold || capacityAtRisk) {
		plan.Reason = "compaction_disabled"
	} else if pressure < config.CleanupThreshold && fullPressure < config.CompactionThreshold && !capacityAtRisk {
		plan.Reason = "below_cleanup_threshold"
	} else if plan.Metrics.CandidateTokens < minimum {
		plan.Reason = "cleanup_savings_below_minimum"
	} else {
		plan.Reason = "cleanup_not_cost_effective"
	}
	plan.Replacements = nil
	return plan, nil
}

// applyProjectionMetrics treats message index zero as a valid target. Keeping
// the initialization sentinel outside CleanupMetrics avoids conflating the
// zero value used by event encodings with the first position in a request.
func applyProjectionMetrics(messages []*agent.Message, replacements []agent.CleanupReplacement, metrics *agent.CleanupMetrics) {
	if metrics == nil || len(replacements) == 0 {
		return
	}
	earliest := len(messages)
	for _, replacement := range replacements {
		metrics.ReclaimedTokens += max(0, replacement.OriginalTokens-replacement.PlaceholderTokens)
		metrics.PlaceholderTokens += max(0, replacement.PlaceholderTokens)
		if replacement.MessageIndex >= 0 && replacement.MessageIndex < earliest {
			earliest = replacement.MessageIndex
		}
	}
	if earliest == len(messages) {
		return
	}
	metrics.EarliestChanged = earliest
	metrics.WarmSuffixTokens = EstimateMessages(messages[earliest+1:])
}

func pressureRatios(tokens, stablePrefix int, config StandardConfig) (pressure, full float64, budget int) {
	window := max(1, config.ContextWindowTokens)
	full = float64(tokens) / float64(window)
	if config.Scope == PressureTotal {
		return full, full, window
	}
	budget = max(1, window-stablePrefix)
	return float64(max(0, tokens-stablePrefix)) / float64(budget), full, budget
}

func stablePrefixTokens(messages []*agent.Message, inspection agent.ModelRequestInspection) int {
	prefix := EstimateInspectedTokens(nil, inspection)
	boundary := min(max(0, inspection.StablePrefixMessages), len(messages))
	for _, message := range messages[:boundary] {
		prefix += EstimateMessages([]*agent.Message{message})
	}
	return prefix
}

func latestPromptUsage(messages []*agent.Message) (int, CacheState) {
	for index := len(messages) - 1; index >= 0; index-- {
		message := messages[index]
		if message == nil || message.ResponseMeta == nil || message.ResponseMeta.Usage == nil || message.ResponseMeta.Usage.PromptTokens <= 0 {
			continue
		}
		if message.ResponseMeta.Usage.PromptTokenDetails.CachedTokens > 0 {
			return message.ResponseMeta.Usage.PromptTokens, CacheWarm
		}
		return message.ResponseMeta.Usage.PromptTokens, CacheUnknown
	}
	return 0, CacheUnknown
}

func validatePlan(messages []*agent.Message, plan agent.CleanupPlan) error {
	seen := make(map[int]struct{}, len(plan.Replacements))
	for index, replacement := range plan.Replacements {
		if replacement.MessageIndex < 0 || replacement.MessageIndex >= len(messages) {
			return fmt.Errorf("Cleanup replacement %d is outside the model request", index)
		}
		message := messages[replacement.MessageIndex]
		if message == nil || message.Role != agent.ToolRole || message.ToolCallID != replacement.ToolCallID || replacement.Placeholder == "" {
			return fmt.Errorf("Cleanup replacement %d does not match its tool result", index)
		}
		if _, duplicate := seen[replacement.MessageIndex]; duplicate {
			return errors.New("Cleanup plan contains duplicate replacement targets")
		}
		seen[replacement.MessageIndex] = struct{}{}
	}
	return nil
}

func flattenReplacements(groups []*interactionGroup) []agent.CleanupReplacement {
	var replacements []agent.CleanupReplacement
	for _, group := range groups {
		replacements = append(replacements, group.replacements...)
	}
	sort.Slice(replacements, func(left, right int) bool { return replacements[left].MessageIndex < replacements[right].MessageIndex })
	return replacements
}

var _ agent.CleanupManager = (*standardManager)(nil)
