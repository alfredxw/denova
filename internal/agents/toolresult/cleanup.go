package toolresult

import agent "github.com/alfredxw/denova/agent"

type CleanupReplacement struct {
	MessageIndex      int
	ToolCallID        string
	Placeholder       string
	OriginalTokens    int
	PlaceholderTokens int
}

type CleanupPlan struct {
	Replacements         []CleanupReplacement
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

// ApplyCleanupPlan applies the transient model projection represented by a
// cleanup plan. Domain stores persist the same rendered placeholders in their
// append-only cleanup records.
func ApplyCleanupPlan(messages []*agent.Message, plan CleanupPlan) []*agent.Message {
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
