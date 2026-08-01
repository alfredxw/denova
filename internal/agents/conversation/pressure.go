package conversation

import (
	agent "github.com/alfredxw/denova/agent"

	"denova/config"
	agentcontext "denova/internal/agents/context"
	agentcompaction "denova/internal/agents/context/compaction"
)

func resolveContextPressurePolicy(cfg *config.Config, agentKind string, messages []*agent.Message) agentcontext.ContextPressurePolicy {
	settings := config.ResolveAgentContext(cfg, agentKind)
	model := config.ResolveAgentModel(cfg, agentKind)
	completionReserve, toolReserve := agentcompaction.EstimateProjectionReserves(cfg, agentKind, 0)
	observed, _ := agentcompaction.LatestPromptUsageCalibration(messages, nil)
	policy := agentcontext.ContextPressurePolicy{
		AgentKind:               agentKind,
		Enabled:                 settings.CompactionEnabled || settings.ToolResultContextEnabled,
		CompactionEnabled:       settings.CompactionEnabled,
		CleanupEnabled:          settings.ToolResultContextEnabled,
		Scope:                   agentcontext.ContextPressureBodyAfterPrefix,
		ContextWindowTokens:     model.ContextWindowTokens,
		ReservedTokens:          completionReserve + toolReserve,
		CleanupThreshold:        config.DefaultToolResultCleanupThreshold,
		CleanupTarget:           config.DefaultToolResultCleanupTarget,
		CleanupMinTokens:        config.DefaultToolResultCleanupMinTokens,
		KeepRecentGroups:        config.DefaultToolResultKeepRecent,
		KeepRecentTokens:        config.DefaultToolResultKeepRecentTokens,
		WarmSuffixTokens:        config.DefaultToolResultWarmSuffixTokens,
		EagerMinTokens:          config.DefaultToolResultEagerMinTokens,
		EagerMinContextRatio:    0.15,
		CompactionThreshold:     settings.CompactionThreshold,
		CompactionRecoveryBand:  config.DefaultContextCompactionRecoveryBand,
		CompactionPromptTokens:  agentcompaction.ForkPromptReserve,
		CheckpointOutputReserve: max(1024, min(8192, model.ContextWindowTokens/25)),
		SafetyMarginTokens:      max(512, model.ContextWindowTokens/100),
		ProviderCacheState:      agentcontext.ProviderCacheStateFromMessages(messages),
		ObservedPromptTokens:    observed,
	}
	return WithDynamicPressurePolicy(policy, messages)
}

// WithDynamicPressurePolicy accounts for the exact fork prompt of the current
// provider-visible messages without mutating the base configuration policy.
func WithDynamicPressurePolicy(policy agentcontext.ContextPressurePolicy, messages []*agent.Message) agentcontext.ContextPressurePolicy {
	forkPolicy := agentcompaction.Policy{
		AgentKind: policy.AgentKind, ContextWindowTokens: policy.ContextWindowTokens,
		CheckpointOutputReserve: policy.CheckpointOutputReserve,
	}
	policy.CompactionPromptTokens = max(policy.CompactionPromptTokens, agentcompaction.EstimateForkPromptTokens(messages, forkPolicy))
	return policy
}

// ResolvePressurePolicy projects product configuration
// into the storage-neutral planner policy shared by writing and game adapters.
func ResolvePressurePolicy(cfg *config.Config, agentKind string, messages []*agent.Message) agentcontext.ContextPressurePolicy {
	return resolveContextPressurePolicy(cfg, agentKind, messages)
}
