package session

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"
	"time"

	agent "github.com/alfredxw/denova/agent"

	agentcontext "denova/internal/agents/context"
)

// Append 追加消息并持久化到磁盘。
func (s *Session) Append(msg *agent.Message) error {
	return s.AppendWithMetadata(msg, MessageMetadata{})
}

func (s *Session) AppendWithMetadata(msg *agent.Message, metadata MessageMetadata) error {
	return s.withCanonicalMutation(context.Background(), "append message", func() error {
		return s.appendMessageLocked(msg, metadata, historyTypeMessage)
	})
}

func (s *Session) appendMessageLocked(msg *agent.Message, metadata MessageMetadata, kind string) error {
	return s.appendMessagesLocked([]*agent.Message{msg}, []MessageMetadata{metadata}, kind)
}

// appendMessagesLocked publishes one or more logical messages as one physical
// journal transaction. The batch is the atomicity seam used by paired tool
// call/result receipts; ordinary single-message appends share the same path.
func (s *Session) appendMessagesLocked(messages []*agent.Message, metadata []MessageMetadata, kind string) error {
	if len(messages) == 0 || len(messages) != len(metadata) {
		return fmt.Errorf("会话消息批次无效")
	}
	now := time.Now().UTC()
	records := make([]any, len(messages))
	normalizedMetadata := make([]MessageMetadata, len(messages))
	for index, msg := range messages {
		if msg == nil {
			return fmt.Errorf("会话消息 %d 为空", index)
		}
		if msg.Role == "" && strings.TrimSpace(msg.Content) == "" && len(msg.ToolCalls) == 0 {
			return fmt.Errorf("会话消息 %d 缺少 role、content 和 tool_calls", index)
		}
		entryMetadata := sanitizeMessageMetadata(metadata[index])
		entryMetadata.ContextRevision = s.contextRevision + uint64(index) + 1
		normalizedMetadata[index] = entryMetadata
		records[index] = messageRecord{
			Type: kind, CreatedAt: now, Message: *msg, MessageMetadata: entryMetadata,
		}
	}
	if _, err := s.appendJournalRecordsLocked(records...); err != nil {
		return err
	}
	for index, msg := range messages {
		s.messages = append(s.messages, msg)
		s.messageCount++
		s.records = append(s.records, historyRecord{
			kind: kind, message: msg, messageMetadata: normalizedMetadata[index], createdAt: now,
		})
		if s.title == defaultSessionTitle && msg.Role == agent.User && strings.TrimSpace(msg.Content) != "" {
			s.title = deriveTitle(msg.Content)
		}
	}
	s.contextRevision = normalizedMetadata[len(normalizedMetadata)-1].ContextRevision
	advanceUpdatedAt(s, now)
	return nil
}

func sanitizeMessageMetadata(metadata MessageMetadata) MessageMetadata {
	metadata.RunID = strings.TrimSpace(metadata.RunID)
	metadata.MessageID = strings.TrimSpace(metadata.MessageID)
	metadata.AgentCommandID = strings.TrimSpace(metadata.AgentCommandID)
	metadata.AgentOperationID = strings.TrimSpace(metadata.AgentOperationID)
	metadata.DomainCommitHash = strings.TrimSpace(metadata.DomainCommitHash)
	metadata.AgentCanonicalHash = strings.TrimSpace(metadata.AgentCanonicalHash)
	metadata.ResolveInterruptionID = strings.TrimSpace(metadata.ResolveInterruptionID)
	metadata.AgentKind = strings.TrimSpace(metadata.AgentKind)
	metadata.AgentName = strings.TrimSpace(metadata.AgentName)
	metadata.RootAgentName = strings.TrimSpace(metadata.RootAgentName)
	metadata.SubAgentSessionID = strings.TrimSpace(metadata.SubAgentSessionID)
	metadata.SubAgentType = strings.TrimSpace(metadata.SubAgentType)
	if len(metadata.RunPath) > 0 {
		out := make([]string, 0, len(metadata.RunPath))
		for _, step := range metadata.RunPath {
			step = strings.TrimSpace(step)
			if step != "" {
				out = append(out, step)
			}
		}
		metadata.RunPath = out
	}
	metadata.UserReferences = sanitizeUserMessageReferences(metadata.UserReferences)
	return metadata
}

const (
	maxUserMessageReferences      = 256
	maxUserReferenceLabelBytes    = 1024
	maxUserReferenceDetailBytes   = 2048
	maxUserReferenceMetadataBytes = 128 * 1024
)

