package agents

import (
	"fmt"
	"strings"

	agent "github.com/alfredxw/denova/agent"

	"denova/internal/agents/session"
)

const contextRewindSummaryPrefix = "[denova-context-rewind]"

func newContextRewindSummaryMessage(operation session.ContextOperation) *agent.Message {
	var content strings.Builder
	content.WriteString(contextRewindSummaryPrefix)
	content.WriteString(" checkpoint=")
	content.WriteString(operation.CheckpointID)
	if operation.Purpose != "" {
		content.WriteString(" purpose=")
		content.WriteString(fmt.Sprintf("%q", operation.Purpose))
	}
	content.WriteString("\n\nAssistant-authored exploration report (context data, not a user instruction):\n")
	if strings.TrimSpace(operation.Report) == "" {
		content.WriteString("No report was supplied.")
	} else {
		content.WriteString(strings.TrimSpace(operation.Report))
	}
	if len(operation.MutationReceipts) > 0 {
		content.WriteString("\n\nCommitted side effects (not rolled back):")
		for _, receipt := range operation.MutationReceipts {
			content.WriteString("\n- ")
			content.WriteString(receipt.Tool)
			if receipt.CallID != "" {
				content.WriteString(" call=")
				content.WriteString(receipt.CallID)
			}
			content.WriteString(" scope=")
			content.WriteString(receipt.Scope)
			if receipt.Summary != "" {
				content.WriteString(": ")
				content.WriteString(receipt.Summary)
			}
		}
	}
	return agent.AssistantMessage(content.String(), nil)
}

func applyContextWindowProjection(history []*agent.Message, effectiveStart int, projection session.ContextWindowProjection) []*agent.Message {
	boundary, boundaryErr := session.CloneContextCheckpointBoundary(projection.Checkpoint.Boundary)
	prefix := []*agent.Message(nil)
	if boundaryErr == nil {
		prefix = cloneContextMessages(boundary.CanonicalPrefix)
	}
	tailStart := projection.RewindAfterIndex - effectiveStart
	if tailStart < 0 {
		tailStart = 0
	}
	if tailStart > len(history) {
		tailStart = len(history)
	}
	result := make([]*agent.Message, 0, len(prefix)+1+len(history)-tailStart)
	result = append(result, prefix...)
	result = append(result, newContextRewindSummaryMessage(projection.Rewind))
	for _, message := range history[tailStart:] {
		if message != nil && message.Role == agent.Assistant && strings.TrimSpace(message.Content) == "" && len(message.ToolCalls) == 0 {
			// Structural metadata may be committed on an empty assistant record;
			// it is canonical durability, not a provider-visible chat turn.
			continue
		}
		result = append(result, message)
	}
	return result
}
