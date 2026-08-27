package context

import (
	"encoding/json"

	agent "github.com/alfredxw/denova/agent"
)

// EstimateTokens returns a conservative provider-neutral estimate for the
// complete model-visible message and tool surface.
func EstimateTokens(messages []*agent.Message, tools []*agent.ToolInfo) int {
	tokens := 0
	tokens += agent.EstimateMessagesTokens(messages)
	if len(tools) > 0 {
		data, err := json.Marshal(tools)
		if err == nil {
			tokens += EstimateStringTokens(string(data))
		} else {
			tokens += len(tools) * 128
		}
	}
	return max(1, tokens)
}

// EstimateMessageTokens estimates one provider-visible message, including
// structured content and tool-call envelopes.
func EstimateMessageTokens(message *agent.Message) int {
	return agent.EstimateMessageTokens(message)
}

// EstimateStringTokens keeps the established mixed ASCII/CJK estimate stable
// across compaction, pressure planning, and UI analysis.
func EstimateStringTokens(content string) int {
	return agent.EstimateTextTokens(content)
}
