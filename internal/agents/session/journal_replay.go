package session

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	agent "github.com/alfredxw/denova/agent"

	"denova/internal/conversationjournal"
)

func loadSession(filePath string) (*Session, error) {
	stat, err := os.Stat(filePath)
	if err != nil {
		return nil, err
	}
	if !stat.Mode().IsRegular() {
		return nil, fmt.Errorf("会话 journal 不是普通文件: %s", filePath)
	}

	id := strings.TrimSuffix(filepath.Base(filePath), ".jsonl")
	generation, err := readSessionJournalIncarnation(filePath)
	if err != nil {
		return nil, err
	}
	projection := newSessionJournalProjection(id, generation)
	journal, err := conversationjournal.Open(
		context.Background(),
		filePath,
		conversationjournal.Identity{ID: id, Generation: generation},
		projection,
		conversationjournal.Options{},
	)
	if err != nil {
		return nil, fmt.Errorf("打开会话 journal 失败 %s: %w", filePath, err)
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = journal.Close()
		}
	}()

	now := time.Now().UTC()
	createdAt := projection.CreatedAt
	if createdAt.IsZero() {
		createdAt = now
	}
	updatedAt := projection.UpdatedAt
	if updatedAt.IsZero() {
		updatedAt = createdAt
	}
	startCursor := projection.recentStartCursor()
	messageBase := projection.messageBaseForCursor(startCursor)
	sess := &Session{
		ID: id, CreatedAt: createdAt, UpdatedAt: updatedAt,
		filePath: filePath, title: projection.Title,
		clearAfterIndex: projection.ClearAfter, contextRevision: projection.ContextRevision,
		journalSize: stat.Size(), journalOffset: journal.Head().VerifiedBytes,
		journalIncarnation: generation, journalLineCount: int(journal.Head().Cursor),
		journal: journal, projection: projection,
		messageBaseIndex: messageBase, messageCount: messageBase,
		messages: make([]*agent.Message, 0), records: make([]historyRecord, 0),
		partialMaterialization: true,
	}
	if err := sess.migrateProjectionCompactionCursorsLocked(context.Background()); err != nil {
		return nil, fmt.Errorf("迁移会话压缩游标 %s: %w", filePath, err)
	}
	for _, structural := range projection.Structural {
		if structural.Cursor >= startCursor {
			continue
		}
		if structural.Compaction != nil {
			value := *structural.Compaction
			sess.records = append(sess.records, historyRecord{kind: historyTypeCompaction, compaction: &value, createdAt: value.CreatedAt})
		}
		if structural.Removal != nil {
			value := *structural.Removal
			sess.records = append(sess.records, historyRecord{kind: historyTypeCompactionRemoved, compactionRemoval: &value, createdAt: value.CreatedAt})
		}
	}
	if projection.PendingInterrupt != nil && projection.PendingInterruptCursor < startCursor {
		pendingRecords, readErr := journal.ReadRange(context.Background(), conversationjournal.Range{
			After: projection.PendingInterruptCursor - 1, Through: projection.PendingInterruptCursor,
		})
		if readErr != nil {
			return nil, fmt.Errorf("读取待恢复中断失败 %s: %w", filePath, readErr)
		}
		for _, record := range pendingRecords {
			if err := appendConversationRecord(sess, record); err != nil {
				return nil, fmt.Errorf("恢复待处理中断 %s: %w", filePath, err)
			}
		}
	}
	if projection.PendingAsk != nil && projection.PendingAskCursor < startCursor {
		pendingRecords, readErr := journal.ReadRange(context.Background(), conversationjournal.Range{
			After: projection.PendingAskCursor - 1, Through: projection.PendingAskCursor,
		})
		if readErr != nil {
			return nil, fmt.Errorf("read pending ask %s: %w", filePath, readErr)
		}
		for _, record := range pendingRecords {
			if err := appendConversationRecord(sess, record); err != nil {
				return nil, fmt.Errorf("restore pending ask %s: %w", filePath, err)
			}
		}
	}
	records, err := journal.ReadRange(context.Background(), conversationjournal.Range{After: startCursor - 1})
	if err != nil {
		return nil, fmt.Errorf("读取会话最近窗口失败 %s: %w", filePath, err)
	}
	for _, record := range records {
		if err := appendConversationRecord(sess, record); err != nil {
			return nil, fmt.Errorf("恢复会话最近窗口 %s cursor %d: %w", filePath, record.Location.Cursor, err)
		}
		sess.materializedCursor = record.Location.Cursor
	}
	sess.partialMaterialization = false
	sess.messageCount = projection.MessageCount
	sess.clearAfterIndex = projection.ClearAfter
	sess.contextRevision = projection.ContextRevision
	sess.title = projection.Title
	sess.CreatedAt = createdAt
	sess.UpdatedAt = updatedAt
	sess.journalOffset = journal.Head().VerifiedBytes
	sess.journalLineCount = int(journal.Head().Cursor)
	stats := journal.ReplayStats()
	sess.lastReplayBytes = stats.BytesRead
	sess.lastReplayRecords = int(stats.TransactionsRead)
	sess.trimTokenUsageDisplayEventsLocked("")
	cleanup = false
	return sess, nil
}