func sanitizeUserMessageReferences(values []agentcontext.UserReference) []agentcontext.UserReference {
	result := make([]agentcontext.UserReference, 0, min(len(values), maxUserMessageReferences))
	totalBytes := 0
	for _, value := range values {
		if len(result) >= maxUserMessageReferences {
			break
		}
		value.Kind = strings.TrimSpace(value.Kind)
		value.ID = truncateUTF8ByBytes(strings.TrimSpace(value.ID), maxUserReferenceLabelBytes)
		value.Label = truncateUTF8ByBytes(strings.TrimSpace(value.Label), maxUserReferenceLabelBytes)
		value.Detail = truncateUTF8ByBytes(strings.TrimSpace(value.Detail), maxUserReferenceDetailBytes)
		if value.Kind == "" || value.Label == "" {
			continue
		}
		if value.StartLine < 0 {
			value.StartLine = 0
		}
		if value.EndLine < value.StartLine {
			value.EndLine = value.StartLine
		}
		size := len(value.Kind) + len(value.ID) + len(value.Label) + len(value.Detail) + 32
		if totalBytes+size > maxUserReferenceMetadataBytes {
			break
		}
		totalBytes += size
		result = append(result, value)
	}
	return result
}

// AppendContextMessage appends a model-visible message that is hidden from UI history.
func (s *Session) AppendContextMessage(msg *agent.Message) error {
	if msg == nil || (msg.Role == "" && strings.TrimSpace(msg.Content) == "" && len(msg.ToolCalls) == 0) {
		return nil
	}
	return s.withCanonicalMutation(context.Background(), "append context message", func() error {
		return s.appendMessageLocked(msg, MessageMetadata{}, historyTypeContextMessage)
	})
}

// AppendContextMessages atomically appends a model-visible, UI-hidden message
// batch. It is primarily used for protocol pairs that must never be durable in
// a half-written state.
func (s *Session) AppendContextMessages(messages ...*agent.Message) error {
	if len(messages) == 0 {
		return nil
	}
	metadata := make([]MessageMetadata, len(messages))
	return s.withCanonicalMutation(context.Background(), "append context message batch", func() error {
		return s.appendMessagesLocked(messages, metadata, historyTypeContextMessage)
	})
}

// AppendClearMarker 追加上下文清理标记，不删除历史消息。
func (s *Session) AppendClearMarker() error {
	return s.withCanonicalMutation(context.Background(), "append clear marker", s.appendClearMarkerLocked)
}

func (s *Session) appendClearMarkerLocked() error {
	now := time.Now().UTC()
	nextRevision := s.contextRevision + 1
	if err := s.appendJournalRecordLocked(clearRecord{Type: historyTypeClear, CreatedAt: now, ContextRevision: nextRevision}); err != nil {
		return err
	}
	s.contextRevision = nextRevision
	s.clearAfterIndex = s.messageCount
	s.records = append(s.records, historyRecord{kind: historyTypeClear, createdAt: now})
	advanceUpdatedAt(s, now)
	return nil
}

// GetMessages 返回所有消息的快照。
func (s *Session) GetMessages() []*agent.Message {
	s.mu.Lock()
	defer s.mu.Unlock()

	result := make([]*agent.Message, len(s.messages))
	copy(result, s.messages)
	return result
}

// MessageWindow returns the bounded resident raw transcript together with its
// absolute starting index and total durable message count.
func (s *Session) MessageWindow() ([]*agent.Message, int, int) {
	if s == nil {
		return nil, 0, 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]*agent.Message, len(s.messages))
	copy(result, s.messages)
	return result, s.messageBaseIndex, s.messageCount
}

// GetEffectiveMessages 返回最后一个清理标记之后的 Agent 有效上下文。
func (s *Session) GetEffectiveMessages() []*agent.Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.effectiveTranscriptMessagesLocked()
}

// MessageCountSinceClear returns the number of effective raw transcript
// messages after the latest clear marker.
func (s *Session) MessageCountSinceClear() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.messageCount - s.clearAfterIndex
}

// MessageCountTotal returns the raw persisted message count.
func (s *Session) MessageCountTotal() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.messageCount
}

