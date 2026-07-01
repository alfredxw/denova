package session

import (
	"fmt"
	"strings"
	"time"

	"github.com/cloudwego/eino/schema"
)

// Append 追加消息并持久化到磁盘。
func (s *Session) Append(msg *schema.Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	s.messages = append(s.messages, msg)
	s.records = append(s.records, historyRecord{kind: historyTypeMessage, message: msg, createdAt: now})
	s.UpdatedAt = now
	if s.title == defaultSessionTitle && msg.Role == schema.User && strings.TrimSpace(msg.Content) != "" {
		s.title = deriveTitle(msg.Content)
	}

	return s.persistLocked()
}

// AppendClearMarker 追加上下文清理标记，不删除历史消息。
func (s *Session) AppendClearMarker() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	s.clearAfterIndex = len(s.messages)
	s.records = append(s.records, historyRecord{kind: historyTypeClear, createdAt: now})
	s.UpdatedAt = now
	return s.persistLocked()
}

// GetMessages 返回所有消息的快照。
func (s *Session) GetMessages() []*schema.Message {
	s.mu.Lock()
	defer s.mu.Unlock()

	result := make([]*schema.Message, len(s.messages))
	copy(result, s.messages)
	return result
}

// GetEffectiveMessages 返回最后一个清理标记之后的 Agent 有效上下文。
func (s *Session) GetEffectiveMessages() []*schema.Message {
	s.mu.Lock()
	defer s.mu.Unlock()

	result := make([]*schema.Message, len(s.messages)-s.clearAfterIndex)
	copy(result, s.messages[s.clearAfterIndex:])
	return result
}

// MessageCountSinceClear returns the number of effective raw transcript
// messages after the latest clear marker.
func (s *Session) MessageCountSinceClear() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.messages) - s.clearAfterIndex
}

// MessageCountTotal returns the raw persisted message count.
func (s *Session) MessageCountTotal() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.messages)
}

// History 返回包含 clear 标记的完整会话历史。
func (s *Session) History() []HistoryEntry {
	s.mu.Lock()
	defer s.mu.Unlock()

	result := make([]HistoryEntry, 0, len(s.records))
	for _, record := range s.records {
		switch record.kind {
		case historyTypeClear:
			result = append(result, HistoryEntry{Type: historyTypeClear, CreatedAt: record.createdAt})
		case historyTypeMessage:
			if record.message == nil {
				continue
			}
			result = append(result, HistoryEntry{
				Type:      historyTypeMessage,
				Role:      string(record.message.Role),
				Content:   record.message.Content,
				Message:   record.message,
				CreatedAt: record.createdAt,
			})
		case historyTypeDisplay:
			if record.display == nil {
				continue
			}
			result = append(result, HistoryEntry{
				Type:                 historyTypeMessage,
				ID:                   record.display.ID,
				Role:                 record.display.Role,
				Content:              record.display.Content,
				Name:                 record.display.Name,
				Args:                 record.display.Args,
				Status:               record.display.Status,
				Result:               record.display.Result,
				Illustration:         cloneChapterIllustration(record.display.Illustration),
				CreatedAt:            record.display.CreatedAt,
				RunID:                record.display.RunID,
				AgentKind:            record.display.AgentKind,
				AgentName:            record.display.AgentName,
				RootAgentName:        record.display.RootAgentName,
				RunPath:              append([]string(nil), record.display.RunPath...),
				SubAgent:             record.display.SubAgent,
				SubAgentSessionID:    record.display.SubAgentSessionID,
				SubAgentType:         record.display.SubAgentType,
				PromptTokens:         record.display.PromptTokens,
				CachedPromptTokens:   record.display.CachedPromptTokens,
				UncachedPromptTokens: record.display.UncachedPromptTokens,
				CacheHitRate:         record.display.CacheHitRate,
				CompletionTokens:     record.display.CompletionTokens,
				ReasoningTokens:      record.display.ReasoningTokens,
				TotalTokens:          record.display.TotalTokens,
				ModelCalls:           record.display.ModelCalls,
				GeneratedBytes:       record.display.GeneratedBytes,
				UsageCalls:           cloneTokenUsageCalls(record.display.UsageCalls),
			})
		}
	}
	return result
}

func cloneTokenUsageCalls(calls []TokenUsageCall) []TokenUsageCall {
	if len(calls) == 0 {
		return nil
	}
	result := make([]TokenUsageCall, len(calls))
	copy(result, calls)
	for i := range result {
		result[i].RequestedTools = append([]string(nil), result[i].RequestedTools...)
		result[i].AfterTools = append([]string(nil), result[i].AfterTools...)
	}
	return result
}

func cloneChapterIllustration(value *ChapterIllustration) *ChapterIllustration {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}

// Clear 兼容旧调用语义：追加 clear 标记，不物理删除消息。
func (s *Session) Clear() error {
	return s.AppendClearMarker()
}

// Rename 更新会话标题并持久化。
func (s *Session) Rename(title string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	title = strings.TrimSpace(title)
	if title == "" {
		return fmt.Errorf("会话标题不能为空")
	}
	s.title = title
	s.touchLocked()
	return s.persistLocked()
}

// Title 返回持久化会话标题。
func (s *Session) Title() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.titleLocked()
}

// MessageCount 返回消息数量。
func (s *Session) MessageCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.messages)
}

func (s *Session) titleLocked() string {
	if strings.TrimSpace(s.title) != "" {
		return s.title
	}
	return defaultSessionTitle
}

func (s *Session) touchLocked() {
	s.UpdatedAt = time.Now().UTC()
}

func deriveTitle(content string) string {
	title := strings.TrimSpace(content)
	if len([]rune(title)) > 60 {
		title = string([]rune(title)[:60]) + "..."
	}
	if title == "" {
		return defaultSessionTitle
	}
	return title
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
