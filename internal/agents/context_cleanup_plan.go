package agents

import (
	"sort"

	agent "github.com/alfredxw/denova/agent"
)

type ToolResultCleanupReplacement struct {
	MessageIndex      int
	ToolCallID        string
	Placeholder       string
	OriginalTokens    int
	PlaceholderTokens int
}

type ToolResultCleanupPlan struct {
	Replacements         []ToolResultCleanupReplacement
	ReclaimedTokens      int
	PlaceholderTokens    int
	EarliestChanged      int
	WarmSuffixTokens     int
	ProjectedTokensAfter int
	PressureAfter        float64
	FullPressureAfter    float64
	RendererVersion      string
	EagerOnly            bool
	EagerGroupCount      int
}

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
			original := estimateMessageTokens(message)
			replacement := message.Clone()
			replacement.Content = rendered.Content
			placeholder := estimateMessageTokens(replacement)
			if placeholder >= original {
				group.protected = true
				group.replacements = nil
				break
			}
			group.replacements = append(group.replacements, ToolResultCleanupReplacement{
				MessageIndex: index, ToolCallID: message.ToolCallID, Placeholder: rendered.Content,
				OriginalTokens: original, PlaceholderTokens: placeholder,
			})
			group.reclaimed += original - placeholder
			group.placeholder += placeholder
		}
	}
}

func buildCleanupPlan(messages []*agent.Message, groups []*toolInteractionGroup, effectiveTokens, stablePrefix, budget int, policy ContextPressurePolicy, eagerOnly bool) (ToolResultCleanupPlan, bool) {
	if len(groups) == 0 {
		return ToolResultCleanupPlan{}, false
	}
	groups = cacheViableCleanupGroups(messages, groups, policy)
	if len(groups) == 0 {
		return ToolResultCleanupPlan{}, false
	}
	sort.SliceStable(groups, func(i, j int) bool {
		// Remove semantically obsolete transfer data before ordinary evidence.
		// The cache mutation gate still rejects an old candidate when changing its
		// suffix would be too expensive; within one semantic tier, prefer the
		// newest result to preserve the longest possible stable prefix.
		leftPriority := cleanupSemanticPriority(groups[i])
		rightPriority := cleanupSemanticPriority(groups[j])
		if leftPriority != rightPriority {
			return leftPriority < rightPriority
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
		return ToolResultCleanupPlan{}, false
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
		return ToolResultCleanupPlan{}, false
	}
	warmSuffix := EstimateContextTokens(messages[earliest+1:], nil)
	plan := ToolResultCleanupPlan{
		EarliestChanged: earliest, WarmSuffixTokens: warmSuffix,
		RendererVersion: toolResultPlaceholderRendererVersion, EagerOnly: eagerOnly,
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
		if earliest < len(messages) && EstimateContextTokens(messages[earliest+1:], nil) <= policy.WarmSuffixTokens {
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

// ApplyToolResultCleanupPlan applies a transient model projection. Persistence
// stores the same rendered placeholders in an append-only CleanupRecord.
func ApplyToolResultCleanupPlan(messages []*agent.Message, plan ToolResultCleanupPlan) []*agent.Message {
	if len(plan.Replacements) == 0 {
		return messages
	}
	result := append([]*agent.Message(nil), messages...)
	for _, replacement := range plan.Replacements {
		if replacement.MessageIndex < 0 || replacement.MessageIndex >= len(result) {
			continue
		}
		message := result[replacement.MessageIndex]
		if message == nil || message.Role != agent.ToolRole || message.ToolCallID != replacement.ToolCallID {
			continue
		}
		next := message.Clone()
		next.Content = replacement.Placeholder
		if next.ToolResult != nil {
			next.ToolResult.ContextHints = nil
			next.ToolResult.ResultRetention = agent.ToolResultProtected
		}
		result[replacement.MessageIndex] = next
	}
	return result
}
