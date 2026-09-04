package session

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	agent "github.com/alfredxw/denova/agent"

	"denova/internal/agents/conversationjournal"
)

type ContextBatchReceipt struct {
	ContextRevision uint64
	Cursor          ContextCursor
}

// CommitContextBatch atomically publishes one UI-hidden Agent protocol batch.
// Exact retries return the original receipt; reusing a cycle sequence with
// different content fails closed.
func (s *Session) CommitContextBatch(
	ctx context.Context,
	expected ContextCursor,
	identity DomainCommitIdentity,
	sequence int,
	messages []*agent.Message,
) (_ ContextBatchReceipt, resultErr error) {
	var receipt ContextBatchReceipt
	identity = normalizeDomainCommitIdentity(identity)
	values := make([]agent.Message, len(messages))
	for index, message := range messages {
		if message == nil {
			return ContextBatchReceipt{}, fmt.Errorf("context batch message %d is nil", index)
		}
		values[index] = *message.Clone()
	}
	candidate := contextBatchRecord{
		Type: historyTypeContextBatch, Identity: identity, Sequence: sequence, Messages: values,
	}
	if err := validateContextBatchRecord(candidate); err != nil {
		return ContextBatchReceipt{}, err
	}
	resultErr = s.withCanonicalMutation(ctx, "commit Agent context batch", func() error {
		if existing, found, err := s.findContextBatchLocked(ctx, identity, sequence, values); err != nil || found {
			receipt = existing
			return err
		}
		if next := s.nextContextBatchSequenceLocked(identity); sequence != next {
			return fmt.Errorf(
				"%w: context batch sequence %d is not the next sequence %d",
				ErrDomainCommitIdentityConflict, sequence, next,
			)
		}
		if current := s.contextCursorLocked(); current.Revision != expected.Revision {
			return fmt.Errorf("%w: expected=%d current=%d", ErrContextRevisionConflict, expected.Revision, current.Revision)
		}
		now := time.Now().UTC()
		candidate.CreatedAt = now
		candidate.ContextRevision = s.contextRevision + uint64(len(values))
		if err := s.appendJournalRecordLocked(candidate); err != nil {
			if recoveryErr := s.refreshCanonicalTailLocked(); recoveryErr == nil {
				reconciled, found, reconcileErr := s.findContextBatchLocked(ctx, identity, sequence, values)
				if reconcileErr != nil {
					return errors.Join(err, reconcileErr)
				}
				if found {
					receipt = reconciled
					return nil
				}
			} else {
				return errors.Join(err, recoveryErr)
			}
			return err
		}
		for index := range values {
			message := values[index].Clone()
			s.messages = append(s.messages, message)
			s.messageCount++
			s.records = append(s.records, historyRecord{
				kind: historyTypeContextMessage, message: message, createdAt: now,
			})
		}
		s.contextRevision = candidate.ContextRevision
		advanceUpdatedAt(s, now)
		receipt = ContextBatchReceipt{
			ContextRevision: candidate.ContextRevision, Cursor: s.contextCursorLocked(),
		}
		return nil
	})
	return receipt, resultErr
}

func (s *Session) nextContextBatchSequenceLocked(identity DomainCommitIdentity) int {
	next := 0
	if s.projection == nil {
		return next
	}
	for _, item := range s.projection.RecentContextBatches {
		if item.Identity == identity && item.Sequence >= next {
			next = item.Sequence + 1
		}
	}
	return next
}

func (s *Session) findContextBatchLocked(
	ctx context.Context,
	identity DomainCommitIdentity,
	sequence int,
	messages []agent.Message,
) (ContextBatchReceipt, bool, error) {
	if s.projection == nil {
		return ContextBatchReceipt{}, false, nil
	}
	for index := len(s.projection.RecentContextBatches) - 1; index >= 0; index-- {
		item := s.projection.RecentContextBatches[index]
		if item.Identity != identity || item.Sequence != sequence {
			continue
		}
		records, err := s.journal.ReadRange(ctx, conversationjournal.Range{
			After: item.Cursor - 1, Through: item.Cursor,
		})
		if err != nil {
			return ContextBatchReceipt{}, false, fmt.Errorf("read context batch sequence %d: %w", sequence, err)
		}
		if len(records) != 1 || records[0].Location.Cursor != item.Cursor {
			return ContextBatchReceipt{}, false, fmt.Errorf("context batch sequence %d is missing from its journal cursor", sequence)
		}
		var stored contextBatchRecord
		if err := json.Unmarshal(records[0].Payload, &stored); err != nil {
			return ContextBatchReceipt{}, false, fmt.Errorf("decode context batch sequence %d: %w", sequence, err)
		}
		equal, err := contextBatchMessagesEqual(stored.Messages, messages)
		if err != nil {
			return ContextBatchReceipt{}, false, err
		}
		if !equal {
			return ContextBatchReceipt{}, false, fmt.Errorf("%w: context batch identity was reused with different content", ErrDomainCommitIdentityConflict)
		}
		return ContextBatchReceipt{
			ContextRevision: item.ContextRevision, Cursor: item.ResultCursor,
		}, true, nil
	}
	return ContextBatchReceipt{}, false, nil
}

func validateContextBatchRecord(batch contextBatchRecord) error {
	if err := validateDomainCommitIdentity(normalizeDomainCommitIdentity(batch.Identity)); err != nil {
		return err
	}
	if batch.Sequence < 0 || len(batch.Messages) == 0 {
		return fmt.Errorf("invalid Agent context batch")
	}
	messages := make([]*agent.Message, len(batch.Messages))
	for index := range batch.Messages {
		message := &batch.Messages[index]
		if message.Role == "" || message.Role == agent.System {
			return fmt.Errorf("context batch message %d has invalid role %q", index, message.Role)
		}
		messages[index] = message.Clone()
	}
	if err := agent.ValidateContextCommitMessages(messages); err != nil {
		return fmt.Errorf("invalid Agent context batch messages: %w", err)
	}
	return nil
}

func contextBatchMessagesEqual(left, right []agent.Message) (bool, error) {
	leftJSON, err := json.Marshal(left)
	if err != nil {
		return false, fmt.Errorf("encode stored context batch messages: %w", err)
	}
	rightJSON, err := json.Marshal(right)
	if err != nil {
		return false, fmt.Errorf("encode candidate context batch messages: %w", err)
	}
	return bytes.Equal(leftJSON, rightJSON), nil
}

func appendContextBatchRecordLine(sess *Session, line []byte) error {
	var batch contextBatchRecord
	if err := json.Unmarshal(line, &batch); err != nil {
		return err
	}
	if err := validateContextBatchRecord(batch); err != nil {
		return err
	}
	for index := range batch.Messages {
		message := batch.Messages[index].Clone()
		sess.messages = append(sess.messages, message)
		sess.messageCount++
		sess.records = append(sess.records, historyRecord{
			kind: historyTypeContextMessage, message: message, createdAt: batch.CreatedAt,
		})
	}
	if batch.ContextRevision > sess.contextRevision {
		sess.contextRevision = batch.ContextRevision
	}
	advanceUpdatedAt(sess, batch.CreatedAt)
	return nil
}
