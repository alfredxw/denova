package context

import (
	"encoding/json"
	"unicode"

	agent "github.com/alfredxw/denova/agent"
)

// EstimateTokens returns a conservative provider-neutral estimate for the
// complete model-visible message and tool surface.
func EstimateTokens(messages []*agent.Message, tools []*agent.ToolInfo) int {
	tokens := 0
	for _, message := range messages {
		tokens += EstimateMessageTokens(message)
	}
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
	if message == nil {
		return 0
	}
	tokens := 4 + EstimateStringTokens(string(message.Role)) + EstimateStringTokens(message.Content)
	tokens += EstimateStringTokens(message.ReasoningContent)
	for _, value := range []any{
		message.ToolCalls,
		message.MultiContent,
		message.UserInputMultiContent,
		message.AssistantGenMultiContent,
	} {
		data, err := json.Marshal(value)
		if err == nil && string(data) != "null" && string(data) != "[]" {
			tokens += EstimateStringTokens(string(data))
		}
	}
	tokens += EstimateStringTokens(message.ToolName)
	tokens += EstimateStringTokens(message.ToolCallID)
	return tokens
}

// EstimateStringTokens keeps the established mixed ASCII/CJK estimate stable
// across compaction, pressure planning, and UI analysis.
func EstimateStringTokens(content string) int {
	if content == "" {
		return 0
	}
	tokens := 0
	asciiRunes := 0
	flushASCII := func() {
		if asciiRunes == 0 {
			return
		}
		tokens += (asciiRunes + 3) / 4
		asciiRunes = 0
	}
	for _, r := range content {
		if r <= unicode.MaxASCII {
			asciiRunes++
			continue
		}
		flushASCII()
		tokens++
	}
	flushASCII()
	return max(1, tokens)
}