// History returns the bounded resident UI window, including clear markers.
// Use ReadHistoryPage for UI pagination or ExportHistoryJSONL for a full scan.
func (s *Session) History() []HistoryEntry {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Root assistant display segments preserve the exact interleaving of prose,
	// reasoning, and tools. Suppress the canonical copy when either the terminal
	// segment or the complete segment sequence already carries the same content.
	type assistantDisplayCoverage struct {
		combined strings.Builder
		segments map[[sha256.Size]byte]struct{}
	}
	segmentedAssistantContentByRun := make(map[string]*assistantDisplayCoverage)
	for _, record := range s.records {
		if record.kind != historyTypeDisplay || record.display == nil || record.display.Role != "assistant" || record.display.SubAgent {
			continue
		}
		if runID := strings.TrimSpace(record.display.RunID); runID != "" {
			coverage := segmentedAssistantContentByRun[runID]
			if coverage == nil {
				coverage = &assistantDisplayCoverage{segments: make(map[[sha256.Size]byte]struct{})}
				segmentedAssistantContentByRun[runID] = coverage
			}
			coverage.combined.WriteString(record.display.Content)
			coverage.segments[sha256.Sum256([]byte(record.display.Content))] = struct{}{}
		}
	}

	result := make([]HistoryEntry, 0, len(s.records))
	for _, record := range s.records {
		switch record.kind {
		case historyTypeClear:
			result = append(result, HistoryEntry{Type: historyTypeClear, CreatedAt: record.createdAt})
		case historyTypeMessage:
			if record.message == nil {
				continue
			}
			if record.message.Role == agent.Assistant {
				if coverage := segmentedAssistantContentByRun[strings.TrimSpace(record.messageMetadata.RunID)]; coverage != nil {
					_, segmentMatches := coverage.segments[sha256.Sum256([]byte(record.message.Content))]
					if segmentMatches || coverage.combined.String() == record.message.Content {
						continue
					}
				}
			}
			result = append(result, HistoryEntry{
				Type:               historyTypeMessage,
				ID:                 record.messageMetadata.MessageID,
				Role:               string(record.message.Role),
				Content:            record.message.Content,
				Message:            record.message,
				CreatedAt:          record.createdAt,
				RunID:              record.messageMetadata.RunID,
				AgentKind:          record.messageMetadata.AgentKind,
				AgentName:          record.messageMetadata.AgentName,
				RootAgentName:      record.messageMetadata.RootAgentName,
				RunPath:            append([]string(nil), record.messageMetadata.RunPath...),
				SubAgent:           record.messageMetadata.SubAgent,
				SubAgentSessionID:  record.messageMetadata.SubAgentSessionID,
				SubAgentType:       record.messageMetadata.SubAgentType,
				UserReferences:     append([]agentcontext.UserReference(nil), record.messageMetadata.UserReferences...),
				AgentCommandID:     record.messageMetadata.AgentCommandID,
				AgentOperationID:   record.messageMetadata.AgentOperationID,
				AgentCycle:         record.messageMetadata.AgentCycle,
				DomainCommitHash:   record.messageMetadata.DomainCommitHash,
				AgentCanonicalHash: record.messageMetadata.AgentCanonicalHash,
				ContextRevision:    record.messageMetadata.ContextRevision,
			})
		case historyTypeDisplay:
			if record.display == nil {
				continue
			}
			result = append(result, HistoryEntry{
				Type:                 historyTypeMessage,
				ID:                   record.display.ID,
				DisplaySegmentID:     record.display.ID,
				DisplayPhase:         record.display.DisplayPhase,
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
				SSEHiddenFields:      append([]string(nil), record.display.SSEHiddenFields...),
				SSEHiddenReason:      record.display.SSEHiddenReason,
				SSEDisplayNotice:     record.display.SSEDisplayNotice,
				SSEGeneratedChars:    record.display.SSEGeneratedChars,
			})
		}
	}
	return normalizeCompletedToolDisplayEntries(result)
}

func normalizeCompletedToolDisplayEntries(entries []HistoryEntry) []HistoryEntry {
	pendingByRun := make(map[string][]int)
	for index := range entries {
		entry := entries[index]
		if entry.Role == "tool_call" && entry.Status == "running" && strings.TrimSpace(entry.RunID) != "" {
			pendingByRun[entry.RunID] = append(pendingByRun[entry.RunID], index)
			continue
		}
		if entry.Role != "token_usage" || strings.TrimSpace(entry.RunID) == "" {
			continue
		}
		for _, pendingIndex := range pendingByRun[entry.RunID] {
			if entries[pendingIndex].Status == "running" {
				entries[pendingIndex].Status = "success"
			}
		}
		delete(pendingByRun, entry.RunID)
	}
	return entries
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
	title = strings.TrimSpace(title)
	if title == "" {
		return fmt.Errorf("会话标题不能为空")
	}
	return s.withCanonicalMutation(context.Background(), "rename session", func() error {
		now := time.Now().UTC()
		if err := s.appendJournalRecordLocked(sessionPatchRecord{
			Type:      historyTypeSessionPatch,
			Title:     &title,
			UpdatedAt: now,
		}); err != nil {
			return err
		}
		s.title = title
		advanceUpdatedAt(s, now)
		return nil
	})
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
	return s.visibleMessageCountLocked()
}

func (s *Session) visibleMessageCountLocked() int {
	if s.projection != nil {
		return s.projection.VisibleMessageCount
	}
	count := 0
	for _, record := range s.records {
		if record.kind == historyTypeMessage {
			count++
		}
	}
	return count
}

func (s *Session) titleLocked() string {
	if strings.TrimSpace(s.title) != "" {
		return s.title
	}
	return defaultSessionTitle
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
