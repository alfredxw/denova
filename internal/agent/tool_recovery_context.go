package agent

import (
	"strings"

	adk "github.com/alfredxw/denova/adk"

	runstate "denova/internal/agent/runtime"
)

// completeUnknownToolResults repairs only a missing result half. A durable
// tool-start without a completion can mean the external effect happened, so
// the next provider input receives a complete call/result exchange that says
// exactly that and forbids automatic retry. Existing results always win, and
// running this projection repeatedly is idempotent.
func completeUnknownToolResults(messages []*adk.Message) []*adk.Message {
	if len(messages) == 0 {
		return messages
	}
	callCounts := make(map[string]int)
	resultCounts := make(map[string]int)
	for _, message := range messages {
		if message == nil {
			continue
		}
		if message.Role == adk.Assistant {
			for _, call := range message.ToolCalls {
				if callID := strings.TrimSpace(call.ID); callID != "" {
					callCounts[callID]++
				}
			}
		}
		if message.Role == adk.Tool {
			if callID := strings.TrimSpace(message.ToolCallID); callID != "" {
				resultCounts[callID]++
			}
		}
	}

	completed := make([]*adk.Message, 0, len(messages))
	for _, message := range messages {
		if message == nil {
			continue
		}
		completed = append(completed, message)
		if message.Role != adk.Assistant {
			continue
		}
		for _, call := range message.ToolCalls {
			callID := strings.TrimSpace(call.ID)
			toolName := strings.TrimSpace(call.Function.Name)
			if callID == "" || toolName == "" || callCounts[callID] != 1 || resultCounts[callID] != 0 {
				continue
			}
			if _, valid := retainedToolCallArguments(call.Function.Arguments); !valid {
				continue
			}
			completed = append(completed, adk.ToolMessage(
				runstate.UnknownToolEffectResult,
				callID,
				adk.WithToolName(toolName),
			))
		}
	}
	return completed
}

func isUnknownToolEffectResult(content string) bool {
	return strings.TrimSpace(content) == runstate.UnknownToolEffectResult
}
