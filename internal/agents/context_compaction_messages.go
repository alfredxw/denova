package agents

import (
	"fmt"
	"strings"

	agent "github.com/alfredxw/denova/agent"
)

func NewContextCompactionSummaryMessage(epoch int, summary string) *agent.Message {
	return agent.UserMessage(fmt.Sprintf("%s epoch=%d\n\n%s", contextCompactionSummaryPrefix, epoch, strings.TrimSpace(summary)))
}

func isContextCompactionMessage(msg *agent.Message) bool {
	return msg != nil && strings.HasPrefix(strings.TrimSpace(msg.Content), contextCompactionSummaryPrefix)
}

// IsContextCompactionSummaryMessage reports whether msg is a model-visible
// context-checkpoint record produced by Denova's compaction pipeline.
func IsContextCompactionSummaryMessage(msg *agent.Message) bool {
	return isContextCompactionMessage(msg)
}

func compactMessagesForModel(messages []*agent.Message, summary string, epoch, retainedTurns int) []*agent.Message {
	systemMessages := make([]*agent.Message, 0)
	contextMessages := make([]*agent.Message, 0, len(messages))
	for _, msg := range messages {
		if msg == nil || isContextCompactionMessage(msg) {
			continue
		}
		if msg.Role == agent.System {
			systemMessages = append(systemMessages, msg)
			continue
		}
		contextMessages = append(contextMessages, msg)
	}
	tail := retainTailByUserTurns(contextMessages, retainedTurns)
	result := make([]*agent.Message, 0, len(systemMessages)+1+len(tail))
	result = append(result, systemMessages...)
	result = append(result, NewContextCompactionSummaryMessage(epoch, summary))
	result = append(result, tail...)
	return result
}

func compactedMessagesAfterSource(messages []*agent.Message, effectiveStart, sourceEndIndex, retainedTurns int) []*agent.Message {
	sourceEndOffset := sourceEndIndex - effectiveStart
	if sourceEndOffset < 0 {
		sourceEndOffset = 0
	}
	if sourceEndOffset > len(messages) {
		sourceEndOffset = len(messages)
	}
	sourceTail := retainTailByUserTurns(compactionContextMessages(messages[:sourceEndOffset]), retainedTurns)
	appended := compactionContextMessages(messages[sourceEndOffset:])
	tail := make([]*agent.Message, 0, len(sourceTail)+len(appended))
	tail = append(tail, sourceTail...)
	tail = append(tail, appended...)
	return tail
}

func compactionContextMessages(messages []*agent.Message) []*agent.Message {
	filtered := make([]*agent.Message, 0, len(messages))
	for _, msg := range messages {
		if msg == nil || isContextCompactionMessage(msg) {
			continue
		}
		filtered = append(filtered, msg)
	}
	return filtered
}

// BuildCompactedModelMessages rebuilds model-visible history after a compaction
// record is persisted and its final epoch is known.
func BuildCompactedModelMessages(messages []*agent.Message, summary string, epoch, retainedTurns int) []*agent.Message {
	return compactMessagesForModel(messages, summary, epoch, retainedTurns)
}
