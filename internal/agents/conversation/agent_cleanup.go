package conversation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	agent "github.com/alfredxw/denova/agent"

	"denova/config"
	agentcontext "denova/internal/agents/context"
)

// agentCleanupManager adapts Denova's established pressure policy to the
// public, storage-free Agent cleanup seam. Agent—not Writing Session or Game
// Story—owns projection durability and settlement.
type agentCleanupManager struct {
	cfg       *config.Config
	agentKind string
	identity  agent.CapabilityIdentity
}

func NewAgentCleanupManager(cfg *config.Config, agentKind string) agent.CleanupManager {
	settings := config.ResolveAgentContext(cfg, agentKind)
	if !settings.ToolResultContextEnabled {
		return nil
	}
	identityConfig := struct {
		AgentKind        string
		Context          config.ResolvedAgentContextSettings
		CleanupThreshold float64
		CleanupTarget    float64
		CleanupMinTokens int
		KeepRecentGroups int
		KeepRecentTokens int
		WarmSuffixTokens int
		EagerMinTokens   int
		Renderer         string
	}{
		agentKind, settings,
		config.DefaultToolResultCleanupThreshold, config.DefaultToolResultCleanupTarget,
		config.DefaultToolResultCleanupMinTokens, config.DefaultToolResultKeepRecent,
		config.DefaultToolResultKeepRecentTokens, config.DefaultToolResultWarmSuffixTokens,
		config.DefaultToolResultEagerMinTokens, agentcontext.ToolResultPlaceholderRendererVersion,
	}
	encoded, _ := json.Marshal(identityConfig)
	digest := sha256.Sum256(encoded)
	return &agentCleanupManager{cfg: cfg, agentKind: agentKind, identity: agent.CapabilityIdentity{
		Kind: "denova.cleanup", Version: 1, ConfigHash: hex.EncodeToString(digest[:]),
	}}
}

func (manager *agentCleanupManager) Identity() agent.CapabilityIdentity { return manager.identity }

func (manager *agentCleanupManager) Plan(_ context.Context, request agent.CleanupPlanRequest) (agent.CleanupPlan, error) {
	messages := request.ModelRequest
	policy := resolveContextPressurePolicy(manager.cfg, manager.agentKind, messages)
	policy.CompactionEnabled = request.CompactionAvailable
	options := request.ModelInspection.Options
	tools := options.Tools
	if options.MaxTokens != nil {
		policy.CheckpointOutputReserve = max(policy.CheckpointOutputReserve, *options.MaxTokens)
	}
	decision := agentcontext.PlanContextPressure(messages, tools, policy)
	plan := agent.CleanupPlan{
		Reason: decision.Reason, Renderer: decision.Cleanup.RendererVersion,
		FallbackToCompaction: request.CompactionAvailable &&
			(decision.Pressure >= policy.CompactionThreshold ||
				decision.FullPressure >= policy.CompactionThreshold ||
				decision.EffectiveTokens+max(0, policy.CompactionPromptTokens)+max(0, policy.CheckpointOutputReserve)+max(0, policy.SafetyMarginTokens) > policy.ContextWindowTokens),
		Metrics: agent.CleanupMetrics{
			EstimatedTokensBefore:      decision.EffectiveTokens,
			LocalProjectedTokens:       decision.LocalProjectedTokens,
			ObservedPromptTokens:       decision.ObservedPromptTokens,
			EffectiveTokens:            decision.EffectiveTokens,
			EstimatedTokensAfter:       decision.Cleanup.ProjectedTokensAfter,
			ReclaimedTokens:            decision.Cleanup.ReclaimedTokens,
			ContextWindowTokens:        policy.ContextWindowTokens,
			PressureBefore:             decision.FullPressure,
			PressureAfter:              decision.Cleanup.FullPressureAfter,
			BodyPressureBefore:         decision.Pressure,
			BodyPressureAfter:          decision.Cleanup.PressureAfter,
			StablePrefixTokens:         decision.StablePrefixTokens,
			CandidateTokens:            decision.CandidateTokens,
			CacheViableCandidateTokens: decision.CacheViableCandidateTokens,
			SkippedBelowMinimumCount:   decision.CleanupSkippedBelowMinimumCount,
			SkippedWarmSuffixCount:     decision.CleanupSkippedWarmSuffixCount,
			EagerCandidateCount:        decision.EagerCandidateCount,
			EagerSelectedCount:         decision.Cleanup.EagerGroupCount,
			SupersededCandidateCount:   decision.SupersededCount,
			DiscardableCandidateCount:  decision.DiscardableCount,
			MinimumCleanupTokens:       decision.MinimumCleanupTokens,
			ProtectedResults:           decision.ProtectedResultCount,
			EarliestChanged:            decision.Cleanup.EarliestChanged,
			WarmSuffixTokens:           decision.Cleanup.WarmSuffixTokens,
			PlaceholderTokens:          decision.Cleanup.PlaceholderTokens,
			ReplacementCount:           len(decision.Cleanup.Replacements),
			EagerOnly:                  decision.Cleanup.EagerOnly,
			PressureScope:              string(decision.Scope),
			ProviderCacheState:         string(decision.ProviderCacheState),
			ExecutionMode:              "agent_projection",
			RendererVersion:            decision.Cleanup.RendererVersion,
		},
	}
	if plan.Metrics.EstimatedTokensAfter <= 0 {
		plan.Metrics.EstimatedTokensAfter = plan.Metrics.EstimatedTokensBefore
		plan.Metrics.PressureAfter = plan.Metrics.PressureBefore
	}
	for _, replacement := range decision.Cleanup.Replacements {
		plan.Replacements = append(plan.Replacements, agent.CleanupReplacement{
			MessageIndex: replacement.MessageIndex, ToolCallID: replacement.ToolCallID,
			Placeholder: replacement.Placeholder, OriginalTokens: replacement.OriginalTokens,
			PlaceholderTokens: replacement.PlaceholderTokens,
		})
	}
	switch decision.Action {
	case agentcontext.ContextMaintenanceCleanup:
		plan.Action = agent.CleanupProject
	case agentcontext.ContextMaintenanceCompaction:
		plan.Action = agent.CleanupCompact
	default:
		plan.Action = agent.CleanupNone
	}
	return plan, nil
}

var _ agent.CleanupManager = (*agentCleanupManager)(nil)
