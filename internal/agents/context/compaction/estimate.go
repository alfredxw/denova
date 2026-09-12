package compaction

import (
	agenttoolresult "github.com/alfredxw/denova/agent/toolresult"

	"denova/config"
	"denova/internal/agents/toolresult"
)

// EstimateProjectionReserves returns bounded reserves for completion and
// retained tool results. expectedOutputChars should be the user-configured
// target when one exists; otherwise a small model-relative reserve is used.
func EstimateProjectionReserves(cfg *config.Config, agentKind string, expectedOutputChars int) (completionTokens, toolResultTokens int) {
	model := config.ResolveAgentModel(cfg, agentKind)
	return EstimateProjectionReservesForModel(cfg, agentKind, expectedOutputChars, model.ContextWindowTokens)
}

// EstimateProjectionReservesForModel applies one product policy to the
// concrete model used by a Definition. Child Agents may inherit their parent's
// policy while using a different context window.
func EstimateProjectionReservesForModel(
	cfg *config.Config,
	agentKind string,
	expectedOutputChars int,
	window int,
) (completionTokens, toolResultTokens int) {
	completionTokens = expectedOutputChars
	if completionTokens <= 0 {
		completionTokens = max(2048, window/50)
	} else {
		// Leave room for the hidden structured result and normal completion
		// variance around the visible user-configured target.
		completionTokens += max(1024, expectedOutputChars/4)
	}
	if window > 0 {
		completionTokens = min(completionTokens, max(2048, window/4))
	}
	contextPolicy := config.ResolveAgentContext(cfg, agentKind)
	if contextPolicy.ToolResultContextEnabled {
		toolResultTokens = (agenttoolresult.Policy{MaxBytes: toolResultLimitBytes(cfg), ContextWindowTokens: window}).BatchTokenLimit()
	}
	return completionTokens, toolResultTokens
}

func toolResultLimitBytes(cfg *config.Config) int {
	limitKB := config.DefaultAgentToolResultLimitKB
	if cfg != nil && cfg.AgentToolResultLimitKB > 0 {
		limitKB = cfg.AgentToolResultLimitKB
	}
	return toolresult.NormalizeLimitBytes(limitKB * 1024)
}
