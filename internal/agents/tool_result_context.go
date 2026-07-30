package agents

import (
	"strings"

	agent "github.com/alfredxw/denova/agent"

	"denova/config"
)

// ToolResultContextPolicy controls whether bounded rich tool exchanges are
// retained in canonical model history. Pressure-driven cleanup is a separate,
// append-only projection; merely crossing a user-turn boundary never changes
// a rich result into a receipt.
type ToolResultContextPolicy struct {
	AgentKind      string
	Enabled        bool
	MaxResultBytes int
}

func resolveToolResultContextPolicy(cfg *config.Config, agentKind string) ToolResultContextPolicy {
	settings := config.ResolveAgentContext(cfg, agentKind)
	return ToolResultContextPolicy{
		AgentKind: strings.TrimSpace(agentKind), Enabled: settings.ToolResultContextEnabled,
		MaxResultBytes: configToolResultMaxBytes(cfg),
	}
}

func ResolveToolResultContextPolicyForConversation(cfg *config.Config, agentKind string) ToolResultContextPolicy {
	return resolveToolResultContextPolicy(cfg, agentKind)
}

func (p ToolResultContextPolicy) normalized() ToolResultContextPolicy {
	p.MaxResultBytes = normalizeToolResultLimitBytes(p.MaxResultBytes)
	return p
}

type toolResultContextConversation interface {
	AppendContextMessages(messages ...*agent.Message) error
	ToolResultContextPolicy() ToolResultContextPolicy
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
	policy := contextConversation.ToolResultContextPolicy().normalized()
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
	// The native run keeps provider reasoning/signatures until the tool loop has
	// finished. Cross-turn durability is a different boundary: only public
	// assistant content and the protocol calls belong in future model context.
	// Usage is recorded by dedicated telemetry, and opaque transport fields may
	// contain provider-private thinking, so do not duplicate either here.
	assistant.ReasoningContent = ""
	assistant.MultiContent = nil
	assistant.UserInputMultiContent = nil
	assistant.AssistantGenMultiContent = nil
	assistant.ResponseMeta = nil
	assistant.Extra = nil
	batch := &pendingToolContextBatch{
		assistant: assistant,
		callIndex: make(map[string]int, len(msg.ToolCalls)),
		results:   make([]*agent.Message, len(msg.ToolCalls)),
		remaining: len(msg.ToolCalls),
	}
	for index, call := range msg.ToolCalls {
		if !validContextToolCall(call) {
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
	result.ToolName = normalizeToolName(r.pending.assistant.ToolCalls[resultIndex].Function.Name)
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

func applyToolResultContextPolicy(messages []*agent.Message, policy ToolResultContextPolicy) []*agent.Message {
	if len(messages) == 0 {
		return messages
	}
	return filterToolContextMessages(completeUnknownToolResults(messages), policy.normalized())
}

func filterToolContextMessages(messages []*agent.Message, policy ToolResultContextPolicy) []*agent.Message {
	type callProjection struct {
		call        agent.ToolCall
		valid       bool
		resultIndex int
		results     int
	}

	filtered := make([]*agent.Message, 0, len(messages))
	for index := 0; index < len(messages); {
		message := messages[index]
		if message == nil {
			index++
			continue
		}
		if message.Role != agent.Assistant || len(message.ToolCalls) == 0 {
			if message.Role != agent.ToolRole {
				filtered = append(filtered, message)
			}
			index++
			continue
		}

		// Provider call IDs identify calls within one assistant response, not
		// across the transcript. Pair only with its contiguous result run so a
		// later reused ID cannot invalidate or steal this exchange.
		batchEnd := toolResultBatchEnd(messages, index)
		calls := make(map[string]callProjection, len(message.ToolCalls))
		for _, call := range message.ToolCalls {
			callID := strings.TrimSpace(call.ID)
			if callID == "" {
				continue
			}
			if existing, found := calls[callID]; found {
				existing.valid = false
				calls[callID] = existing
				continue
			}
			calls[callID] = callProjection{call: call, valid: validContextToolCall(call), resultIndex: -1}
		}
		for resultIndex := index + 1; resultIndex < batchEnd; resultIndex++ {
			result := messages[resultIndex]
			if result == nil {
				continue
			}
			callID := strings.TrimSpace(result.ToolCallID)
			projection, found := calls[callID]
			if !found {
				continue
			}
			projection.results++
			if projection.results == 1 {
				projection.resultIndex = resultIndex
			}
			calls[callID] = projection
		}

		nextAssistant := message.Clone()
		nextAssistant.ToolCalls = nil
		retainedResults := make(map[int]agent.ToolCall, len(message.ToolCalls))
		for _, call := range message.ToolCalls {
			callID := strings.TrimSpace(call.ID)
			projection, found := calls[callID]
			if !found || !projection.valid || projection.results != 1 {
				continue
			}
			result := messages[projection.resultIndex]
			if result == nil || result.ToolCallID != callID || strings.TrimSpace(result.Content) == "" ||
				(!policy.Enabled && !isUnknownToolEffectResult(result.Content)) {
				continue
			}
			nextAssistant.ToolCalls = append(nextAssistant.ToolCalls, call)
			retainedResults[projection.resultIndex] = projection.call
		}
		if len(nextAssistant.ToolCalls) > 0 || contextAssistantHasIndependentContent(nextAssistant) {
			filtered = append(filtered, nextAssistant)
		}
		for resultIndex := index + 1; resultIndex < batchEnd; resultIndex++ {
			call, retained := retainedResults[resultIndex]
			if !retained {
				continue
			}
			nextResult := messages[resultIndex].Clone()
			nextResult.ToolCalls = nil
			nextResult.ToolName = normalizeToolName(call.Function.Name)
			filtered = append(filtered, nextResult)
		}
		index = batchEnd
	}
	return filtered
}

func toolResultBatchEnd(messages []*agent.Message, assistantIndex int) int {
	end := assistantIndex + 1
	for end < len(messages) {
		if messages[end] == nil {
			end++
			continue
		}
		if messages[end].Role != agent.ToolRole {
			break
		}
		end++
	}
	return end
}

func ApplyToolResultContextPolicyForConversation(messages []*agent.Message, policy ToolResultContextPolicy) []*agent.Message {
	return applyToolResultContextPolicy(messages, policy)
}
