package agent

import (
	"encoding/json"
	"unicode"
)

// EstimateMessagesTokens applies one provider-neutral estimate to every
// context-budgeting path. It includes model-visible attachment instructions
// and inline image payloads instead of treating attachments as free.
func EstimateMessagesTokens(messages []*Message) int {
	tokens := 0
	for _, message := range messages {
		tokens += EstimateMessageTokens(message)
	}
	return tokens
}

// EstimateMessageTokens estimates one provider-visible message, including
// structured content, tool-call envelopes, and native attachment payloads.
func EstimateMessageTokens(message *Message) int {
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
	for _, attachment := range message.Attachments {
		if IsNativeImageMediaType(attachment.MediaType) {
			tokens += EstimateTextTokens("data:"+attachment.MediaType+";base64,") + estimateBase64PayloadTokens(attachment.Size)
		}
	}
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

func estimateBase64PayloadTokens(byteSize int64) int {
	if byteSize <= 0 {
		return 1
	}
	// Base64 uses four ASCII characters per three input bytes; the shared
	// ASCII estimate charges roughly one token per four encoded characters.
	estimate := (byteSize + 2) / 3
	maxInt := int64(^uint(0) >> 1)
	if estimate > maxInt {
		return int(maxInt)
	}
	return int(estimate)
}
