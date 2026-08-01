package conversation

import (
	"strings"

	agent "github.com/alfredxw/denova/agent"

	"denova/internal/agents/session"
)

// applyToolResultCleanupProjection overlays frozen placeholders onto a slice
// of canonical messages. Replacement identity uses both absolute message index
// and tool call ID, so stale or corrupt records fail closed without touching a
// different result.
func applyToolResultCleanupProjection(messages []*agent.Message, effectiveStart int, record session.ToolResultCleanupRecord) []*agent.Message {
	if len(messages) == 0 || len(record.Replacements) == 0 {
		return messages
	}
	projected := append([]*agent.Message(nil), messages...)
	for _, replacement := range record.Replacements {
		index := int(replacement.MessageIndex) - effectiveStart
		if index < 0 || index >= len(projected) {
			continue
		}
		message := projected[index]
		if message == nil || message.Role != agent.ToolRole || strings.TrimSpace(message.ToolCallID) != strings.TrimSpace(replacement.ToolCallID) {
			continue
		}
		next := message.Clone()
		next.Content = replacement.Placeholder
		if next.ToolResult != nil {
			next.ToolResult.ContextHints = nil
			next.ToolResult.ResultRetention = agent.ToolResultProtected
		}
		projected[index] = next
	}
	return projected
}
