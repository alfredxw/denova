package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	agent "github.com/alfredxw/denova/agent"
)

type ContextBatchReceipt struct {
	Hash            string
	ContextRevision uint64
	Cursor          ContextCursor
}

// CommitContextBatch atomically publishes one UI-hidden Agent protocol batch.
// Exact retries return the original receipt; reusing a cycle ordinal with
// different content fails closed.
func (s *Session) CommitContextBatch(
	ctx context.Context,
	expected ContextCursor,
	identity DomainCommitIdentity,
	kind string,
	ordinal int,
	hash string,
	messages []*agent.Message,
) (_ ContextBatchReceipt, resultErr error) {
	var receipt ContextBatchReceipt
	identity = normalizeDomainCommitIdentity(identity)
	kind, hash = strings.TrimSpace(kind), strings.TrimSpace(hash)
	values := make([]agent.Message, len(messages))
	for index, message := range messages {
		if message == nil {
			return ContextBatchReceipt{}, fmt.Errorf("context batch message %d is nil", index)
		}
		values[index] = *message.Clone()
	}
	candidate := contextBatchRecord{
		Type: historyTypeContextBatch, Identity: identity, Kind: kind,
		Ordinal: ordinal, Hash: hash, Messages: values,
	}
	if err := validateContextBatchRecord(candidate); err != nil {
		return ContextBatchReceipt{}, err
	}
	resultErr = s.withCanonicalMutation(ctx, "commit Agent context batch", func() error {
		if existing, found, err := s.findContextBatchLocked(identity, kind, ordinal, hash); err != nil || found {
			receipt = existing
			return err
		}
		if current := s.contextCursorLocked(); current.Revision != expected.Revision {
			return fmt.Errorf("%w: expected=%d current=%d", ErrContextRevisionConflict, expected.Revision, current.Revision)
		}
		now := time.Now().UTC()
		candidate.CreatedAt = now
		candidate.ContextRevision = s.contextRevision + uint64(len(values))
		if err := s.appendJournalRecordLocked(candidate); err != nil {
			if recoveryErr := s.refreshCanonicalTailLocked(); recoveryErr == nil {
				reconciled, found, reconcileErr := s.findContextBatchLocked(identity, kind, ordinal, hash)
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
			Hash: hash, ContextRevision: candidate.ContextRevision, Cursor: s.contextCursorLocked(),
		}
		return nil
	})
	return receipt, resultErr
}

func (s *Session) findContextBatchLocked(
	identity DomainCommitIdentity,
	kind string,
	ordinal int,
	hash string,
) (ContextBatchReceipt, bool, error) {
	if s.projection == nil {
		return ContextBatchReceipt{}, false, nil
	}
	for index := len(s.projection.RecentContextBatches) - 1; index >= 0; index-- {
		item := s.projection.RecentContextBatches[index]
		if item.Identity != identity || item.Kind != kind || item.Ordinal != ordinal {
			continue
		}
		if item.Hash != hash {
			return ContextBatchReceipt{}, false, fmt.Errorf("%w: context batch identity was reused with different content", ErrDomainCommitIdentityConflict)
		}
		return ContextBatchReceipt{
			Hash: item.Hash, ContextRevision: item.ContextRevision, Cursor: item.ResultCursor,
		}, true, nil
	}
	return ContextBatchReceipt{}, false, nil
}

func validateContextBatchRecord(batch contextBatchRecord) error {
	if err := validateDomainCommitIdentity(normalizeDomainCommitIdentity(batch.Identity)); err != nil {
		return err
	}
	if strings.TrimSpace(batch.Kind) == "" || batch.Ordinal < 0 || strings.TrimSpace(batch.Hash) == "" || len(batch.Messages) == 0 {
		return fmt.Errorf("invalid Agent context batch")
	}
	for index := range batch.Messages {
		message := &batch.Messages[index]
		if message.Role == "" || message.Role == agent.System {
			return fmt.Errorf("context batch message %d has invalid role %q", index, message.Role)
		}
	}
	return nil
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
