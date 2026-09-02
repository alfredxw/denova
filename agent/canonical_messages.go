package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// LoadCanonicalMessages refreshes an idle host-backed Session from the
// canonical conversation lane. Host-canonical logs keep only a compact
// checkpoint; standalone logs remain self-contained and persist the imported
// transcript normally.
func (session *Session) LoadCanonicalMessages(ctx context.Context, messages []*Message) error {
	if err := session.usable(); err != nil {
		return err
	}
	ordered := canonicalContextStateOrder(messages)
	if err := validateImportedTranscript(ordered); err != nil {
		return err
	}
	contextState, err := rebuildContextStateSnapshot(ordered)
	if err != nil {
		return err
	}
	session.mu.Lock()
	defer session.mu.Unlock()
	if session.active != nil {
		return ErrSessionBusy
	}
	hadCurrentTranscript := len(session.engineState) != 0
	currentMatches := false
	currentCompatible := false
	if hadCurrentTranscript {
		current, decodeErr := decodeEngineTranscript(session.engineState)
		if decodeErr != nil {
			return decodeErr
		}
		if len(current.Messages) <= len(ordered) {
			currentHash, hashErr := hashCanonical(current.Messages)
			if hashErr != nil {
				return hashErr
			}
			prefixHash, hashErr := hashCanonical(ordered[:len(current.Messages)])
			if hashErr != nil {
				return hashErr
			}
			currentCompatible = currentHash == prefixHash
			currentMatches = currentCompatible && len(current.Messages) == len(ordered)
		}
	}
	hadCheckpoint := strings.TrimSpace(session.messageCheckpoint.Hash) != ""
	checkpointCompatible := false
	if hadCheckpoint && session.messageCheckpoint.MessageCount <= len(ordered) {
		prefixHash, hashErr := hashCanonical(ordered[:session.messageCheckpoint.MessageCount])
		if hashErr != nil {
			return hashErr
		}
		checkpointCompatible = session.messageCheckpoint.Hash == prefixHash
	}
	// Capability records live in the same canonical journal as host messages.
	// A cold session with no message checkpoint can therefore trust them (this
	// is also how released Product Session compactions are imported). Only a
	// previously observed prefix can prove that source messages were edited or
	// removed underneath those projections. Ordinary append-only progress is
	// compatible even when a crash occurred before the next checkpoint.
	if (hadCurrentTranscript || hadCheckpoint) && !currentCompatible && !checkpointCompatible {
		for _, capability := range []string{
			clearCapability, cleanupCapability, compactionCapability, compactionHealthCapability,
		} {
			delete(session.capabilities, capability)
		}
		if err := session.persistCapabilitiesLocked(ctx); err != nil {
			return err
		}
	}
	encoded, err := json.Marshal(engineTranscript{
		Version: engineTranscriptVersion, Messages: cloneMessages(ordered), ContextState: contextState,
	})
	if err != nil {
		return fmt.Errorf("encode canonical Agent messages: %w", err)
	}
	session.engineState = encoded
	if !session.canonicalMessages && !currentMatches {
		if err := session.persistTranscriptLocked(ctx); err != nil {
			return err
		}
	}
	for _, message := range ordered {
		if message == nil || message.TaskCompletion == nil {
			continue
		}
		id := strings.TrimSpace(message.TaskCompletion.CompletionID)
		if id != "" {
			session.taskCompletions.delivered[id] = struct{}{}
			delete(session.taskCompletions.pending, id)
		}
	}
	return nil
}

// canonicalContextStateOrder restores the semantic order of context-state
// updates. The host must append accepted input first as the durable admission
// fence, so state updates committed for that cycle physically follow it even
// though they are model-visible immediately before that user message.
func canonicalContextStateOrder(messages []*Message) []*Message {
	input := cloneMessages(messages)
	result := make([]*Message, 0, len(input))
	for index := 0; index < len(input); {
		message := input[index]
		if message == nil || message.Role != User || IsContextStateMessage(message) {
			result = append(result, message)
			index++
			continue
		}
		end := index + 1
		for end < len(input) && IsContextStateMessage(input[end]) {
			end++
		}
		result = append(result, input[index+1:end]...)
		result = append(result, message)
		index = end
	}
	return result
}

func validateImportedTranscript(messages []*Message) error {
	pending := make(map[string]struct{})
	for index, message := range messages {
		if message == nil {
			return fmt.Errorf("Agent transcript message %d is nil", index)
		}
		if len(pending) > 0 && message.Role != ToolRole {
			return fmt.Errorf("Agent transcript message %d splits an incomplete tool-result batch", index)
		}
		switch message.Role {
		case User:
			if len(message.ToolCalls) != 0 || strings.TrimSpace(message.ToolCallID) != "" {
				return fmt.Errorf("Agent transcript user message %d contains tool protocol fields", index)
			}
		case Assistant:
			if strings.TrimSpace(message.ToolCallID) != "" {
				return fmt.Errorf("Agent transcript assistant message %d contains a tool result ID", index)
			}
			for _, call := range message.ToolCalls {
				id := strings.TrimSpace(call.ID)
				if id == "" || strings.TrimSpace(call.Function.Name) == "" {
					return fmt.Errorf("Agent transcript assistant message %d has an invalid tool call", index)
				}
				if _, duplicate := pending[id]; duplicate {
					return fmt.Errorf("Agent transcript assistant message %d repeats tool call %q", index, id)
				}
				pending[id] = struct{}{}
			}
		case ToolRole:
			if len(message.ToolCalls) != 0 {
				return fmt.Errorf("Agent transcript tool message %d contains nested tool calls", index)
			}
			id := strings.TrimSpace(message.ToolCallID)
			if _, ok := pending[id]; !ok || id == "" {
				return fmt.Errorf("Agent transcript tool message %d has no matching pending call", index)
			}
			delete(pending, id)
		case System:
			return fmt.Errorf("Agent transcript message %d is system-owned and cannot be product-imported", index)
		default:
			return fmt.Errorf("Agent transcript message %d has unsupported role %q", index, message.Role)
		}
	}
	if len(pending) > 0 {
		return errors.New("Agent transcript ends with an incomplete tool-result batch")
	}
	return nil
}
