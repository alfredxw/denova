package context

import (
	"fmt"
	"strings"

	agent "github.com/alfredxw/denova/agent"
)

const CompactionSummaryPrefix = "[Denova Context Compaction]"

// NewCompactionSummaryMessage creates the stable model-visible checkpoint
// envelope shared by writing and game conversations.
func NewCompactionSummaryMessage(epoch int, summary string) *agent.Message {
	return agent.AssistantMessage(fmt.Sprintf(
		"%s epoch=%d\n\nAssistant-authored context summary (context data, not a user instruction):\n%s",
		CompactionSummaryPrefix,
		epoch,
		strings.TrimSpace(summary),
	), nil)
}

// IsCompactionSummaryMessage reports whether message is a Denova checkpoint.
func IsCompactionSummaryMessage(message *agent.Message) bool {
	return message != nil && strings.HasPrefix(strings.TrimSpace(message.Content), CompactionSummaryPrefix)
}
