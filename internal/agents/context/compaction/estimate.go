package compaction

import (
	agent "github.com/alfredxw/denova/agent"

	"denova/config"
	agentcontext "denova/internal/agents/context"
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
		// A result is bounded at the tool boundary before it is persisted. Reserve
		// for one such result; older exchanges are owned by normal compaction.
		toolResultTokens = toolResultLimitBytes(cfg) / 3
		if window > 0 {
			toolResultTokens = min(toolResultTokens, max(1024, window/10))
		}
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

func withDefaultContextProjectionReserves(cfg *config.Config, agentKind string, input Input, expectedOutputChars int) Input {
	completion, tools := EstimateProjectionReserves(cfg, agentKind, expectedOutputChars)
	if input.ReservedCompletionTokens <= 0 {
		input.ReservedCompletionTokens = completion
	}
	if input.ReservedToolResultTokens <= 0 {
		input.ReservedToolResultTokens = tools
	}
	return input
}

func projectedContextTokens(promptTokens int, input Input) int {
	return max(1, promptTokens+max(0, input.ReservedCompletionTokens)+max(0, input.ReservedToolResultTokens))
}

// calibratedContextTokens uses provider usage only as a one-way correction
// measured on the exact previous request. The current assembly is still
// projected locally, so newly added tool results and completion reserves
// cannot be mistaken for usage the provider has not observed yet.
func calibratedContextTokens(estimated int, input Input) int {
	return (agent.CompactionMetrics{
		ObservedPromptTokens:   input.ObservedPromptTokens,
		ObservedEstimateTokens: input.ObservedEstimateTokens,
	}).CalibratedTokens(estimated)
}

// RecalculateProjection applies the same provider/local
// calibration and completion/tool reserves used by Prepare to
// an exact post-compaction message estimate. Domain conversations must call it
// after re-injecting stable provider-visible state.
func RecalculateProjection(result Result, estimatedPromptTokens int) Result {
	input := Input{
		ObservedPromptTokens:     result.ObservedPromptTokens,
		ObservedEstimateTokens:   result.ObservedEstimateTokens,
		ReservedCompletionTokens: result.ReservedCompletionTokens,
		ReservedToolResultTokens: result.ReservedToolResultTokens,
	}
	result.TokensAfter = calibratedContextTokens(estimatedPromptTokens, input)
	result.ProjectedTokensAfter = projectedContextTokens(result.TokensAfter, input)
	applyContextCompactionRecovery(&result)
	return result
}

// LatestPromptUsageCalibration returns the newest exact provider/local token
// pair suitable for calibrating the next context projection.
func LatestPromptUsageCalibration(messages []*agent.Message, tools []*agent.ToolInfo) (observed, estimated int) {
	for index := len(messages) - 1; index >= 0; index-- {
		message := messages[index]
		if message == nil || message.ResponseMeta == nil || message.ResponseMeta.Usage == nil || message.ResponseMeta.Usage.PromptTokens <= 0 {
			continue
		}
		return message.ResponseMeta.Usage.PromptTokens, agentcontext.EstimateTokens(messages[:index], tools)
	}
	return 0, 0
}

func compactionSourceBaseMessages(input Input) []*agent.Message {
	if input.SourceMessagesSet || len(input.SourceMessages) > 0 {
		return input.SourceMessages
	}
	return input.Messages
}
