package agents

import (
	"encoding/json"
	"unicode"

	agent "github.com/alfredxw/denova/agent"

	"denova/config"
)

// EstimateContextProjectionReserves returns bounded reserves for completion and
// retained tool results. expectedOutputChars should be the user-configured
// target when one exists; otherwise a small model-relative reserve is used.
func EstimateContextProjectionReserves(cfg *config.Config, agentKind string, expectedOutputChars int) (completionTokens, toolResultTokens int) {
	model := config.ResolveAgentModel(cfg, agentKind)
	window := model.ContextWindowTokens
	completionTokens = expectedOutputChars
	if completionTokens <= 0 {
		completionTokens = max(2048, window/50)
	} else {
		// Leave room for the hidden structured result and normal completion
		// variance around the visible user-configured target.
		completionTokens += max(1024, expectedOutputChars/4)
	}
	if window > 0 {
		completionTokens = min(completionTokens, max(2048, window/4))
	}
	toolPolicy := resolveToolResultContextPolicy(cfg, agentKind).normalized()
	if toolPolicy.Enabled {
		// A result is bounded at the tool boundary before it is persisted. Reserve
		// for one such result; older exchanges are owned by normal compaction.
		toolResultTokens = toolPolicy.MaxResultBytes / 3
		if window > 0 {
			toolResultTokens = min(toolResultTokens, max(1024, window/10))
		}
	}
	return completionTokens, toolResultTokens
}

func withDefaultContextProjectionReserves(cfg *config.Config, agentKind string, input ContextCompactionInput, expectedOutputChars int) ContextCompactionInput {
	completion, tools := EstimateContextProjectionReserves(cfg, agentKind, expectedOutputChars)
	if input.ReservedCompletionTokens <= 0 {
		input.ReservedCompletionTokens = completion
	}
	if input.ReservedToolResultTokens <= 0 {
		input.ReservedToolResultTokens = tools
	}
	return input
}

func projectedContextTokens(promptTokens int, input ContextCompactionInput) int {
	return max(1, promptTokens+max(0, input.ReservedCompletionTokens)+max(0, input.ReservedToolResultTokens))
}

func compactionSourceBaseMessages(input ContextCompactionInput) []*agent.Message {
	if len(input.SourceMessages) > 0 {
		return input.SourceMessages
	}
	return input.Messages
}

func EstimateContextTokens(messages []*agent.Message, tools []*agent.ToolInfo) int {
	tokens := 0
	for _, msg := range messages {
		tokens += estimateMessageTokens(msg)
	}
	if len(tools) > 0 {
		data, err := json.Marshal(tools)
		if err == nil {
			tokens += estimateStringTokens(string(data))
		} else {
			tokens += len(tools) * 128
		}
	}
	if tokens < 1 {
		return 1
	}
	return tokens
}

func estimateMessageTokens(msg *agent.Message) int {
	if msg == nil {
		return 0
	}
	tokens := 4 + estimateStringTokens(string(msg.Role)) + estimateStringTokens(msg.Content)
	tokens += estimateStringTokens(msg.ReasoningContent)
	if len(msg.ToolCalls) > 0 {
		if data, err := json.Marshal(msg.ToolCalls); err == nil {
			tokens += estimateStringTokens(string(data))
		}
	}
	if len(msg.MultiContent) > 0 {
		if data, err := json.Marshal(msg.MultiContent); err == nil {
			tokens += estimateStringTokens(string(data))
		}
	}
	if len(msg.UserInputMultiContent) > 0 {
		if data, err := json.Marshal(msg.UserInputMultiContent); err == nil {
			tokens += estimateStringTokens(string(data))
		}
	}
	if len(msg.AssistantGenMultiContent) > 0 {
		if data, err := json.Marshal(msg.AssistantGenMultiContent); err == nil {
			tokens += estimateStringTokens(string(data))
		}
	}
	if msg.ToolName != "" {
		tokens += estimateStringTokens(msg.ToolName)
	}
	if msg.ToolCallID != "" {
		tokens += estimateStringTokens(msg.ToolCallID)
	}
	return tokens
}

func estimateStringTokens(content string) int {
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
	if tokens < 1 {
		return 1
	}
	return tokens
}
