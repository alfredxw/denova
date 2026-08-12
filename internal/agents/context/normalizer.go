package context

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	agent "github.com/alfredxw/denova/agent"
)

// ErrInvalidModelContextProtocol means the final provider-neutral transcript
// still contains a tool call/result ordering or identity violation.
var ErrInvalidModelContextProtocol = errors.New("模型上下文违反工具调用/结果协议 / model context violates the tool call/result protocol")

type contextToolCallOccurrence struct {
	messageIndex int
	callIndex    int
	valid        bool
}

type contextToolBatchKey struct {
	ownerIndex int
	callID     string
}

type contextToolResultOccurrence struct {
	messageIndex int
	ownerIndex   int
	canonicalID  bool
}

// NormalizeModelContextMessages repairs the provider-neutral tool protocol
// without applying retention, cleanup, or summarization policy. Valid rich
// results are cloned byte-for-byte. Ambiguous pairs are removed as one unit;
// only a unique valid call with no result receives the existing deterministic
// effect_unknown completion.
func NormalizeModelContextMessages(messages []*agent.Message) ([]*agent.Message, error) {
	if len(messages) == 0 {
		return messages, nil
	}

	callOccurrences := make(map[contextToolBatchKey][]contextToolCallOccurrence)
	resultOccurrences := make(map[contextToolBatchKey][]contextToolResultOccurrence)
	resultOwners := contextToolResultOwners(messages)
	for messageIndex, message := range messages {
		if message == nil {
			continue
		}
		if message.Role == agent.Assistant {
			for callIndex, call := range message.ToolCalls {
				key := strings.TrimSpace(call.ID)
				if key == "" {
					continue
				}
				batchKey := contextToolBatchKey{ownerIndex: messageIndex, callID: key}
				callOccurrences[batchKey] = append(callOccurrences[batchKey], contextToolCallOccurrence{
					messageIndex: messageIndex,
					callIndex:    callIndex,
					valid:        ValidToolCall(call),
				})
			}
		}
		if message.Role == agent.ToolRole {
			key := strings.TrimSpace(message.ToolCallID)
			if key == "" {
				continue
			}
			ownerIndex, owned := resultOwners[messageIndex]
			if !owned {
				continue
			}
			batchKey := contextToolBatchKey{ownerIndex: ownerIndex, callID: key}
			resultOccurrences[batchKey] = append(resultOccurrences[batchKey], contextToolResultOccurrence{
				messageIndex: messageIndex,
				ownerIndex:   ownerIndex,
				canonicalID:  message.ToolCallID == key,
			})
		}
	}

	keptCalls := make(map[int]map[int]bool)
	keptResults := make(map[int]bool)
	for batchKey, calls := range callOccurrences {
		if len(calls) != 1 || !calls[0].valid {
			continue
		}
		call := calls[0]
		results := resultOccurrences[batchKey]
		switch len(results) {
		case 0:
			rememberContextToolCall(keptCalls, call)
		case 1:
			result := results[0]
			if !result.canonicalID || result.ownerIndex != call.messageIndex {
				continue
			}
			rememberContextToolCall(keptCalls, call)
			keptResults[result.messageIndex] = true
		}
	}

	normalized := make([]*agent.Message, 0, len(messages))
	for messageIndex, message := range messages {
		if message == nil {
			continue
		}
		switch message.Role {
		case agent.ToolRole:
			if !keptResults[messageIndex] {
				continue
			}
			next := message.Clone()
			// A result cannot itself introduce another call half.
			next.ToolCalls = nil
			normalized = append(normalized, next)
		case agent.Assistant:
			next := message.Clone()
			hadToolCalls := len(next.ToolCalls) > 0
			next.ToolCallID = ""
			next.ToolName = ""
			next.ToolResult = nil
			if hadToolCalls {
				calls := make([]agent.ToolCall, 0, len(next.ToolCalls))
				for callIndex, call := range next.ToolCalls {
					if keptCalls[messageIndex][callIndex] {
						calls = append(calls, call)
					}
				}
				next.ToolCalls = calls
			}
			if hadToolCalls && len(next.ToolCalls) == 0 && !AssistantHasIndependentContent(next) {
				continue
			}
			normalized = append(normalized, next)
		default:
			next := message.Clone()
			// Tool protocol fields on system/user messages are malformed halves,
			// while their ordinary content remains independently meaningful.
			next.ToolCalls = nil
			next.ToolCallID = ""
			next.ToolName = ""
			next.ToolResult = nil
			normalized = append(normalized, next)
		}
	}

	normalized = completeUnknownContextToolBatches(normalized)
	if err := validateNormalizedModelContext(normalized); err != nil {
		return nil, err
	}
	return normalized, nil
}

