package cleanup

import (
	"encoding/json"
	"unicode"

	agent "github.com/alfredxw/denova/agent"
)

// EstimateTokens is a conservative provider-neutral estimate over messages
// and the exact tool schema captured by an executable compaction snapshot.
func EstimateTokens(messages []*agent.Message, snapshot *agent.ModelRequestSnapshot) int {
	tokens := EstimateMessages(messages)
	if snapshot != nil {
		if encoded, err := json.Marshal(snapshot.ResolvedOptions().Tools); err == nil {
			tokens += EstimateStringTokens(string(encoded))
		}
	}
	return max(1, tokens)
}

// EstimateInspectedTokens is the pure Cleanup counterpart to EstimateTokens.
// The detached inspection carries the same provider-neutral schemas without
// exposing a callable model snapshot to CleanupManager implementations.
func EstimateInspectedTokens(messages []*agent.Message, inspection agent.ModelRequestInspection) int {
	tokens := EstimateMessages(messages)
	if encoded, err := json.Marshal(inspection.Options.Tools); err == nil {
		tokens += EstimateStringTokens(string(encoded))
	}
	return max(1, tokens)
}

func EstimateMessages(messages []*agent.Message) int {
	tokens := 0
	for _, message := range messages {
		if message == nil {
			continue
		}
		tokens += 4 + EstimateStringTokens(string(message.Role)) + EstimateStringTokens(message.Content) + EstimateStringTokens(message.ReasoningContent)
		for _, value := range []any{message.ToolCalls, message.MultiContent, message.UserInputMultiContent, message.AssistantGenMultiContent} {
			if encoded, err := json.Marshal(value); err == nil && string(encoded) != "null" && string(encoded) != "[]" {
				tokens += EstimateStringTokens(string(encoded))
			}
		}
		tokens += EstimateStringTokens(message.ToolName) + EstimateStringTokens(message.ToolCallID)
	}
	return tokens
}

func EstimateStringTokens(content string) int {
	if content == "" {
		return 0
	}
	tokens, ascii := 0, 0
	flush := func() {
		if ascii > 0 {
			tokens += (ascii + 3) / 4
			ascii = 0
		}
	}
	for _, value := range content {
		if value <= unicode.MaxASCII {
			ascii++
		} else {
			flush()
			tokens++
		}
	}
	flush()
	return max(1, tokens)
}
