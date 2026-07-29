package agents

import (
	"strings"

	agent "github.com/alfredxw/denova/agent"

	runstate "github.com/alfredxw/denova/agent/runtime"
)

// completeUnknownToolResults repairs only a missing result half. A durable
// tool-start without a completion can mean the external effect happened, so
// the next provider input receives a complete call/result exchange that says
// exactly that and forbids automatic retry. Existing results always win, and
// running this projection repeatedly is idempotent.
func completeUnknownToolResults(messages []*agent.Message) []*agent.Message {
	if len(messages) == 0 {
		return messages
	}
	callCounts := make(map[string]int)
	resultCounts := make(map[string]int)
	for _, message := range messages {
		if message == nil {
			continue
		}
		if message.Role == agent.Assistant {
			for _, call := range message.ToolCalls {
				if callID := strings.TrimSpace(call.ID); callID != "" {
					callCounts[callID]++
				}
			}
		}
		if message.Role == agent.ToolRole {
			if callID := strings.TrimSpace(message.ToolCallID); callID != "" {
				resultCounts[callID]++
			}
		}
	}

	completed := make([]*agent.Message, 0, len(messages))
	for _, message := range messages {
		if message == nil {
			continue
		}
		completed = append(completed, message)
		if message.Role != agent.Assistant {
			continue
		}
		for _, call := range message.ToolCalls {
			callID := strings.TrimSpace(call.ID)
			toolName := strings.TrimSpace(call.Function.Name)
			if callID == "" || toolName == "" || callCounts[callID] != 1 || resultCounts[callID] != 0 {
				continue
			}
			if validateToolArgumentsJSON(call.Function.Arguments) != nil {
				continue
			}
			completed = append(completed, agent.ToolMessage(
				agent.SyntheticToolResult(agent.ToolResultError, agent.ToolSyntheticEffectUnknown, runstate.UnknownToolEffectResult),
				callID,
				agent.WithToolName(toolName),
			))
		}
	}
	return completed
}

func isUnknownToolEffectResult(content string) bool {
	return strings.TrimSpace(content) == runstate.UnknownToolEffectResult
}
