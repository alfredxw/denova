package toolresult

import (
	"fmt"
	"strings"

	agent "github.com/alfredxw/denova/agent"
)

// ResolveCleanupTargets translates provider-visible replacement
// indexes back to one canonical model-history slice. Tool call IDs are only
// batch-local for some providers, so matching uses the occurrence ordinal from
// the end of the transcript instead of assuming global ID uniqueness.
func ResolveCleanupTargets(
	visible []*agent.Message,
	canonical []*agent.Message,
	plan CleanupPlan,
) ([]CleanupReplacement, error) {
	resolved := make([]CleanupReplacement, 0, len(plan.Replacements))
	used := make(map[int]struct{}, len(plan.Replacements))
	for _, replacement := range plan.Replacements {
		if replacement.MessageIndex < 0 || replacement.MessageIndex >= len(visible) {
			return nil, fmt.Errorf("cleanup replacement index %d is outside provider snapshot", replacement.MessageIndex)
		}
		target := visible[replacement.MessageIndex]
		if target == nil || target.Role != agent.ToolRole || strings.TrimSpace(target.ToolCallID) != strings.TrimSpace(replacement.ToolCallID) {
			return nil, fmt.Errorf("cleanup replacement %q does not match provider snapshot", replacement.ToolCallID)
		}
		ordinal := toolResultOccurrenceFromEnd(visible, replacement.MessageIndex, target.ToolCallID, target.ToolName)
		canonicalIndex := toolResultIndexFromEnd(canonical, ordinal, target.ToolCallID, target.ToolName)
		if canonicalIndex < 0 {
			return nil, fmt.Errorf("cleanup tool call %q occurrence %d does not resolve to canonical context", target.ToolCallID, ordinal)
		}
		if _, duplicate := used[canonicalIndex]; duplicate {
			return nil, fmt.Errorf("cleanup replacements resolve to duplicate canonical index %d", canonicalIndex)
		}
		used[canonicalIndex] = struct{}{}
		next := replacement
		next.MessageIndex = canonicalIndex
		resolved = append(resolved, next)
	}
	return resolved, nil
}

func toolResultOccurrenceFromEnd(messages []*agent.Message, targetIndex int, callID, toolName string) int {
	ordinal := 0
	for index := len(messages) - 1; index >= targetIndex; index-- {
		if sameCleanupToolResultIdentity(messages[index], callID, toolName) {
			ordinal++
		}
	}
	return ordinal
}

func toolResultIndexFromEnd(messages []*agent.Message, ordinal int, callID, toolName string) int {
	for index := len(messages) - 1; index >= 0; index-- {
		if !sameCleanupToolResultIdentity(messages[index], callID, toolName) {
			continue
		}
		ordinal--
		if ordinal == 0 {
			return index
		}
	}
	return -1
}

func sameCleanupToolResultIdentity(message *agent.Message, callID, toolName string) bool {
	if message == nil || message.Role != agent.ToolRole || strings.TrimSpace(message.ToolCallID) != strings.TrimSpace(callID) {
		return false
	}
	toolName = normalizeToolName(toolName)
	return toolName == "" || normalizeToolName(message.ToolName) == toolName
}
