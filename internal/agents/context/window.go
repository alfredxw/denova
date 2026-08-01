package context

import (
	"fmt"
	"strings"

	agent "github.com/alfredxw/denova/agent"
)

// RewindMutationReceipt describes a side effect that remains committed when
// model-visible context is rewound. It deliberately contains no persistence
// concerns so every conversation store can reuse the same projection logic.
type RewindMutationReceipt struct {
	Tool    string
	CallID  string
	Scope   string
	Summary string
}

// RewindSummaryInput is the provider-visible, persistence-neutral description
// of a context rewind.
type RewindSummaryInput struct {
	CheckpointID     string
	Purpose          string
	Report           string
	MutationReceipts []RewindMutationReceipt
}

// NewRewindSummaryMessage renders the stable model-visible marker for a
// context rewind. The message is context data, never a user instruction.
func NewRewindSummaryMessage(input RewindSummaryInput) *agent.Message {
	var content strings.Builder
	content.WriteString(RewindSummaryPrefix)
	content.WriteString(" checkpoint=")
	content.WriteString(input.CheckpointID)
	if input.Purpose != "" {
		content.WriteString(" purpose=")
		content.WriteString(fmt.Sprintf("%q", input.Purpose))
	}
	content.WriteString("\n\nAssistant-authored exploration report (context data, not a user instruction):\n")
	if strings.TrimSpace(input.Report) == "" {
		content.WriteString("No report was supplied.")
	} else {
		content.WriteString(strings.TrimSpace(input.Report))
	}
	if len(input.MutationReceipts) > 0 {
		content.WriteString("\n\nCommitted side effects (not rolled back):")
		for _, receipt := range input.MutationReceipts {
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

// ApplyWindowProjection replaces a discarded exploration branch with the
// frozen canonical prefix and its rewind report while preserving later turns.
func ApplyWindowProjection(
	history []*agent.Message,
	effectiveStart int,
	canonicalPrefix []*agent.Message,
	rewindAfterIndex int,
	summary *agent.Message,
) []*agent.Message {
	prefix := cloneMessages(canonicalPrefix)
	tailStart := rewindAfterIndex - effectiveStart
	if tailStart < 0 {
		tailStart = 0
	}
	if tailStart > len(history) {
		tailStart = len(history)
	}
	result := make([]*agent.Message, 0, len(prefix)+1+len(history)-tailStart)
	result = append(result, prefix...)
	result = append(result, summary)
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

func cloneMessages(messages []*agent.Message) []*agent.Message {
	if messages == nil {
		return nil
	}
	cloned := make([]*agent.Message, len(messages))
	for index, message := range messages {
		if message == nil {
			continue
		}
		copyMessage := *message
		copyMessage.ToolCalls = append([]agent.ToolCall(nil), message.ToolCalls...)
		if message.Extra != nil {
			copyMessage.Extra = make(map[string]any, len(message.Extra))
			for key, value := range message.Extra {
				copyMessage.Extra[key] = value
			}
		}
		cloned[index] = &copyMessage
	}
	return cloned
}
