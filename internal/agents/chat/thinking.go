package chat

import (
	"strings"

	"denova/internal/agents/run"
)

// emitThinkingContent forwards provider thinking unchanged to both the live
// display stream and the display-only assistant trace. Thinking is excluded
// from later model context by the conversation boundary, not by rewriting it.
func emitThinkingContent(
	fullThinking *strings.Builder,
	content string,
	meta agentEventMetadata,
	eventType string,
	emit func(agentrun.Event),
) {
	if content == "" {
		return
	}
	if fullThinking != nil && !meta.SubAgent {
		fullThinking.WriteString(content)
	}
	emit(agentrun.Event{Type: eventType, Data: meta.appendTo(map[string]interface{}{"content": content})})
}
