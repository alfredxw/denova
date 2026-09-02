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

	"denova/internal/agents/conversationjournal"
	"denova/internal/agents/sessionjournal"
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
	priorMessageTransactions := projection.messageTransactionsBefore(startCursor)
	messageBase := projection.messageBaseForCursor(startCursor)
	if len(priorMessageTransactions) > 0 {
		messageBase = priorMessageTransactions[0].Index
	}
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
	// The mixed-event window is bounded by physical transactions, while the
	// canonical transcript is bounded by logical messages. Read older retained
	// message transactions directly so a tool-heavy turn cannot evict its user
	// input merely because it produced many display updates.
	for _, locator := range priorMessageTransactions {
		messageRecords, readErr := journal.ReadRange(context.Background(), conversationjournal.Range{
			After: locator.Cursor - 1, Through: locator.Cursor,
		})
		if readErr != nil {
			return nil, fmt.Errorf("read session canonical message transaction %s cursor %d: %w", filePath, locator.Cursor, readErr)
		}
		if len(messageRecords) != 1 || messageRecords[0].Location.Cursor != locator.Cursor {
			return nil, fmt.Errorf("session canonical message transaction missing %s cursor %d", filePath, locator.Cursor)
		}
		if err := appendConversationRecord(sess, messageRecords[0]); err != nil {
			return nil, fmt.Errorf("restore session canonical message transaction %s cursor %d: %w", filePath, locator.Cursor, err)
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
	// The projection already contains the latest durable runtime config, while
	// the bounded materialization above can include one or more of the same
	// session_patch records. Seed the materialization from zero, replay its
	// local sequence, then install the authoritative projected snapshot. This
	// keeps legacy sessions (whose header has no config) restart-safe as well as
	// sessions whose recent window starts after an older config revision.
	sess.runtimeConfig = nil
	sess.runtimeConfigRevision = projection.RuntimeConfigRevision
	if projection.RuntimeConfig != nil {
		value := *projection.RuntimeConfig
		sess.runtimeConfig = &value
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
	// A physical transaction may contain many logical context messages. Bound
	// the resident logical window after replay just as the hot append path does.
	sess.trimMaterializedWindowLocked()
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
		if header.RuntimeConfig != nil {
			if header.RuntimeConfigRevision != 1 {
				return fmt.Errorf("session header runtime config revision must be 1")
			}
			if err := validateRuntimeConfigState(header.RuntimeConfig, header.RuntimeConfigRevision, ""); err != nil {
				return fmt.Errorf("session header runtime config: %w", err)
			}
			value := *header.RuntimeConfig
			sess.runtimeConfig = &value
			sess.runtimeConfigRevision = header.RuntimeConfigRevision
		} else if header.RuntimeConfigRevision != 0 {
			return fmt.Errorf("session header runtime config revision exists without a config")
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
	case historyTypeDisplay:
		return appendDisplayRecordLine(sess, line, lineNumber)
	case historyTypeMessage, historyTypeContextMessage:
		return appendMessageRecordLine(sess, line, typed.Type)
	case historyTypeContextBatch:
		return appendContextBatchRecordLine(sess, line)
	case historyTypeSessionPatch:
		return applySessionPatchLine(sess, line)
	case historyTypeDisplayPatch:
		return applyDisplayPatchLine(sess, line)
	case historyTypeInterruptionPatch:
		return applyInterruptionPatchLine(sess, line)
	case sessionjournal.RecordType:
		return nil
	case "":
		return appendLegacyMessageLine(sess, line)
	case "session":
		return fmt.Errorf("session header 只能出现在首条记录")
	default:
		if isRetiredSessionJournalRecordType(typed.Type) {
			return nil
		}
		return fmt.Errorf("未知 journal record type: %q", typed.Type)
	}
}
