package context

import (
	"encoding/json"
	"io"
	"strings"

	agent "github.com/alfredxw/denova/agent"

	"denova/internal/agents/toolresult"
)

// toolInteractionGroup keeps one assistant tool-call batch and its contiguous
// results atomic while the pressure planner evaluates retention and cleanup.
type toolInteractionGroup struct {
	start                int
	end                  int
	resultIndexes        []int
	resultTokens         int
	originalResultTokens int
	awaitingModelStep    bool
	protected            bool
	largeRecoverable     bool
	eager                bool
	discardable          bool
	superseded           bool
	supersededBy         map[string]string
	replacements         []toolresult.CleanupReplacement
	reclaimed            int
	placeholder          int
}

func collectToolInteractionGroups(messages []*agent.Message, policy ContextPressurePolicy) ([]*toolInteractionGroup, int) {
	eagerMinimum := toolresult.EagerMinimumTokens(policy.EagerMinTokens, policy.ContextWindowTokens, policy.EagerMinContextRatio)
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
			if id == "" || calls[id].ID != "" || !validToolArgumentsJSON(call.Function.Arguments) {
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
				tokens := EstimateMessageTokens(result)
				group.resultTokens += tokens
				group.originalResultTokens += estimatedOriginalToolResultTokens(result)
				if ToolResultProtected(result) {
					group.protected = true
					protectedResults++
				}
				if result.ToolResult != nil && result.ToolResult.ContextHints != nil {
					group.discardable = group.discardable || result.ToolResult.ContextHints.ContextValue == agent.ToolResultContextDiscardable
				}
			}
			group.end++
		}
		// A later assistant step proves the model has already consumed this tool
		// batch. Protect only the still-unconsumed batch; a user-turn boundary is
		// not a semantic reason to pin every recoverable result body in that turn.
		group.awaitingModelStep = !hasLaterAssistantStep(messages, group.end)
		if len(seen) != len(calls) || len(calls) == 0 || group.awaitingModelStep {
			group.protected = true
		}
		group.largeRecoverable = !group.protected && group.originalResultTokens >= eagerMinimum
		group.eager = group.largeRecoverable && groupAllowsEager(messages, group)
		groups = append(groups, group)
		index = group.end - 1
	}
	return groups, protectedResults
}

func hasLaterAssistantStep(messages []*agent.Message, start int) bool {
	for index := start; index < len(messages); index++ {
		if message := messages[index]; message != nil && message.Role == agent.Assistant {
			return true
		}
	}
	return false
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
	return EstimateMessageTokens(message)
}

func validToolArgumentsJSON(arguments string) bool {
	arguments = strings.TrimSpace(arguments)
	if arguments == "" {
		return true
	}
	decoder := json.NewDecoder(strings.NewReader(arguments))
	decoder.UseNumber()
	var payload map[string]any
	if err := decoder.Decode(&payload); err != nil || payload == nil {
		return false
	}
	var trailing any
	return decoder.Decode(&trailing) == io.EOF
}

func ToolResultProtected(message *agent.Message) bool {
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
		if group.awaitingModelStep {
			continue
		}
		if group.eager || group.largeRecoverable {
			// Large recoverable results must remain available through one later
			// assistant step, but must not make the recent-group window impossible
			// to clean under pressure. Eager candidates additionally passed the
			// settled-run gate and may transition below ordinary pressure.
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
	return result.ResultRetention
}
