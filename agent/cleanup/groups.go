package cleanup

import (
	"encoding/json"
	"io"
	"sort"
	"strings"

	agent "github.com/alfredxw/denova/agent"
)

type interactionGroup struct {
	start, end       int
	resultIndexes    []int
	resultTokens     int
	protected        bool
	awaitingModel    bool
	eager            bool
	largeRecoverable bool
	discardable      bool
	superseded       bool
	supersededBy     map[string]string
	replacements     []agent.CleanupReplacement
	reclaimed        int
}

func collectGroups(messages []*agent.Message, config StandardConfig) ([]*interactionGroup, int) {
	eagerMinimum := max(config.EagerMinTokens, int(float64(config.ContextWindowTokens)*config.EagerMinContextRatio))
	groups := make([]*interactionGroup, 0)
	protectedResults := 0
	for index := 0; index < len(messages); index++ {
		message := messages[index]
		if message == nil || message.Role != agent.Assistant || len(message.ToolCalls) == 0 {
			continue
		}
		group := &interactionGroup{start: index, end: index + 1, supersededBy: make(map[string]string)}
		calls := make(map[string]agent.ToolCall, len(message.ToolCalls))
		for _, call := range message.ToolCalls {
			id := strings.TrimSpace(call.ID)
			if id == "" || calls[id].ID != "" || !validArguments(call.Function.Arguments) {
				group.protected = true
				continue
			}
			calls[id] = call
		}
		seen := make(map[string]bool, len(calls))
		for group.end < len(messages) && messages[group.end] != nil && messages[group.end].Role == agent.ToolRole {
			result := messages[group.end]
			id := strings.TrimSpace(result.ToolCallID)
			if _, known := calls[id]; !known || seen[id] {
				group.protected = true
			} else {
				seen[id] = true
				group.resultIndexes = append(group.resultIndexes, group.end)
				group.resultTokens += EstimateMessages([]*agent.Message{result})
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
		group.awaitingModel = !hasLaterAssistant(messages, group.end)
		if len(calls) == 0 || len(seen) != len(calls) || group.awaitingModel {
			group.protected = true
		}
		originalTokens := 0
		for _, resultIndex := range group.resultIndexes {
			originalTokens += originalResultTokens(messages[resultIndex])
		}
		group.largeRecoverable = !group.protected && originalTokens >= eagerMinimum
		group.eager = group.largeRecoverable && groupAllowsEager(messages, group)
		groups = append(groups, group)
		index = group.end - 1
	}
	markSuperseded(messages, groups)
	protectRecent(groups, config)
	return groups, protectedResults
}

func validArguments(arguments string) bool {
	arguments = strings.TrimSpace(arguments)
	if arguments == "" {
		return true
	}
	decoder := json.NewDecoder(strings.NewReader(arguments))
	decoder.UseNumber()
	var value map[string]any
	if err := decoder.Decode(&value); err != nil || value == nil {
		return false
	}
	var trailing any
	return decoder.Decode(&trailing) == io.EOF
}

func hasLaterAssistant(messages []*agent.Message, start int) bool {
	for _, message := range messages[start:] {
		if message != nil && message.Role == agent.Assistant {
			return true
		}
	}
	return false
}

func originalResultTokens(message *agent.Message) int {
	if message != nil && message.ToolResult != nil {
		if hints := message.ToolResult.ContextHints; hints != nil && hints.Recovery.EstimatedTokens > 0 {
			return hints.Recovery.EstimatedTokens
		}
		for _, artifact := range message.ToolResult.Artifacts {
			if artifact.EstimatedTokens > 0 {
				return artifact.EstimatedTokens
			}
		}
	}
	return EstimateMessages([]*agent.Message{message})
}

func toolResultProtected(message *agent.Message) bool {
	if message == nil || message.ToolResult == nil {
		return true
	}
	result := message.ToolResult
	if result.Status != agent.ToolResultSuccess || result.SyntheticReason == agent.ToolSyntheticEffectUnknown || result.ResultRetention == agent.ToolResultProtected {
		return true
	}
	if persistence := result.ArtifactPersistence; persistence != nil && persistence.Attempted && !persistence.Complete {
		return true
	}
	return result.ContextHints == nil || !usableRecovery(result.ContextHints.Recovery, result.Artifacts)
}

func groupAllowsEager(messages []*agent.Message, group *interactionGroup) bool {
	if group == nil || len(group.resultIndexes) == 0 {
		return false
	}
	for _, index := range group.resultIndexes {
		if result := messages[index]; result == nil || result.ToolResult == nil || result.ToolResult.ResultRetention != agent.ToolResultEagerCandidate {
			return false
		}
	}
	modelStep := false
	for _, message := range messages[group.end:] {
		if message == nil {
			continue
		}
		if message.Role == agent.Assistant && len(message.ToolCalls) == 0 {
			modelStep = true
		}
		if modelStep && message.Role == agent.User {
			return true
		}
	}
	return false
}

func markSuperseded(messages []*agent.Message, groups []*interactionGroup) {
	latest := make(map[string]string)
	for index := len(groups) - 1; index >= 0; index-- {
		group := groups[index]
		for _, resultIndex := range group.resultIndexes {
			message := messages[resultIndex]
			if message == nil || message.ToolResult == nil || message.ToolResult.ContextHints == nil || message.ToolResult.Status != agent.ToolResultSuccess {
				continue
			}
			key := strings.TrimSpace(message.ToolResult.ContextHints.SupersessionKey)
			if key == "" {
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

func protectRecent(groups []*interactionGroup, config StandardConfig) {
	groupsKept, tokensKept := 0, 0
	for index := len(groups) - 1; index >= 0; index-- {
		group := groups[index]
		if group.awaitingModel || group.eager || group.largeRecoverable {
			continue
		}
		if groupsKept < config.KeepRecentGroups || tokensKept < config.KeepRecentTokens {
			group.protected = true
			groupsKept++
			tokensKept += group.resultTokens
			continue
		}
		break
	}
}

func prepareReplacements(messages []*agent.Message, groups []*interactionGroup) {
	for _, group := range groups {
		if group.protected {
			continue
		}
		calls := make(map[string]agent.ToolCall)
		for _, call := range messages[group.start].ToolCalls {
			calls[call.ID] = call
		}
		for _, index := range group.resultIndexes {
			message := messages[index]
			placeholder, ok := renderPlaceholder(calls[message.ToolCallID], message, group.supersededBy[message.ToolCallID])
			if !ok {
				group.protected = true
				group.replacements = nil
				group.reclaimed = 0
				break
			}
			original := EstimateMessages([]*agent.Message{message})
			replacementMessage := message.Clone()
			replacementMessage.Content = placeholder
			replacement := EstimateMessages([]*agent.Message{replacementMessage})
			if replacement >= original {
				group.protected = true
				group.replacements = nil
				group.reclaimed = 0
				break
			}
			group.replacements = append(group.replacements, agent.CleanupReplacement{
				MessageIndex: index, ToolCallID: message.ToolCallID, Placeholder: placeholder,
				OriginalTokens: original, PlaceholderTokens: replacement,
			})
			group.reclaimed += original - replacement
		}
	}
}

func selectGroups(messages []*agent.Message, groups []*interactionGroup, effective, stablePrefix, budget int, config StandardConfig, eagerOnly bool) []*interactionGroup {
	viable := cacheViableGroups(messages, groups, config, eagerOnly)
	sort.SliceStable(viable, func(left, right int) bool {
		leftPriority, rightPriority := semanticPriority(viable[left]), semanticPriority(viable[right])
		if leftPriority != rightPriority {
			return leftPriority < rightPriority
		}
		if viable[left].reclaimed != viable[right].reclaimed {
			return viable[left].reclaimed > viable[right].reclaimed
		}
		return viable[left].start > viable[right].start
	})
	target := int(float64(budget) * config.CleanupTarget)
	if config.Scope == PressureBodyAfterPrefix {
		target += stablePrefix
	}
	required := max(max(1, effective-target), max(config.CleanupMinTokens, budget/10))
	selected, reclaimed := make([]*interactionGroup, 0, len(viable)), 0
	for _, group := range viable {
		selected = append(selected, group)
		reclaimed += group.reclaimed
		if !eagerOnly && reclaimed >= required {
			break
		}
	}
	return selected
}

func cacheViableGroups(messages []*agent.Message, groups []*interactionGroup, config StandardConfig, eagerOnly bool) []*interactionGroup {
	viable := make([]*interactionGroup, 0, len(groups))
	for _, group := range groups {
		if group.protected || group.reclaimed <= 0 || eagerOnly && !group.eager {
			continue
		}
		earliest := group.replacements[0].MessageIndex
		if config.CacheState != CacheCold && EstimateMessages(messages[earliest+1:]) > config.WarmSuffixTokens {
			continue
		}
		viable = append(viable, group)
	}
	return viable
}

func semanticPriority(group *interactionGroup) int {
	if group.superseded {
		return 0
	}
	if group.discardable {
		return 1
	}
	return 2
}

func selectedOnlyEager(groups []*interactionGroup) bool {
	if len(groups) == 0 {
		return false
	}
	for _, group := range groups {
		if !group.eager {
			return false
		}
	}
	return true
}
