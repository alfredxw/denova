package session

import (
	"context"
	"fmt"
	"strings"
	"time"

	agent "github.com/alfredxw/denova/agent"
)

// AppendDisplayEvent 追加仅用于前端展示的事件，不进入 Agent 有效上下文。
func (s *Session) AppendDisplayEvent(event DisplayEvent) error {
	if strings.TrimSpace(event.Role) == "" {
		return fmt.Errorf("展示事件 role 不能为空")
	}
	return s.withCanonicalMutation(context.Background(), "append display event", func() error {
		if event.CreatedAt.IsZero() {
			event.CreatedAt = time.Now().UTC()
		}
		recordID := newDisplayRecordID()
		if err := s.appendJournalRecordLocked(displayRecord{
			Type:         historyTypeDisplay,
			RecordID:     recordID,
			DisplayEvent: event,
		}); err != nil {
			return err
		}
		s.records = append(s.records, historyRecord{
			journalID:                    recordID,
			kind:                         historyTypeDisplay,
			display:                      &event,
			createdAt:                    event.CreatedAt,
			displayArgsPersistedBytes:    len(event.Args),
			displayContentPersistedBytes: len(event.Content),
		})
		if event.Role == "token_usage" {
			s.trimTokenUsageDisplayEventsLocked(event.AgentKind)
		}
		advanceUpdatedAt(s, event.CreatedAt)
		return nil
	})
}

