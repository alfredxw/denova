package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/alfredxw/denova/agent/context/history"
	"github.com/alfredxw/denova/agent/schema"
	"github.com/alfredxw/denova/agent/session/canonical"
)

// RestoreCanonicalCheckpoint reconstructs an aligned checkpoint using only its
// retained message coordinates. It also accepts released checkpoints containing
// duplicate bodies: those bodies are replaced with the canonical journal values.
//
// The checkpoint is a bounded cache over the canonical journal, never the
// recovery authority. When the retained window cannot be proven identical to
// the persisted fingerprint, the checkpoint is declined so the caller rebuilds
// from the journal instead of rejecting a conversation whose history is still
// complete and readable.
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
		if message != nil && message.Role == schema.User && !history.IsContextStateMessage(message) && !schema.IsAgentContinuationMessage(message) {
			pendingUser = message
			return nil
		}
		appendMessage(message)
		return nil
	}
	if window, ok := source.(canonical.CanonicalHistoryWindow); ok {
		messages, err := window.ReadCanonicalWindow(ctx, checkpoint.MessageCount, checkpoint.Archive)
		if err != nil {
			if errors.Is(err, canonical.ErrInvalidCanonicalMessages) {
				declineCanonicalCheckpoint(ctx, head, checkpoint, "window coordinates changed", err)
				return nil, false, nil
			}
			return nil, false, err
		}
		state.Messages = messages
		count = checkpoint.MessageCount
	} else if visitor, ok := source.(canonical.CanonicalHistoryVisitor); ok {
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
		declineCanonicalCheckpoint(ctx, head, checkpoint, "message count changed",
			fmt.Errorf("%w: aligned checkpoint message count changed (%d != %d)", canonical.ErrInvalidCanonicalMessages, count, checkpoint.MessageCount))
		return nil, false, nil
	}
	hash, err := schema.HashCanonicalMessages(state.Messages)
	if err != nil {
		return nil, false, err
	}
	if hash != checkpoint.Hash {
		declineCanonicalCheckpoint(ctx, head, checkpoint, "content changed",
			fmt.Errorf("%w: aligned checkpoint content changed", canonical.ErrInvalidCanonicalMessages))
		return nil, false, nil
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

// declineCanonicalCheckpoint records why the fast path was abandoned. Rebuilding
// the transcript allocates from the canonical journal, which is a bounded cost
// paid once per cold load rather than a conversation-level failure.
func declineCanonicalCheckpoint(ctx context.Context, head canonical.CanonicalHistoryHead, checkpoint PersistedMessageCheckpoint, reason string, err error) {
	slog.WarnContext(ctx, "canonical Agent checkpoint declined; rebuilding from the canonical journal",
		"reason", reason,
		"history_identity", head.Identity,
		"history_revision", head.Revision,
		"checkpoint_message_count", checkpoint.MessageCount,
		"checkpoint_has_archive", checkpoint.Archive != nil,
		"checkpoint_pending_messages", len(checkpoint.Pending),
		"error", err,
	)
}
