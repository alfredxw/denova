package session

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// MarkInterrupted 记录一次异常中断，供用户后续明确要求继续时恢复。
func (s *Session) MarkInterrupted(userMessage, assistantContent, reason string) error {
	return s.withCanonicalMutation(context.Background(), "mark interruption", func() error {
		now := time.Now().UTC()
		record := &Interruption{
			ID:               newInterruptionID(),
			Status:           InterruptionPending,
			UserMessage:      strings.TrimSpace(userMessage),
			AssistantContent: assistantContent,
			Reason:           strings.TrimSpace(reason),
			CreatedAt:        now,
		}
		if err := s.appendJournalRecordLocked(interruptionRecord{
			Type:         historyTypeInterrupt,
			Interruption: *record,
		}); err != nil {
			return err
		}
		s.records = append(s.records, historyRecord{kind: historyTypeInterrupt, interruption: record, createdAt: now})
		advanceUpdatedAt(s, now)
		return nil
	})
}

// PendingInterruption 返回最近一条待恢复的异常中断记录。
func (s *Session) PendingInterruption() *Interruption {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i := len(s.records) - 1; i >= 0; i-- {
		record := s.records[i]
		if record.kind != historyTypeInterrupt || record.interruption == nil {
			continue
		}
		if record.interruption.Status == InterruptionPending {
			copied := *record.interruption
			return &copied
		}
	}
	return nil
}

// ResolveInterruption 标记异常中断已被恢复处理。
func (s *Session) ResolveInterruption(id string) error {
	return s.withCanonicalMutation(context.Background(), "resolve interruption", func() error {
		now := time.Now().UTC()
		for _, record := range s.records {
			if record.kind != historyTypeInterrupt || record.interruption == nil {
				continue
			}
			if record.interruption.ID == id {
				if err := s.appendJournalRecordLocked(interruptionPatchRecord{
					Type:       historyTypeInterruptionPatch,
					TargetID:   id,
					Status:     InterruptionResolved,
					ResolvedAt: &now,
					UpdatedAt:  now,
				}); err != nil {
					return err
				}
				record.interruption.Status = InterruptionResolved
				record.interruption.ResolvedAt = &now
				advanceUpdatedAt(s, now)
				return nil
			}
		}
		return fmt.Errorf("异常中断记录不存在: %s", id)
	})
}
