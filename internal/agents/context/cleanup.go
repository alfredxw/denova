package context

import (
	"sort"

	agent "github.com/alfredxw/denova/agent"

	"denova/internal/agents/toolresult"
)

func prepareCleanupReplacements(messages []*agent.Message, groups []*toolInteractionGroup) {
	for _, group := range groups {
		if group.protected {
			continue
		}
		callByID := make(map[string]agent.ToolCall)
		for _, call := range messages[group.start].ToolCalls {
			callByID[call.ID] = call
		}
		for _, index := range group.resultIndexes {
			message := messages[index]
			rendered, ok := renderToolResultPlaceholder(callByID[message.ToolCallID], message, group.supersededBy[message.ToolCallID])
			if !ok {
				group.protected = true
				group.replacements = nil
				break
			}
			original := EstimateMessageTokens(message)
			replacement := message.Clone()
			replacement.Content = rendered.Content
			placeholder := EstimateMessageTokens(replacement)
			if placeholder >= original {
				group.protected = true
				group.replacements = nil
				break
			}
			group.replacements = append(group.replacements, toolresult.CleanupReplacement{
				MessageIndex: index, ToolCallID: message.ToolCallID, Placeholder: rendered.Content,
				OriginalTokens: original, PlaceholderTokens: placeholder,
			})
			group.reclaimed += original - placeholder
			group.placeholder += placeholder
		}
	}
}

func buildCleanupPlan(messages []*agent.Message, groups []*toolInteractionGroup, effectiveTokens, stablePrefix, budget int, policy ContextPressurePolicy, eagerOnly bool) (toolresult.CleanupPlan, bool) {
	if len(groups) == 0 {
		return toolresult.CleanupPlan{}, false
	}
	groups = cacheViableCleanupGroups(messages, groups, policy)
	if len(groups) == 0 {
		return toolresult.CleanupPlan{}, false
	}
	sort.SliceStable(groups, func(i, j int) bool {
		// Remove semantically obsolete transfer data before ordinary evidence.
		// The cache mutation gate still rejects an old candidate when changing its
		// suffix would be too expensive; within one semantic tier, reclaim the
		// largest result first and use recency only as the tie-breaker.
		leftPriority := cleanupSemanticPriority(groups[i])
		rightPriority := cleanupSemanticPriority(groups[j])
		if leftPriority != rightPriority {
			return leftPriority < rightPriority
		}
		if groups[i].reclaimed != groups[j].reclaimed {
			return groups[i].reclaimed > groups[j].reclaimed
		}
		return groups[i].start > groups[j].start
	})
	targetTokens := int(float64(budget) * policy.CleanupTarget)
	if policy.Scope == ContextPressureBodyAfterPrefix {
		targetTokens += stablePrefix
	}
	required := max(max(1, effectiveTokens-targetTokens), max(policy.CleanupMinTokens, budget/10))
	selected := make([]*toolInteractionGroup, 0, len(groups))
	reclaimed := 0
	for _, group := range groups {
		selected = append(selected, group)
		reclaimed += group.reclaimed
		if !eagerOnly && reclaimed >= required {
			break
		}
	}
	if len(selected) == 0 {
		return toolresult.CleanupPlan{}, false
	}
	earliest := len(messages)
	for _, group := range selected {
		for _, replacement := range group.replacements {
			if replacement.MessageIndex < earliest {
				earliest = replacement.MessageIndex
			}
		}
	}
	if earliest >= len(messages) {
		return toolresult.CleanupPlan{}, false
	}
	warmSuffix := EstimateTokens(messages[earliest+1:], nil)
	plan := toolresult.CleanupPlan{
		EarliestChanged: earliest, WarmSuffixTokens: warmSuffix,
		RendererVersion: ToolResultPlaceholderRendererVersion, EagerOnly: eagerOnly,
	}
	for _, group := range selected {
		plan.Replacements = append(plan.Replacements, group.replacements...)
		plan.ReclaimedTokens += group.reclaimed
		plan.PlaceholderTokens += group.placeholder
		if group.eager {
			plan.EagerGroupCount++
		}
	}
	sort.Slice(plan.Replacements, func(i, j int) bool { return plan.Replacements[i].MessageIndex < plan.Replacements[j].MessageIndex })
	plan.ProjectedTokensAfter = max(1, effectiveTokens-plan.ReclaimedTokens)
	plan.FullPressureAfter = float64(plan.ProjectedTokensAfter) / float64(max(1, policy.ContextWindowTokens))
	if policy.Scope == ContextPressureTotal {
		plan.PressureAfter = plan.FullPressureAfter
	} else {
		plan.PressureAfter = float64(max(0, plan.ProjectedTokensAfter-stablePrefix)) / float64(max(1, budget))
	}
	return plan, true
}

// cacheViableCleanupGroups applies the mutation-cost boundary before semantic
// ranking. Otherwise one obsolete but very old result can poison the whole
// batch even when a newer group can independently establish the cleanup
// waterline without rewriting a deep warm prefix. Any combination of the
// retained groups remains inside the same suffix budget because its earliest
// member has already passed this monotonic gate.
func cacheViableCleanupGroups(messages []*agent.Message, groups []*toolInteractionGroup, policy ContextPressurePolicy) []*toolInteractionGroup {
	if policy.CleanupExecutionMode == ToolResultCleanupNativeCacheEdit || policy.ProviderCacheState == ProviderCacheCold {
		return groups
	}
	viable := make([]*toolInteractionGroup, 0, len(groups))
	for _, group := range groups {
		if group == nil || len(group.replacements) == 0 {
			continue
		}
		earliest := len(messages)
		for _, replacement := range group.replacements {
			if replacement.MessageIndex < earliest {
				earliest = replacement.MessageIndex
			}
		}
		if earliest < len(messages) && EstimateTokens(messages[earliest+1:], nil) <= policy.WarmSuffixTokens {
			viable = append(viable, group)
		}
	}
	return viable
}

func cleanupSemanticPriority(group *toolInteractionGroup) int {
	if group == nil {
		return 3
	}
	if group.superseded {
		return 0
	}
	if group.discardable {
		return 1
	}
	return 2
}
