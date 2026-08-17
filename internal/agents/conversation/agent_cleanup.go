package conversation

import (
	"denova/config"
	agentcompaction "denova/internal/agents/context/compaction"

	agent "github.com/alfredxw/denova/agent"
	publiccleanup "github.com/alfredxw/denova/agent/cleanup"
)

// NewAgentCleanupManager applies Denova's product policy to the model selected
// for one root Agent. The public Standard manager remains the sole planner.
func NewAgentCleanupManager(cfg *config.Config, agentKind string) agent.CleanupManager {
	model := config.ResolveAgentModel(cfg, agentKind)
	return NewAgentCleanupManagerForModel(cfg, agentKind, model.ContextWindowTokens)
}

// NewAgentCleanupManagerForModel applies policy from policyKind while sizing
// every pressure and reserve decision for the concrete Definition model. Child
// Agents inherit product policy without inheriting the parent's model window.
func NewAgentCleanupManagerForModel(
	cfg *config.Config,
	policyKind string,
	contextWindowTokens int,
) agent.CleanupManager {
	settings := config.ResolveAgentContext(cfg, policyKind)
	if !settings.ToolResultContextEnabled {
		return nil
	}
	completionReserve, toolReserve := agentcompaction.EstimateProjectionReservesForModel(
		cfg, policyKind, 0, contextWindowTokens,
	)
	return publiccleanup.Standard(publiccleanup.StandardConfig{
		Scope:                   publiccleanup.PressureBodyAfterPrefix,
		ContextWindowTokens:     contextWindowTokens,
		ReservedTokens:          completionReserve + toolReserve,
		CleanupThreshold:        config.DefaultToolResultCleanupThreshold,
		CleanupTarget:           config.DefaultToolResultCleanupTarget,
		CleanupMinTokens:        config.DefaultToolResultCleanupMinTokens,
		KeepRecentGroups:        config.DefaultToolResultKeepRecent,
		KeepRecentTokens:        config.DefaultToolResultKeepRecentTokens,
		WarmSuffixTokens:        config.DefaultToolResultWarmSuffixTokens,
		EagerMinTokens:          config.DefaultToolResultEagerMinTokens,
		EagerMinContextRatio:    0.15,
		CompactionEnabled:       settings.CompactionEnabled,
		CompactionThreshold:     settings.CompactionThreshold,
		CompactionPromptTokens:  agentcompaction.ForkPromptReserve,
		CheckpointOutputReserve: max(1024, min(8192, contextWindowTokens/25)),
		SafetyMarginTokens:      max(512, contextWindowTokens/100),
	})
}
