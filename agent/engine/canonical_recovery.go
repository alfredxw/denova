package engine

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/alfredxw/denova/agent/context/history"
	"github.com/alfredxw/denova/agent/schema"
	"github.com/alfredxw/denova/agent/session/canonical"
)

// RestoreCanonicalCheckpoint reconstructs an aligned checkpoint using only its
// retained message coordinates. It also accepts released checkpoints containing
// duplicate bodies: those bodies are replaced with the canonical journal values.
func RestoreCanonicalCheckpoint(ctx context.Context, checkpoint PersistedMessageCheckpoint, head canonical.CanonicalHistoryHead, source canonical.CanonicalHistorySource) (json.RawMessage, bool, error) {
	if len(checkpoint.Metadata) == 0 || checkpoint.Hash == "" {
		return nil, false, nil
	}
	var state engineTranscript
	if err := json.Unmarshal(checkpoint.Metadata, &state); err != nil {
		return nil, false, err
	}
	if state.HistoryHead != head {
		return nil, false, nil
	}
	state.Messages = nil
	count := 0
	appendMessage := func(message *schema.Message) {
		if checkpoint.Archive.Contains(count) {
			state.Messages = append(state.Messages, message)
		}
		count++
	}
	// Admission persists user input before its context-state records. Reorder
	// with one pending user, exactly like CanonicalContextStateOrder, without
	// materializing the archived prefix.
	var pendingUser *schema.Message
	visit := func(message *schema.Message) error {
		if pendingUser != nil && !history.IsContextStateMessage(message) {
			appendMessage(pendingUser)
			pendingUser = nil
		}
		if message != nil && message.Role == schema.User && !history.IsContextStateMessage(message) {
			pendingUser = message
			return nil
		}
		appendMessage(message)
		return nil
	}
	if visitor, ok := source.(canonical.CanonicalHistoryVisitor); ok {
		if err := visitor.VisitCanonicalMessages(ctx, visit); err != nil {
			return nil, false, err
		}
	} else {
		messages, err := source.CanonicalMessages(ctx)
		if err != nil {
			return nil, false, err
		}
		for _, message := range messages {
			if err := visit(message); err != nil {
				return nil, false, err
			}
		}
	}
	if pendingUser != nil {
		appendMessage(pendingUser)
	}
	if count != checkpoint.MessageCount {
		return nil, false, fmt.Errorf("%w: aligned checkpoint message count changed (%d != %d)", canonical.ErrInvalidCanonicalMessages, count, checkpoint.MessageCount)
	}
	hash, err := schema.HashCanonical(state.Messages)
	if err != nil {
		return nil, false, err
	}
	if hash != checkpoint.Hash {
		return nil, false, fmt.Errorf("%w: aligned checkpoint content changed", canonical.ErrInvalidCanonicalMessages)
	}
	state.Messages = append(state.Messages, schema.CloneMessages(checkpoint.Pending)...)
	encoded, err := json.Marshal(state)
	if err != nil {
		return nil, false, err
	}
	if _, err := decodeEngineTranscript(encoded); err != nil {
		return nil, false, err
	}
	return encoded, true, nil
}
