package session

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// AppendDisplayEvent appends one immutable, display-only record (thinking,
// settled tool card, intermediate progress prose, partial aborted prose,
// compaction, token usage, execution summary, or plan) without affecting model
// context. Streaming deltas stay in process memory and are never patched to disk.
func (s *Session) AppendDisplayEvent(event DisplayEvent) error {
	if strings.TrimSpace(event.Role) == "" {
		return fmt.Errorf("展示事件 role 不能为空")
	}
	return s.withCanonicalMutation(context.Background(), "append display event", func() error {
		if event.CreatedAt.IsZero() {
			event.CreatedAt = time.Now().UTC()
		}
		return s.appendDisplayEventLocked(event)
	})
}

func (s *Session) appendDisplayEventLocked(event DisplayEvent) error {
	event.Ask = cloneAskInteraction(event.Ask)
	recordID := newDisplayRecordID()
	if err := s.appendJournalRecordLocked(displayRecord{
		Type:         historyTypeDisplay,
		RecordID:     recordID,
		DisplayEvent: event,
	}); err != nil {
		return err
	}
	s.records = append(s.records, historyRecord{
		journalID: recordID,
		kind:      historyTypeDisplay,
		display:   &event,
		createdAt: event.CreatedAt,
	})
	if event.Role == "token_usage" {
		s.trimTokenUsageDisplayEventsLocked(event.AgentKind)
	}
	advanceUpdatedAt(s, event.CreatedAt)
	return nil
}

// RecordDisplayAsk inserts or updates the display projection for one Ask
// interaction so pending and resolved states share a single timeline card.
func (s *Session) RecordDisplayAsk(event DisplayEvent) error {
	if event.Ask == nil {
		return fmt.Errorf("display Ask interaction is required")
	}
	interactionID := strings.TrimSpace(event.Ask.ID)
	if interactionID == "" {
		return fmt.Errorf("display Ask interaction ID is required")
	}
	status := strings.TrimSpace(event.Ask.Status)
	if status != AskPending && status != AskAnswered && status != AskCancelled {
		return fmt.Errorf("invalid display Ask status: %s", status)
	}
	return s.withCanonicalMutation(context.Background(), "record display Ask", func() error {
		now := time.Now().UTC()
		interaction := cloneAskInteraction(event.Ask)
		for index := len(s.records) - 1; index >= 0; index-- {
			record := &s.records[index]
			if record.kind != historyTypeDisplay || record.display == nil || record.display.Role != "ask" || record.display.ID != interactionID {
				continue
			}
			if interaction.CreatedAt.IsZero() {
				interaction.CreatedAt = record.display.CreatedAt
			}
			if interaction.CreatedAt.IsZero() {
				interaction.CreatedAt = now
			}
			if status != AskPending && interaction.ResolvedAt == nil {
				resolvedAt := now
				interaction.ResolvedAt = &resolvedAt
			}
			statusCopy := status
			if err := s.appendJournalRecordLocked(displayPatchRecord{
				Type: historyTypeDisplayPatch, TargetRecordID: record.journalID,
				CreatedAt: now, Status: &statusCopy, Ask: cloneAskInteraction(interaction),
			}); err != nil {
				return err
			}
			record.display.Status = status
			record.display.Ask = interaction
			advanceUpdatedAt(s, now)
			return nil
		}

		if interaction.CreatedAt.IsZero() {
			interaction.CreatedAt = now
		}
		if status != AskPending && interaction.ResolvedAt == nil {
			resolvedAt := now
			interaction.ResolvedAt = &resolvedAt
		}
		event.ID = interactionID
		event.Role = "ask"
		event.Status = status
		event.Ask = interaction
		if event.CreatedAt.IsZero() {
			event.CreatedAt = interaction.CreatedAt
		}
		return s.appendDisplayEventLocked(event)
	})
}

func (s *Session) trimTokenUsageDisplayEventsLocked(agentKind string) {
	s.records = trimTokenUsageDisplayEvents(s.records, agentKind)
}

func trimTokenUsageDisplayEvents(records []historyRecord, agentKind string) []historyRecord {
	target := strings.TrimSpace(agentKind)
	counts := make(map[string]int)
	kept := records
	for i := len(kept) - 1; i >= 0; i-- {
		record := kept[i]
		if record.kind != historyTypeDisplay || record.display == nil || record.display.Role != "token_usage" {
			continue
		}
		key := tokenUsageAgentKey(record.display.AgentKind)
		if target != "" && key != tokenUsageAgentKey(target) {
			continue
		}
		counts[key]++
		if counts[key] <= maxTokenUsageDisplayEvents {
			continue
		}
		kept = append(kept[:i], kept[i+1:]...)
	}
	return kept
}

func tokenUsageAgentKey(agentKind string) string {
	agentKind = strings.TrimSpace(agentKind)
	if agentKind == "" {
		return "__unknown__"
	}
	return agentKind
}
