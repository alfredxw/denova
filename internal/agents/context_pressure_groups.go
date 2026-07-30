package agents

import (
	"strings"

	agent "github.com/alfredxw/denova/agent"
)

// toolInteractionGroup keeps one assistant tool-call batch and its contiguous
// results atomic while the pressure planner evaluates retention and cleanup.
type toolInteractionGroup struct {
	start                int
	end                  int
	resultIndexes        []int
	resultTokens         int
	originalResultTokens int
	currentTurn          bool
	protected            bool
	eager                bool
	discardable          bool
	superseded           bool
	supersededBy         map[string]string
	replacements         []ToolResultCleanupReplacement
	reclaimed            int
	placeholder          int
}

func collectToolInteractionGroups(messages []*agent.Message, policy ContextPressurePolicy) ([]*toolInteractionGroup, int) {
	lastUser := -1
	for index, message := range messages {
		if message != nil && message.Role == agent.User {
			lastUser = index
		}
	}
	eagerMinimum := eagerToolResultMinimumTokens(policy.EagerMinTokens, policy.ContextWindowTokens, policy.EagerMinContextRatio)
	groups := make([]*toolInteractionGroup, 0)
	protectedResults := 0
	for index := 0; index < len(messages); index++ {
		message := messages[index]
		if message == nil || message.Role != agent.Assistant || len(message.ToolCalls) == 0 {
			continue
		}
		group := &toolInteractionGroup{start: index, end: index + 1, supersededBy: make(map[string]string)}
		calls := make(map[string]agent.ToolCall, len(message.ToolCalls))
		for _, call := range message.ToolCalls {
			id := strings.TrimSpace(call.ID)
			if id == "" || calls[id].ID != "" || validateToolArgumentsJSON(call.Function.Arguments) != nil {
				group.protected = true
				continue
			}
			calls[id] = call
		}
		seen := make(map[string]bool, len(calls))
		for group.end < len(messages) && messages[group.end] != nil && messages[group.end].Role == agent.ToolRole {
			result := messages[group.end]
			id := strings.TrimSpace(result.ToolCallID)
			if _, ok := calls[id]; !ok || seen[id] {
				group.protected = true
			} else {
				seen[id] = true
				group.resultIndexes = append(group.resultIndexes, group.end)
				tokens := estimateMessageTokens(result)
				group.resultTokens += tokens
				group.originalResultTokens += estimatedOriginalToolResultTokens(result)
				if toolResultProtected(result) {
					group.protected = true
					protectedResults++
				}
				if result.ToolResult != nil && result.ToolResult.ContextHints != nil {
					group.discardable = group.discardable || result.ToolResult.ContextHints.ContextValue == agent.ToolResultContextDiscardable
				}
			}
			group.end++
		}
		group.currentTurn = group.start > lastUser
		if len(seen) != len(calls) || len(calls) == 0 || group.currentTurn {
			group.protected = true
		}
		group.eager = !group.protected && group.originalResultTokens >= eagerMinimum && groupAllowsEager(messages, group)
		groups = append(groups, group)
		index = group.end - 1
	}
	return groups, protectedResults
}

func eagerToolResultMinimumTokens(configured, contextWindow int, ratio float64) int {
	if configured < 0 {
		configured = 0
	}
	if ratio <= 0 || ratio >= 1 {
		ratio = 0.15
	}
	return max(configured, int(float64(max(0, contextWindow))*ratio))
}

func estimatedOriginalToolResultTokens(message *agent.Message) int {
	if message == nil {
		return 0
	}
	if result := message.ToolResult; result != nil {
		if result.ContextHints != nil && result.ContextHints.Recovery.EstimatedTokens > 0 {
			return result.ContextHints.Recovery.EstimatedTokens
		}
		for _, artifact := range result.Artifacts {
			if artifact.EstimatedTokens > 0 {
				return artifact.EstimatedTokens
			}
		}
	}
	return estimateMessageTokens(message)
}

func toolResultProtected(message *agent.Message) bool {
	if message == nil || message.ToolResult == nil {
		return true
	}
	result := message.ToolResult
	if result.Status != agent.ToolResultSuccess || result.SyntheticReason == agent.ToolSyntheticEffectUnknown || effectiveToolResultRetention(result) == agent.ToolResultProtected {
		return true
	}
	if persistence := result.ArtifactPersistence; persistence != nil && persistence.Attempted && !persistence.Complete {
		return true
	}
	if result.ContextHints == nil {
		return true
	}
	recovery := result.ContextHints.Recovery
	return recovery.Kind == "" || !usableToolResultRecoveryForCleanup(recovery, result.Artifacts)
}

func groupAllowsEager(messages []*agent.Message, group *toolInteractionGroup) bool {
	if group == nil {
		return false
	}
	hasEagerResult := false
	for _, index := range group.resultIndexes {
		result := messages[index]
		if result != nil && result.ToolResult != nil && effectiveToolResultRetention(result.ToolResult) == agent.ToolResultEagerCandidate {
			hasEagerResult = true
		} else {
			return false
		}
	}
	if !hasEagerResult {
		return false
	}
	// A canonical assistant step after the result proves the source run reached
	// the post-tool model boundary. The current user message proves settlement.
	hasModelStep := false
	hasLaterUser := false
	for index := group.end; index < len(messages); index++ {
		message := messages[index]
		if message == nil {
			continue
		}
		if message.Role == agent.Assistant && len(message.ToolCalls) == 0 {
			hasModelStep = true
		}
		if hasModelStep && message.Role == agent.User {
			hasLaterUser = true
			break
		}
	}
	return hasModelStep && hasLaterUser
}

func markSupersededGroups(messages []*agent.Message, groups []*toolInteractionGroup) {
	latest := make(map[string]string)
	for groupIndex := len(groups) - 1; groupIndex >= 0; groupIndex-- {
		group := groups[groupIndex]
		for _, index := range group.resultIndexes {
			message := messages[index]
			if message == nil || message.ToolResult == nil || message.ToolResult.ContextHints == nil {
				continue
			}
			key := strings.TrimSpace(message.ToolResult.ContextHints.SupersessionKey)
			if key == "" || message.ToolResult.Status != agent.ToolResultSuccess {
				continue
			}
			if newer := latest[key]; newer != "" {
				group.superseded = true
				group.supersededBy[message.ToolCallID] = newer
			} else {
				latest[key] = message.ToolCallID
			}
		}
	}
}

func protectRecentToolGroups(groups []*toolInteractionGroup, policy ContextPressurePolicy) {
	protectedTokens := 0
	protectedGroups := 0
	for index := len(groups) - 1; index >= 0; index-- {
		group := groups[index]
		if group.currentTurn {
			continue
		}
		if group.eager {
			// Eager candidates already passed stricter size, recovery and settled-
			// run gates; they intentionally bypass ordinary recent-group aging.
			continue
		}
		if protectedGroups < policy.KeepRecentGroups || protectedTokens < policy.KeepRecentTokens {
			group.protected = true
			protectedGroups++
			protectedTokens += group.resultTokens
			continue
		}
		break
	}
}

func effectiveToolResultRetention(result *agent.ToolResultSummary) agent.ToolResultRetentionMode {
	if result == nil {
		return ""
	}
	if result.ResultRetention != "" {
		return result.ResultRetention
	}
	switch result.ContextRetention {
	case agent.ToolContextTransient, agent.ToolContextReceipt:
		return agent.ToolResultDeferred
	default:
		return ""
	}
}
