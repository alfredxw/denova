package chat

import (
	"fmt"
	"strings"

	agent "github.com/alfredxw/denova/agent"
	agentcleanup "github.com/alfredxw/denova/agent/cleanup"

	"denova/config"
	agentcontext "denova/internal/agents/context"
	agentcompaction "denova/internal/agents/context/compaction"
	"denova/internal/agents/prompts"
)

// BuildInspectedContextAnalysis projects the public Agent Session's exact,
// middleware-final provider request into Denova's read-only diagnostics DTO.
// It never reconstructs history or maintenance state from product journals.
func BuildInspectedContextAnalysis(
	cfg *config.Config,
	agentKind, mode string,
	composition prompts.SystemPromptComposition,
	inspection agent.Inspection,
) ContextAnalysis {
	messages := inspection.ModelRequest.Messages
	stablePrefix := min(max(0, inspection.ModelRequest.StablePrefixMessages), len(messages))
	systemMessages := make([]*agent.Message, 0, 1)
	contextMessages := make([]ContextAnalysisPart, 0, len(messages))
	for index, message := range messages {
		if message == nil {
			continue
		}
		if message.Role == agent.System {
			systemMessages = append(systemMessages, message)
			continue
		}
		source := "Conversation history / 会话历史"
		title := fmt.Sprintf("Model message %d / 模型消息 %d", index+1, index+1)
		switch {
		case agentcontext.IsCompactionSummaryMessage(message):
			source = "Compaction / 上下文压缩"
			title = "Model-visible history checkpoint / 模型可见历史检查点"
		case index < stablePrefix:
			source = "Stable context / 稳定上下文"
			title = "Stable model-prefix message / 稳定模型前缀消息"
		case index == len(messages)-1:
			source = "Current turn / 本轮上下文"
			title = "Current user message / 本轮用户消息"
		}
		contextMessages = append(contextMessages, contextAnalysisPartFromMessage(
			fmt.Sprintf("model_message_%d", index+1), source, title, message,
		))
	}

	systemPrompt, systemParts := inspectedSystemPrompt(composition, systemMessages)
	tokens := agentcleanup.EstimateInspectedTokens(messages, inspection.ModelRequest)
	completionReserve, toolResultReserve := agentcompaction.EstimateProjectionReserves(cfg, agentKind, 0)
	if inspection.ModelRequest.Options.MaxTokens != nil {
		completionReserve = max(0, *inspection.ModelRequest.Options.MaxTokens)
	}
	window := config.ResolveAgentModel(cfg, agentKind).ContextWindowTokens
	threshold := config.ResolveAgentContext(cfg, agentKind).CompactionThreshold
	if inspection.Compaction != nil {
		if inspection.Compaction.Metrics.ContextWindowTokens > 0 {
			window = inspection.Compaction.Metrics.ContextWindowTokens
		}
		if inspection.Compaction.Metrics.Threshold > 0 {
			threshold = inspection.Compaction.Metrics.Threshold
		}
	}
	projected := tokens + completionReserve + toolResultReserve
	ratio := 0.0
	if window > 0 {
		ratio = float64(projected) / float64(window)
	}
	compaction := contextAnalysisCompactionFromInspection(inspection.Compaction)
	parts := inspectedContextProvenanceParts(inspection)
	if len(parts) == 0 {
		parts = append([]ContextAnalysisPart(nil), contextMessages...)
	}
	return ContextAnalysis{
		AgentKind: agentKind, Mode: mode,
		SystemPrompt: systemPrompt, SystemPromptParts: systemParts,
		ContextParts: parts, ContextMessages: contextMessages, MessageCount: len(contextMessages),
		TokenEstimate: tokens, ProjectedTokenEstimate: projected,
		ReservedCompletionTokens: completionReserve, ReservedToolResultTokens: toolResultReserve,
		ContextWindowTokens: window, ContextUsageRatio: ratio,
		CompactionEpoch:  inspectionCompactionEpoch(inspection.Compaction),
		CompactionActive: inspection.Compaction != nil,
		WouldCompact:     window > 0 && threshold > 0 && ratio >= threshold,
		Compaction:       compaction,
	}
}

// inspectedContextProvenanceParts explains the exact model request without
// re-running product context assembly. A fragment is reported only when its
// materialized content is still present after cleanup, compaction, and caller
// middleware; ContextMessages remains the byte-for-byte provider request.
func inspectedContextProvenanceParts(inspection agent.Inspection) []ContextAnalysisPart {
	parts := make([]ContextAnalysisPart, 0, len(inspection.ContextFragments))
	for _, fragment := range inspection.ContextFragments {
		content := strings.TrimSpace(fragment.Content)
		if content == "" || !inspectionContainsContextContent(inspection.ModelRequest.Messages, content) {
			continue
		}
		note := fmt.Sprintf(
			"purpose=%s; revision=%s; placement=%s; hard_limit_bytes=%d",
			strings.TrimSpace(fragment.Purpose), strings.TrimSpace(fragment.Revision),
			fragment.Placement, fragment.HardLimit,
		)
		parts = append(parts, NewContextAnalysisPart(ContextAnalysisPartInput{
			ID: strings.TrimSpace(fragment.Resource), Source: strings.TrimSpace(fragment.Source),
			Title: strings.TrimSpace(fragment.Resource), Content: fragment.Content, Note: note,
		}))
	}
	return parts
}

