package agent

import (
	"encoding/json"
	"unicode"
)

// EstimateMessagesTextTokens estimates the textual part of messages. Use
// InputEstimator for a complete request, including native image inputs.
func EstimateMessagesTextTokens(messages []*Message) int {
	tokens := 0
	for _, message := range messages {
		tokens += EstimateMessageTextTokens(message)
	}
	return tokens
}

// EstimateMessageTextTokens includes structured content, tool-call envelopes,
// and attachment instructions, but excludes native image pixels.
func EstimateMessageTextTokens(message *Message) int {
	if message == nil {
		return 0
	}
	content := message.Content
	if message.Role == User {
		content = ModelUserContent(message)
	}
	tokens := 4 + EstimateTextTokens(string(message.Role)) + EstimateTextTokens(content)
	tokens += EstimateTextTokens(message.ReasoningContent)
	for _, value := range []any{
		message.ToolCalls,
		message.MultiContent,
		message.UserInputMultiContent,
		message.AssistantGenMultiContent,
	} {
		encoded, err := json.Marshal(value)
		if err == nil && string(encoded) != "null" && string(encoded) != "[]" {
			tokens += EstimateTextTokens(string(encoded))
		}
	}
	tokens += EstimateTextTokens(message.ToolName) + EstimateTextTokens(message.ToolCallID)
	return tokens
}

// EstimateTextTokens keeps the established mixed ASCII/CJK estimate stable.
func EstimateTextTokens(content string) int {
	if content == "" {
		return 0
	}
	tokens, asciiRunes := 0, 0
	flushASCII := func() {
		if asciiRunes > 0 {
			tokens += (asciiRunes + 3) / 4
			asciiRunes = 0
		}
	}
	for _, value := range content {
		if value <= unicode.MaxASCII {
			asciiRunes++
		} else {
			flushASCII()
			tokens++
		}
	}
	flushASCII()
	return max(1, tokens)
}

// EstimateRequestTextTokens includes message text and tool schemas. Complete
// model requests must use InputEstimator to include native images as well.
func EstimateRequestTextTokens(messages []*Message, tools []*ToolInfo) int {
	tokens := EstimateMessagesTextTokens(messages)
	if encoded, err := json.Marshal(tools); err == nil && string(encoded) != "null" {
		tokens += EstimateTextTokens(string(encoded))
	}
	return max(1, tokens)
}
