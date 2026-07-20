package session

import (
	"fmt"
	"strings"
	"time"
)

// AppendDisplayEvent 追加仅用于前端展示的事件，不进入 Agent 有效上下文。
func (s *Session) AppendDisplayEvent(event DisplayEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if strings.TrimSpace(event.Role) == "" {
		return fmt.Errorf("展示事件 role 不能为空")
	}
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now().UTC()
	}
	s.records = append(s.records, historyRecord{
		kind:                      historyTypeDisplay,
		display:                   &event,
		createdAt:                 event.CreatedAt,
		displayArgsPersistedBytes: len(event.Args),
	})
	if event.Role == "token_usage" {
		s.trimTokenUsageDisplayEventsLocked(event.AgentKind)
	}
	s.UpdatedAt = event.CreatedAt
	return s.persistLocked()
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
	s.mu.Lock()
	defer s.mu.Unlock()

	id = strings.TrimSpace(id)
	name = strings.TrimSpace(name)
	if index := findDisplayToolRecordIndex(s.records, id, name); index >= 0 {
		s.records[index].display.Status = status
		s.UpdatedAt = time.Now().UTC()
		return s.persistLocked()
	}
	return nil
}

// AppendDisplayToolArgs appends streamed tool arguments to a persisted tool card.
func (s *Session) AppendDisplayToolArgs(id, name, delta string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	id = strings.TrimSpace(id)
	name = strings.TrimSpace(name)
	if delta == "" {
		return nil
	}
	if index := findDisplayToolRecordIndex(s.records, id, name); index >= 0 {
		record := &s.records[index]
		record.display.Args += delta
		s.UpdatedAt = time.Now().UTC()
		if !shouldPersistDisplayToolArgs(record) {
			return nil
		}
		record.displayArgsPersistedBytes = len(record.display.Args)
		// 流式工具参数按批次保存；后续 tool_result 会再落完整卡片状态。
		_ = s.persistLocked()
		return nil
	}
	return nil
}

func shouldPersistDisplayToolArgs(record *historyRecord) bool {
	if record == nil || record.display == nil {
		return false
	}
	return len(record.display.Args)-record.displayArgsPersistedBytes >= displayToolArgsPersistBytes
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
func (s *Session) UpdateDisplayToolResult(id, name, status, result string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	id = strings.TrimSpace(id)
	name = strings.TrimSpace(name)
	if index := findDisplayToolRecordIndex(s.records, id, name); index >= 0 {
		s.records[index].display.Status = status
		s.records[index].display.Result = result
		s.UpdatedAt = time.Now().UTC()
		return s.persistLocked()
	}
	return nil
}

func (s *Session) UpdateDisplayToolIllustration(id, name string, illustration *ChapterIllustration) error {
	if illustration == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	id = strings.TrimSpace(id)
	name = strings.TrimSpace(name)
	if index := findDisplayToolRecordIndex(s.records, id, name); index >= 0 {
		s.records[index].display.Illustration = cloneChapterIllustration(illustration)
		s.UpdatedAt = time.Now().UTC()
		return s.persistLocked()
	}
	return nil
}

// AppendDisplayEventContent appends streamed display-only content to a card.
func (s *Session) AppendDisplayEventContent(id, role, delta string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	id = strings.TrimSpace(id)
	role = strings.TrimSpace(role)
	if id == "" || role == "" || delta == "" {
		return nil
	}
	for i := len(s.records) - 1; i >= 0; i-- {
		record := s.records[i]
		if record.kind != historyTypeDisplay || record.display == nil {
			continue
		}
		if record.display.ID == id && record.display.Role == role {
			s.records[i].display.Content += delta
			s.UpdatedAt = time.Now().UTC()
			return s.persistLocked()
		}
	}
	return nil
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