func appendConversationRecord(sess *Session, record conversationjournal.Record) error {
	if record.Location.Cursor == 1 {
		return appendFirstRecordLine(sess, record.Payload)
	}
	return appendRecordLine(sess, record.Payload, int(record.Location.Cursor))
}

func appendFirstRecordLine(sess *Session, line []byte) error {
	var typed struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(line, &typed); err != nil {
		return err
	}
	if typed.Type == "session" {
		var header sessionHeader
		if err := json.Unmarshal(line, &header); err != nil {
			return err
		}
		fallbackID := sess.ID
		header.ID = firstNonEmpty(header.ID, fallbackID)
		sess.ID = header.ID
		journalIncarnation := sessionHeaderIncarnation(header)
		sess.CreatedAt = header.CreatedAt
		if sess.CreatedAt.IsZero() {
			sess.CreatedAt = time.Now().UTC()
		}
		sess.UpdatedAt = header.UpdatedAt
		if sess.UpdatedAt.IsZero() {
			sess.UpdatedAt = sess.CreatedAt
		}
		if strings.TrimSpace(header.Title) != "" {
			sess.title = header.Title
		}
		sess.journalIncarnation = journalIncarnation
		return nil
	}
	if typed.Type != "" {
		return fmt.Errorf("首条记录必须是 session header 或旧格式消息，实际 type=%q", typed.Type)
	}
	sess.journalIncarnation = legacyMessageJournalIncarnation(sess.ID)
	return appendLegacyMessageLine(sess, line)
}

func appendRecordLine(sess *Session, line []byte, lineNumber int) error {
	var typed struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(line, &typed); err != nil {
		return err
	}
	switch typed.Type {
	case historyTypeClear:
		return appendClearRecordLine(sess, line)
	case historyTypeInterrupt:
		return appendInterruptionRecordLine(sess, line, lineNumber)
	case historyTypeAsk:
		return appendAskRecordLine(sess, line)
	case historyTypeCompaction:
		return appendCompactionRecordLine(sess, line, lineNumber)
	case historyTypeCompactionRemoved:
		return appendCompactionRemovalRecordLine(sess, line, lineNumber)
	case historyTypeDisplay:
		return appendDisplayRecordLine(sess, line, lineNumber)
	case historyTypeMessage, historyTypeContextMessage:
		return appendMessageRecordLine(sess, line, typed.Type)
	case historyTypeSessionPatch:
		return applySessionPatchLine(sess, line)
	case historyTypeDisplayPatch:
		return applyDisplayPatchLine(sess, line)
	case historyTypeInterruptionPatch:
		return applyInterruptionPatchLine(sess, line)
	case historyTypeAskPatch:
		return applyAskPatchLine(sess, line)
	case "":
		return appendLegacyMessageLine(sess, line)
	case "session":
		return fmt.Errorf("session header 只能出现在首条记录")
	default:
		return fmt.Errorf("未知 journal record type: %q", typed.Type)
	}
}
