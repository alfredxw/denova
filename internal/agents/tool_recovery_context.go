package agents

import (
	"strings"

	agent "github.com/alfredxw/denova/agent"

	runstate "github.com/alfredxw/denova/agent/runtime"
)

// completeUnknownToolResults repairs only a missing result half. A durable
// tool-start without a completion can mean the external effect happened, so
// the next provider input receives a complete call/result exchange that says
// exactly that and forbids automatic retry. Existing results in the same
// assistant batch always win, and running this projection repeatedly is
// idempotent.
func completeUnknownToolResults(messages []*agent.Message) []*agent.Message {
	if len(messages) == 0 {
		return messages
	}
	completed := make([]*agent.Message, 0, len(messages))
	for index := 0; index < len(messages); {
		message := messages[index]
		if message == nil {
			index++
			continue
		}
		completed = append(completed, message)
		if message.Role != agent.Assistant || len(message.ToolCalls) == 0 {
			index++
			continue
		}

		batchEnd := toolResultBatchEnd(messages, index)
		callCounts := make(map[string]int, len(message.ToolCalls))
		resultCounts := make(map[string]int, batchEnd-index-1)
		for _, call := range message.ToolCalls {
			if callID := strings.TrimSpace(call.ID); callID != "" {
				callCounts[callID]++
			}
		}
		for resultIndex := index + 1; resultIndex < batchEnd; resultIndex++ {
			result := messages[resultIndex]
			if result == nil {
				continue
			}
			if callID := strings.TrimSpace(result.ToolCallID); callID != "" {
				resultCounts[callID]++
			}
		}
		for _, call := range message.ToolCalls {
			callID := strings.TrimSpace(call.ID)
			if !validContextToolCall(call) || callCounts[callID] != 1 || resultCounts[callID] != 0 {
				continue
			}
			completed = append(completed, agent.ToolMessage(
				agent.SyntheticToolResult(agent.ToolResultError, agent.ToolSyntheticEffectUnknown, runstate.UnknownToolEffectResult),
				callID,
				agent.WithToolName(call.Function.Name),
			))
		}
		for resultIndex := index + 1; resultIndex < batchEnd; resultIndex++ {
			if messages[resultIndex] != nil {
				completed = append(completed, messages[resultIndex])
			}
		}
		index = batchEnd
	}
	return completed
}

func isUnknownToolEffectResult(content string) bool {
	return strings.TrimSpace(content) == runstate.UnknownToolEffectResult
}