// FinalizeDisplayAssistantRun classifies only display-only root prose for one
// completed run. It never mutates the canonical assistant message or model
// context. All earlier prose becomes progress; the terminal segment remains
// visible as either the final answer or a partial answer after interruption.
func (s *Session) FinalizeDisplayAssistantRun(runID, finalSegmentID, terminalPhase string) error {
	runID = strings.TrimSpace(runID)
	finalSegmentID = strings.TrimSpace(finalSegmentID)
	terminalPhase = strings.TrimSpace(terminalPhase)
	if runID == "" || finalSegmentID == "" {
		return nil
	}
	if terminalPhase != DisplayPhaseFinal && terminalPhase != DisplayPhasePartial {
		return fmt.Errorf("invalid assistant display terminal phase: %s", terminalPhase)
	}
	return s.withCanonicalMutation(context.Background(), "finalize display assistant run", func() error {
		now := time.Now().UTC()
		changed := false
		for index := range s.records {
			record := &s.records[index]
			if record.kind != historyTypeDisplay || record.display == nil || record.display.Role != "assistant" || record.display.SubAgent || strings.TrimSpace(record.display.RunID) != runID {
				continue
			}
			phase := DisplayPhaseProgress
			if record.display.ID == finalSegmentID {
				phase = terminalPhase
			}
			if record.display.DisplayPhase == phase {
				continue
			}
			phaseCopy := phase
			if err := s.appendJournalRecordLocked(displayPatchRecord{
				Type: historyTypeDisplayPatch, TargetRecordID: record.journalID,
				CreatedAt: now, DisplayPhase: &phaseCopy,
			}); err != nil {
				return err
			}
			record.display.DisplayPhase = phase
			changed = true
		}
		if changed {
			advanceUpdatedAt(s, now)
		}
		return nil
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

// UpdateDisplayToolStatus 更新已持久化工具卡片的执行状态，不保存工具参数或输出。
func (s *Session) UpdateDisplayToolStatus(id, name, status string) error {
	id = strings.TrimSpace(id)
	name = strings.TrimSpace(name)
	return s.withCanonicalMutation(context.Background(), "update display tool status", func() error {
		if index := findDisplayToolRecordIndex(s.records, id, name); index >= 0 {
			record := &s.records[index]
			now := time.Now().UTC()
			statusCopy := status
			pendingArgs := record.display.Args[record.displayArgsPersistedBytes:]
			if err := s.appendJournalRecordLocked(displayPatchRecord{
				Type:           historyTypeDisplayPatch,
				TargetRecordID: s.records[index].journalID,
				CreatedAt:      now,
				Status:         &statusCopy,
				ArgsAppend:     pendingArgs,
			}); err != nil {
				return err
			}
			record.display.Status = status
			record.displayArgsPersistedBytes = len(record.display.Args)
			advanceUpdatedAt(s, now)
		}
		return nil
	})
}

// AppendDisplayToolArgs appends streamed tool arguments to a persisted tool card.
func (s *Session) AppendDisplayToolArgs(id, name, delta string) error {
	id = strings.TrimSpace(id)
	name = strings.TrimSpace(name)
	if delta == "" {
		return nil
	}
	appliedLocally := false
	shouldFlush := false
	s.mu.Lock()
	if index := findDisplayToolRecordIndex(s.records, id, name); index >= 0 {
		record := &s.records[index]
		record.display.Args += delta
		appliedLocally = true
		shouldFlush = len(record.display.Args)-record.displayArgsPersistedBytes >= displayStreamPersistBatchBytes
		advanceUpdatedAt(s, time.Now().UTC())
	}
	s.mu.Unlock()
	if appliedLocally && !shouldFlush {
		return nil
	}
	return s.withCanonicalMutation(context.Background(), "append display tool arguments", func() error {
		if index := findDisplayToolRecordIndex(s.records, id, name); index >= 0 {
			record := &s.records[index]
			now := time.Now().UTC()
			if !appliedLocally {
				record.display.Args += delta
			}
			pending := record.display.Args[record.displayArgsPersistedBytes:]
			if len(pending) < displayStreamPersistBatchBytes {
				advanceUpdatedAt(s, now)
				return nil
			}
			if err := s.appendJournalRecordLocked(displayPatchRecord{
				Type:           historyTypeDisplayPatch,
				TargetRecordID: s.records[index].journalID,
				CreatedAt:      now,
				ArgsAppend:     pending,
			}); err != nil {
				return err
			}
			record.displayArgsPersistedBytes = len(record.display.Args)
			advanceUpdatedAt(s, now)
		}
		return nil
	})
}

func truncateUTF8ByBytes(value string, maxBytes int) string {
	if maxBytes <= 0 || value == "" {
		return ""
	}
	if len(value) <= maxBytes {
		return value
	}
	lastBoundary := 0
	for index := range value {
		if index > maxBytes {
			break
		}
		lastBoundary = index
	}
	if lastBoundary <= 0 {
		return ""
	}
	return value[:lastBoundary]
}

// UpdateDisplayToolResult stores the result preview for a persisted tool card.
func (s *Session) UpdateDisplayToolResult(id, name, status, result string, presentation *agent.ToolPresentation) error {
	id = strings.TrimSpace(id)
	name = strings.TrimSpace(name)
	return s.withCanonicalMutation(context.Background(), "update display tool result", func() error {
		if index := findDisplayToolRecordIndex(s.records, id, name); index >= 0 {
			record := &s.records[index]
			now := time.Now().UTC()
			statusCopy := status
			resultCopy := result
			pendingArgs := record.display.Args[record.displayArgsPersistedBytes:]
			if err := s.appendJournalRecordLocked(displayPatchRecord{
				Type:             historyTypeDisplayPatch,
				TargetRecordID:   s.records[index].journalID,
				CreatedAt:        now,
				Status:           &statusCopy,
				Result:           &resultCopy,
				ToolPresentation: cloneSessionToolPresentation(presentation),
				ArgsAppend:       pendingArgs,
			}); err != nil {
				return err
			}
			record.display.Status = status
			record.display.Result = result
			if normalized := cloneSessionToolPresentation(presentation); normalized != nil {
				record.display.ToolPresentation = normalized
			}
			record.displayArgsPersistedBytes = len(record.display.Args)
			advanceUpdatedAt(s, now)
		}
		return nil
	})
}

func (s *Session) UpdateDisplayToolIllustration(id, name string, illustration *ChapterIllustration) error {
	if illustration == nil {
		return nil
	}
	id = strings.TrimSpace(id)
	name = strings.TrimSpace(name)
	return s.withCanonicalMutation(context.Background(), "update display tool illustration", func() error {
		if index := findDisplayToolRecordIndex(s.records, id, name); index >= 0 {
			now := time.Now().UTC()
			illustrationCopy := cloneChapterIllustration(illustration)
			if err := s.appendJournalRecordLocked(displayPatchRecord{
				Type:           historyTypeDisplayPatch,
				TargetRecordID: s.records[index].journalID,
				CreatedAt:      now,
				Illustration:   illustrationCopy,
			}); err != nil {
				return err
			}
			s.records[index].display.Illustration = illustrationCopy
			advanceUpdatedAt(s, now)
		}
		return nil
	})
}

// AppendDisplayEventContent appends streamed display-only content to a card.
func (s *Session) AppendDisplayEventContent(id, role, delta string) error {
	id = strings.TrimSpace(id)
	role = strings.TrimSpace(role)
	if id == "" || role == "" || delta == "" {
		return nil
	}
	appliedLocally := false
	shouldFlush := false
	s.mu.Lock()
	for i := len(s.records) - 1; i >= 0; i-- {
		record := &s.records[i]
		if record.kind != historyTypeDisplay || record.display == nil || record.display.ID != id || record.display.Role != role {
			continue
		}
		record.display.Content += delta
		appliedLocally = true
		shouldFlush = len(record.display.Content)-record.displayContentPersistedBytes >= displayStreamPersistBatchBytes
		advanceUpdatedAt(s, time.Now().UTC())
		break
	}
	s.mu.Unlock()
	if appliedLocally && !shouldFlush {
		return nil
	}
	return s.withCanonicalMutation(context.Background(), "append display event content", func() error {
		for i := len(s.records) - 1; i >= 0; i-- {
			record := &s.records[i]
			if record.kind != historyTypeDisplay || record.display == nil {
				continue
			}
			if record.display.ID == id && record.display.Role == role {
				if !appliedLocally {
					record.display.Content += delta
				}
				pending := record.display.Content[record.displayContentPersistedBytes:]
				now := time.Now().UTC()
				if len(pending) < displayStreamPersistBatchBytes {
					advanceUpdatedAt(s, now)
					return nil
				}
				if err := s.appendJournalRecordLocked(displayPatchRecord{
					Type:           historyTypeDisplayPatch,
					TargetRecordID: record.journalID,
					CreatedAt:      now,
					ContentAppend:  pending,
				}); err != nil {
					return err
				}
				record.displayContentPersistedBytes = len(record.display.Content)
				advanceUpdatedAt(s, now)
				return nil
			}
		}
		return nil
	})
}

// FlushDisplayEventContent commits the final streamed display tail at a part
// boundary without forcing one fsync per token.
func (s *Session) FlushDisplayEventContent(id, role string) error {
	id = strings.TrimSpace(id)
	role = strings.TrimSpace(role)
	if id == "" || role == "" {
		return nil
	}
	hasPending := false
	s.mu.Lock()
	for i := len(s.records) - 1; i >= 0; i-- {
		record := &s.records[i]
		if record.kind == historyTypeDisplay && record.display != nil && record.display.ID == id && record.display.Role == role {
			hasPending = record.displayContentPersistedBytes < len(record.display.Content)
			break
		}
	}
	s.mu.Unlock()
	if !hasPending {
		return nil
	}
	return s.withCanonicalMutation(context.Background(), "flush display event content", func() error {
		for i := len(s.records) - 1; i >= 0; i-- {
			record := &s.records[i]
			if record.kind != historyTypeDisplay || record.display == nil || record.display.ID != id || record.display.Role != role {
				continue
			}
			pending := record.display.Content[record.displayContentPersistedBytes:]
			if pending == "" {
				return nil
			}
			now := time.Now().UTC()
			if err := s.appendJournalRecordLocked(displayPatchRecord{
				Type: historyTypeDisplayPatch, TargetRecordID: record.journalID,
				CreatedAt: now, ContentAppend: pending,
			}); err != nil {
				return err
			}
			record.displayContentPersistedBytes = len(record.display.Content)
			advanceUpdatedAt(s, now)
			return nil
		}
		return nil
	})
}

func findDisplayToolRecordIndex(records []historyRecord, id, name string) int {
	if id != "" {
		for i := len(records) - 1; i >= 0; i-- {
			if isDisplayToolRecord(records[i]) && records[i].display.ID == id {
				return i
			}
		}
		return -1
	}
	if name != "" {
		match := -1
		for i := len(records) - 1; i >= 0; i-- {
			if isDisplayToolRecord(records[i]) && records[i].display.Name == name {
				if match >= 0 {
					return -1
				}
				match = i
			}
		}
		return match
	}
	if id == "" && name == "" {
		for i := len(records) - 1; i >= 0; i-- {
			if isDisplayToolRecord(records[i]) {
				return i
			}
		}
	}
	return -1
}

func isDisplayToolRecord(record historyRecord) bool {
	return record.kind == historyTypeDisplay && record.display != nil && record.display.Role == "tool_call"
}