// completeUnknownContextToolBatches scopes the existing recovery projection to
// one assistant response at a time. Provider call IDs are not globally unique
// across a transcript, so a later reused ID must not suppress a missing-result
// repair in the current batch.
func completeUnknownContextToolBatches(messages []*agent.Message) []*agent.Message {
	completed := make([]*agent.Message, 0, len(messages))
	for index := 0; index < len(messages); {
		message := messages[index]
		if message == nil || message.Role != agent.Assistant || len(message.ToolCalls) == 0 {
			completed = append(completed, message)
			index++
			continue
		}
		end := index + 1
		for end < len(messages) && messages[end] != nil && messages[end].Role == agent.ToolRole {
			end++
		}
		completed = append(completed, completeUnknownToolResults(messages[index:end])...)
		index = end
	}
	return completed
}

func contextToolResultOwners(messages []*agent.Message) map[int]int {
	owners := make(map[int]int)
	owner := -1
	for index, message := range messages {
		if message == nil {
			continue
		}
		switch message.Role {
		case agent.Assistant:
			owner = -1
			if len(message.ToolCalls) > 0 {
				owner = index
			}
		case agent.ToolRole:
			if owner >= 0 {
				owners[index] = owner
			}
		default:
			owner = -1
		}
	}
	return owners
}

// ValidToolCall reports whether a provider-neutral call has a canonical
// identity, supported type, and one complete JSON object for arguments.
func ValidToolCall(call agent.ToolCall) bool {
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
	return ValidateToolArgumentsJSON(call.Function.Arguments) == nil
}

func rememberContextToolCall(kept map[int]map[int]bool, call contextToolCallOccurrence) {
	if kept[call.messageIndex] == nil {
		kept[call.messageIndex] = make(map[int]bool)
	}
	kept[call.messageIndex][call.callIndex] = true
}

// AssistantHasIndependentContent reports whether removing malformed tool
// halves would still leave a meaningful assistant message.
func AssistantHasIndependentContent(message *agent.Message) bool {
	if message == nil {
		return false
	}
	return message.Content != "" || message.Name != "" || message.ReasoningContent != "" ||
		len(message.MultiContent) > 0 || len(message.AssistantGenMultiContent) > 0
}

