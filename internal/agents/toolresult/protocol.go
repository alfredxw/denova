package toolresult

import (
	"encoding/json"
	"io"
	"strings"

	agent "github.com/alfredxw/denova/agent"
)

func validToolCall(call agent.ToolCall) bool {
	callID := strings.TrimSpace(call.ID)
	toolName := strings.TrimSpace(call.Function.Name)
	callType := strings.TrimSpace(call.Type)
	if callID == "" || call.ID != callID || toolName == "" || call.Function.Name != toolName {
		return false
	}
	if call.Type != callType || (callType != "" && callType != "function") {
		return false
	}
	if call.Index != nil && *call.Index < 0 {
		return false
	}
	arguments := strings.TrimSpace(call.Function.Arguments)
	if arguments == "" {
		return true
	}
	decoder := json.NewDecoder(strings.NewReader(arguments))
	decoder.UseNumber()
	var payload map[string]any
	if err := decoder.Decode(&payload); err != nil || payload == nil {
		return false
	}
	var trailing any
	return decoder.Decode(&trailing) == io.EOF
}

func assistantHasIndependentContent(message *agent.Message) bool {
	if message == nil {
		return false
	}
	return message.Content != "" || message.Name != "" || message.ReasoningContent != "" ||
		len(message.MultiContent) > 0 || len(message.AssistantGenMultiContent) > 0
}
