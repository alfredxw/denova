package toolresult

import (
	"strings"

	agent "github.com/alfredxw/denova/agent"

	"denova/config"
)

// ContextPolicy controls whether bounded rich tool exchanges are
// retained in canonical model history. Pressure-driven cleanup is a separate,
// append-only projection; merely crossing a user-turn boundary never changes
// a rich result into a receipt.
type ContextPolicy struct {
	AgentKind      string
	Enabled        bool
	MaxResultBytes int
}

func ResolveContextPolicy(cfg *config.Config, agentKind string) ContextPolicy {
	settings := config.ResolveAgentContext(cfg, agentKind)
	return ContextPolicy{
		AgentKind: strings.TrimSpace(agentKind), Enabled: settings.ToolResultContextEnabled,
		MaxResultBytes: LimitBytes(cfg),
	}
}

func (p ContextPolicy) Normalize() ContextPolicy {
	p.MaxResultBytes = NormalizeLimitBytes(p.MaxResultBytes)
	return p
}

func ApplyContextPolicy(messages []*agent.Message, policy ContextPolicy) []*agent.Message {
	if len(messages) == 0 {
		return messages
	}
	return filterToolContextMessages(CompleteUnknownToolResults(messages), policy.Normalize())
}

func filterToolContextMessages(messages []*agent.Message, policy ContextPolicy) []*agent.Message {
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
			calls[callID] = callProjection{call: call, valid: validToolCall(call), resultIndex: -1}
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
				(!policy.Enabled && !IsUnknownEffectResult(result.Content)) {
				continue
			}
			nextAssistant.ToolCalls = append(nextAssistant.ToolCalls, call)
			retainedResults[projection.resultIndex] = projection.call
		}
		if len(nextAssistant.ToolCalls) > 0 || assistantHasIndependentContent(nextAssistant) {
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