func inspectionContainsContextContent(messages []*agent.Message, content string) bool {
	for _, message := range messages {
		if message != nil && strings.Contains(message.Content, content) {
			return true
		}
	}
	return false
}

// BuildInteractiveInspectedContextAnalysis preserves Game's source-oriented
// diagnostics while taking every model-visible byte from public Session
// inspection. Product code may explain the final structured turn prompt, but
// it must never rebuild history, Cleanup, or Compaction independently.
func BuildInteractiveInspectedContextAnalysis(
	cfg *config.Config,
	composition prompts.SystemPromptComposition,
	inspection agent.Inspection,
) ContextAnalysis {
	analysis := BuildInspectedContextAnalysis(
		cfg, config.AgentKindInteractiveStory, "interactive", composition, inspection,
	)
	if len(analysis.ContextMessages) == 0 {
		return analysis
	}
	last := len(analysis.ContextMessages) - 1
	analysis.ContextMessages[last].Source = "Current interactive turn / 本轮互动"
	analysis.ContextMessages[last].Title = "Interactive instruction and dynamic context / 互动指令与动态上下文"
	analysis.ContextMessages[last].Parts = buildInteractiveStoryInstructionContextParts(
		analysis.ContextMessages[last].Content,
	)
	analysis.ContextParts = append([]ContextAnalysisPart(nil), analysis.ContextMessages...)
	return analysis
}

// BuildInteractiveDirectorInspectedContextAnalysis annotates the exact public
// Director request without rebuilding its resident Lore or current planning
// instruction. The public Session remains the only history/maintenance view.
func BuildInteractiveDirectorInspectedContextAnalysis(
	cfg *config.Config,
	composition prompts.SystemPromptComposition,
	inspection agent.Inspection,
) ContextAnalysis {
	analysis := BuildInspectedContextAnalysis(
		cfg, config.AgentKindInteractiveDirector, "interactive_director", composition, inspection,
	)
	if len(analysis.ContextMessages) == 0 {
		return analysis
	}
	last := len(analysis.ContextMessages) - 1
	for index := 0; index < last; index++ {
		if analysis.ContextMessages[index].Source != "Stable context / 稳定上下文" {
			continue
		}
		analysis.ContextMessages[index].Source = "Resident lore / 常驻资料"
		analysis.ContextMessages[index].Title = "Enabled resident lore / 已启用常驻资料"
	}
	analysis.ContextMessages[last].Source = "Current Director task / 当前导演任务"
	analysis.ContextMessages[last].Title = "Director maintenance instruction / 导演维护指令"
	analysis.ContextMessages[last].Parts = buildInteractiveDirectorInstructionContextParts(
		analysis.ContextMessages[last].Content,
	)
	analysis.ContextParts = append([]ContextAnalysisPart(nil), analysis.ContextMessages...)
	return analysis
}

func inspectedSystemPrompt(
	composition prompts.SystemPromptComposition,
	messages []*agent.Message,
) (string, []ContextAnalysisPart) {
	contents := make([]string, 0, len(messages))
	for _, message := range messages {
		if message != nil {
			contents = append(contents, message.Content)
		}
	}
	prompt := strings.Join(contents, "\n\n")
	if len(messages) == 1 && composition.Instruction() == messages[0].Content {
		return prompt, systemPromptAnalysisParts(composition)
	}
	parts := make([]ContextAnalysisPart, 0, len(messages))
	for index, message := range messages {
		parts = append(parts, contextAnalysisPartFromMessage(
			fmt.Sprintf("system_message_%d", index+1),
			"Exact model request / 精确模型请求",
			fmt.Sprintf("System message %d / 系统消息 %d", index+1, index+1),
			message,
		))
	}
	return prompt, parts
}

func inspectionCompactionEpoch(compaction *agent.CompactionState) int {
	if compaction == nil {
		return 0
	}
	return int(compaction.Revision)
}

func contextAnalysisCompactionFromInspection(compaction *agent.CompactionState) *ContextAnalysisCompaction {
	if compaction == nil {
		return nil
	}
	metrics := compaction.Metrics
	tokensAfter := metrics.ProjectedTokensAfter
	if tokensAfter <= 0 {
		tokensAfter = compaction.TokenEstimate
	}
	sourceCount := metrics.SourceMessageCount
	if sourceCount <= 0 {
		sourceCount = max(0, compaction.ReplacementTo-compaction.ReplacementFrom)
	}
	return &ContextAnalysisCompaction{
		ID: compaction.ID, Epoch: int(compaction.Revision), Summary: compaction.Summary,
		TokensBefore: metrics.ProjectedTokensBefore, TokensAfter: tokensAfter,
		TargetRatio: metrics.RecoveryBand, SourceMessageCount: sourceCount,
		Removable: true,
	}
}