func validateNormalizedModelContext(messages []*agent.Message) error {
	pending := make(map[string]bool)
	for index, message := range messages {
		if message == nil {
			return modelContextProtocolError("message %d is nil", index)
		}
		switch message.Role {
		case agent.Assistant:
			if len(pending) > 0 {
				return modelContextProtocolError("message %d starts before the previous tool batch is complete", index)
			}
			if message.ToolCallID != "" || message.ToolName != "" || message.ToolResult != nil {
				return modelContextProtocolError("assistant message %d contains result-only fields", index)
			}
			for _, call := range message.ToolCalls {
				if !ValidToolCall(call) {
					return modelContextProtocolError("assistant message %d contains an invalid tool call", index)
				}
				if pending[call.ID] {
					return modelContextProtocolError("tool call id %q is duplicated", call.ID)
				}
				pending[call.ID] = true
			}
		case agent.ToolRole:
			if len(message.ToolCalls) > 0 {
				return modelContextProtocolError("tool result message %d contains nested tool calls", index)
			}
			callID := strings.TrimSpace(message.ToolCallID)
			if callID == "" || message.ToolCallID != callID || !pending[callID] {
				return modelContextProtocolError("tool result message %d is orphaned or duplicated", index)
			}
			delete(pending, callID)
		case agent.System, agent.User, agent.RoleType("developer"):
			if len(pending) > 0 {
				return modelContextProtocolError("message %d interrupts an incomplete tool batch", index)
			}
			if len(message.ToolCalls) > 0 || message.ToolCallID != "" || message.ToolName != "" || message.ToolResult != nil {
				return modelContextProtocolError("message %d contains misplaced tool protocol fields", index)
			}
		default:
			return modelContextProtocolError("message %d has unsupported role %q", index, message.Role)
		}
	}
	if len(pending) > 0 {
		return modelContextProtocolError("final tool batch is incomplete")
	}
	return nil
}

func modelContextProtocolError(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalidModelContextProtocol, fmt.Sprintf(format, args...))
}

// ValidateToolArgumentsJSON accepts only one complete JSON object. Model
// context repair and live tool execution share this exact structural rule.
func ValidateToolArgumentsJSON(arguments string) error {
	arguments = strings.TrimSpace(arguments)
	if arguments == "" {
		return nil
	}
	decoder := json.NewDecoder(strings.NewReader(arguments))
	decoder.UseNumber()
	var payload map[string]any
	if err := decoder.Decode(&payload); err != nil {
		return err
	}
	if payload == nil {
		return fmt.Errorf("arguments must be a JSON object")
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("arguments contain trailing JSON data")
		}
		return fmt.Errorf("arguments contain trailing data: %w", err)
	}
	return nil
}

func completeUnknownToolResults(messages []*agent.Message) []*agent.Message {
	if len(messages) == 0 {
		return messages
	}
	completed := make([]*agent.Message, 0, len(messages))
	for index := 0; index < len(messages); {
		message := messages[index]
		if message == nil {
			index++
			continue
		}
		completed = append(completed, message)
		if message.Role != agent.Assistant || len(message.ToolCalls) == 0 {
			index++
			continue
		}

		batchEnd := index + 1
		for batchEnd < len(messages) && messages[batchEnd] != nil && messages[batchEnd].Role == agent.ToolRole {
			batchEnd++
		}
		callCounts := make(map[string]int, len(message.ToolCalls))
		resultCounts := make(map[string]int, batchEnd-index-1)
		for _, call := range message.ToolCalls {
			if callID := strings.TrimSpace(call.ID); callID != "" {
				callCounts[callID]++
			}
		}
		for resultIndex := index + 1; resultIndex < batchEnd; resultIndex++ {
			result := messages[resultIndex]
			if result != nil {
				if callID := strings.TrimSpace(result.ToolCallID); callID != "" {
					resultCounts[callID]++
				}
			}
		}
		for _, call := range message.ToolCalls {
			callID := strings.TrimSpace(call.ID)
			if !ValidToolCall(call) || callCounts[callID] != 1 || resultCounts[callID] != 0 {
				continue
			}
			completed = append(completed, agent.ToolMessage(
				agent.SyntheticToolResult(agent.ToolResultError, agent.ToolSyntheticEffectUnknown, agent.UnknownToolEffectResult),
				callID,
				agent.WithToolName(call.Function.Name),
			))
		}
		for resultIndex := index + 1; resultIndex < batchEnd; resultIndex++ {
			if messages[resultIndex] != nil {
				completed = append(completed, messages[resultIndex])
			}
		}
		index = batchEnd
	}
	return completed
}

// IsUnknownToolEffectResult identifies the deterministic recovery projection
// used when a durable tool start has no matching completion.
func IsUnknownToolEffectResult(content string) bool {
	return strings.TrimSpace(content) == agent.UnknownToolEffectResult
}
