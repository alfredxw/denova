package toolresult

import (
	agent "github.com/alfredxw/denova/agent"
)

func validToolCall(call agent.ToolCall) bool {
	_, err := agent.NormalizeToolCallForModelContext(call, nil)
	return err == nil
}

func assistantHasIndependentContent(message *agent.Message) bool {
	if message == nil {
		return false
	}
	return message.Content != "" || message.Name != "" || message.ReasoningContent != "" ||
		len(message.MultiContent) > 0 || len(message.AssistantGenMultiContent) > 0
}
