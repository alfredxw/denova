package compaction

import (
	agent "github.com/alfredxw/denova/agent"
	basecontext "github.com/alfredxw/denova/agent/context"
	"strings"
)

func PreserveLeadingMessage(messages []*agent.Message, content string) []*agent.Message {
	content = strings.TrimSpace(content)
	if content == "" {
		return messages
	}
	for _, message := range messages {
		if message != nil && strings.TrimSpace(message.Content) == content {
			return messages
		}
	}
	boundary := 0
	for boundary < len(messages) {
		message := messages[boundary]
		if message == nil {
			break
		}
		role := strings.TrimSpace(string(message.Role))
		if role != string(agent.System) && role != "developer" {
			break
		}
		boundary++
	}
	leading := agent.UserMessage(content)
	leading.Extra = map[string]any{basecontext.MessageExtraPlacement: string(basecontext.PlacementLeadingMessage)}
	result := make([]*agent.Message, 0, len(messages)+1)
	result = append(result, messages[:boundary]...)
	result = append(result, leading)
	result = append(result, messages[boundary:]...)
	return result
}
