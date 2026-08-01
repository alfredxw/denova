package chat

import (
	"denova/internal/agents/tool"
	"strings"

	agent "github.com/alfredxw/denova/agent"
	"github.com/alfredxw/denova/agent/providers"

	agentcontext "denova/internal/agents/context"
	"denova/internal/agents/toolresult"
)

type toolResultContextConversation interface {
	AppendContextMessages(messages ...*agent.Message) error
	ToolResultContextPolicy() toolresult.ContextPolicy
}

type pendingToolContextBatch struct {
	assistant *agent.Message
	callIndex map[string]int
	results   []*agent.Message
	remaining int
}

type toolResultContextRecorder struct {
	conversation toolResultContextConversation
	pending      *pendingToolContextBatch
}

func newToolResultContextRecorder(conversation Conversation) *toolResultContextRecorder {
	contextConversation, ok := conversation.(toolResultContextConversation)
	if !ok || contextConversation == nil {
		return &toolResultContextRecorder{}
	}
	policy := contextConversation.ToolResultContextPolicy().Normalize()
	if !policy.Enabled {
		return &toolResultContextRecorder{}
	}
	return &toolResultContextRecorder{conversation: contextConversation}
}

// RecordAssistantToolCalls stages one provider response in memory. Its calls
// and bounded rich results are committed as one interaction group only after
// every execution reaches a terminal state.
func (r *toolResultContextRecorder) RecordAssistantToolCalls(msg *agent.Message, meta agentEventMetadata) {
	if r == nil || r.conversation == nil || meta.SubAgent || msg == nil || len(msg.ToolCalls) == 0 {
		return
	}
	// The Agent runtime completes one assistant tool batch before it can start
	// another. Seeing an overlapping batch is therefore ambiguous; discard both
	// rather than let a provider-local call ID pair across responses.
	if r.pending != nil {
		r.pending = nil
		return
	}
	if (msg.Role != "" && msg.Role != agent.Assistant) || msg.ToolCallID != "" || msg.ToolName != "" || msg.ToolResult != nil {
		return
	}

	assistant := msg.Clone()
	assistant.Role = agent.Assistant
	// UI reasoning fields and response telemetry do not cross this boundary.
	// Protocol continuation is the sole exception: Responses store:false needs
	// provider-returned output items to recreate the exact stateless tool
	// sequence. The envelope remains model-only and is never projected as UI
	// reasoning.
	assistant.ReasoningContent = ""
	assistant.MultiContent = nil
	assistant.UserInputMultiContent = nil
	assistant.AssistantGenMultiContent = nil
	assistant.ResponseMeta = nil
	assistant.Extra = providers.ContinuationExtra(assistant.Extra)
	batch := &pendingToolContextBatch{
		assistant: assistant,
		callIndex: make(map[string]int, len(msg.ToolCalls)),
		results:   make([]*agent.Message, len(msg.ToolCalls)),
		remaining: len(msg.ToolCalls),
	}
	for index, call := range msg.ToolCalls {
		if !agentcontext.ValidToolCall(call) {
			return
		}
		if _, duplicate := batch.callIndex[call.ID]; duplicate {
			return
		}
		batch.callIndex[call.ID] = index
	}
	r.pending = batch
}

func (r *toolResultContextRecorder) RecordToolResult(message *agent.Message, meta agentEventMetadata) error {
	if r == nil || r.conversation == nil || meta.SubAgent || message == nil || r.pending == nil {
		return nil
	}
	callID := strings.TrimSpace(message.ToolCallID)
	resultIndex, ok := r.pending.callIndex[callID]
	if !ok {
		return nil
	}
	if message.Role != agent.ToolRole || message.ToolCallID != callID || strings.TrimSpace(message.Content) == "" || len(message.ToolCalls) > 0 {
		r.pending = nil
		return nil
	}
	if r.pending.results[resultIndex] != nil {
		// A duplicate result makes the batch ambiguous until it is published.
		// Once a complete batch is published, later duplicates see no pending
		// state and are harmlessly ignored.
		r.pending = nil
		return nil
	}

	result := message.Clone()
	result.ToolName = agenttool.NormalizeName(r.pending.assistant.ToolCalls[resultIndex].Function.Name)
	r.pending.results[resultIndex] = result
	r.pending.remaining--
	if r.pending.remaining > 0 {
		return nil
	}

	batch := r.pending
	r.pending = nil
	messages := make([]*agent.Message, 1, 1+len(batch.results))
	messages[0] = batch.assistant
	messages = append(messages, batch.results...)
	return r.conversation.AppendContextMessages(messages...)
}
