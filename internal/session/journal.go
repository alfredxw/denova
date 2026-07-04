package session

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cloudwego/eino/schema"
)

func (s *Session) persistLocked() error {
	header := sessionHeader{
		Type:      "session",
		ID:        s.ID,
		Title:     s.titleLocked(),
		CreatedAt: s.CreatedAt,
		UpdatedAt: s.UpdatedAt,
	}

	var sb strings.Builder
	if err := writeJSONLine(&sb, header); err != nil {
		return err
	}
	for _, record := range s.records {
		switch record.kind {
		case historyTypeClear:
			if err := writeJSONLine(&sb, clearRecord{Type: historyTypeClear, CreatedAt: record.createdAt}); err != nil {
				return err
			}
		case historyTypeInterrupt:
			if record.interruption == nil {
				continue
			}
			if err := writeJSONLine(&sb, interruptionRecord{Type: historyTypeInterrupt, Interruption: *record.interruption}); err != nil {
				return err
			}
		case historyTypeCompaction:
			if record.compaction == nil {
				continue
			}
			if err := writeJSONLine(&sb, *record.compaction); err != nil {
				return err
			}
		case historyTypeCompactionRemoved:
			if record.compactionRemoval == nil {
				continue
			}
			if err := writeJSONLine(&sb, *record.compactionRemoval); err != nil {
				return err
			}
		case historyTypeDisplay:
			if record.display == nil {
				continue
			}
			if err := writeJSONLine(&sb, displayRecord{Type: historyTypeDisplay, DisplayEvent: *record.display}); err != nil {
				return err
			}
		case historyTypeMessage:
			if record.message == nil {
				continue
			}
			message := messageRecord{Type: historyTypeMessage, CreatedAt: record.createdAt, Message: *record.message}
			if err := writeJSONLine(&sb, message); err != nil {
				return err
			}
		case historyTypeContextMessage:
			if record.message == nil {
				continue
			}
			message := messageRecord{Type: historyTypeContextMessage, CreatedAt: record.createdAt, Message: *record.message}
			if err := writeJSONLine(&sb, message); err != nil {
				return err
			}
		}
	}
	return os.WriteFile(s.filePath, []byte(sb.String()), 0o644)
}

// sessionHeader JSONL 文件首行的元数据。
type sessionHeader struct {
	Type      string    `json:"type"`
	ID        string    `json:"id"`
	Title     string    `json:"title,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at,omitempty"`
}

type clearRecord struct {
	Type      string    `json:"type"`
	CreatedAt time.Time `json:"created_at"`
}

type interruptionRecord struct {
	Type string `json:"type"`
	Interruption
}

type displayRecord struct {
	Type string `json:"type"`
	DisplayEvent
}

func createSession(id, filePath, title string) (*Session, error) {
	now := time.Now().UTC()
	if strings.TrimSpace(title) == "" {
		title = defaultSessionTitle
	}
	header := sessionHeader{
		Type:      "session",
		ID:        id,
		Title:     title,
		CreatedAt: now,
		UpdatedAt: now,
	}
	var sb strings.Builder
	if err := writeJSONLine(&sb, header); err != nil {
		return nil, err
	}
	if err := os.WriteFile(filePath, []byte(sb.String()), 0o644); err != nil {
		return nil, err
	}
	return &Session{
		ID:              id,
		CreatedAt:       now,
		UpdatedAt:       now,
		filePath:        filePath,
		title:           title,
		clearAfterIndex: 0,
		messages:        make([]*schema.Message, 0),
		records:         make([]historyRecord, 0),
	}, nil
}

func loadSession(filePath string) (*Session, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	if !scanner.Scan() {
		return nil, fmt.Errorf("会话文件为空: %s", filePath)
	}

	id := strings.TrimSuffix(filepath.Base(filePath), ".jsonl")
	now := time.Now().UTC()
	sess := &Session{
		ID:              id,
		CreatedAt:       now,
		UpdatedAt:       now,
		filePath:        filePath,
		title:           defaultSessionTitle,
		clearAfterIndex: 0,
		messages:        make([]*schema.Message, 0),
		records:         make([]historyRecord, 0),
	}

	firstLine := strings.TrimSpace(scanner.Text())
	var header sessionHeader
	if err := json.Unmarshal([]byte(firstLine), &header); err == nil && header.Type == "session" {
		sess.ID = firstNonEmpty(header.ID, id)
		sess.CreatedAt = header.CreatedAt
		if sess.CreatedAt.IsZero() {
			sess.CreatedAt = now
		}
		sess.UpdatedAt = header.UpdatedAt
		if sess.UpdatedAt.IsZero() {
			sess.UpdatedAt = sess.CreatedAt
		}
		if strings.TrimSpace(header.Title) != "" {
			sess.title = header.Title
		}
	} else if err := appendMessageLine(sess, firstLine); err != nil {
		return nil, fmt.Errorf("会话头部解析失败 %s: %w", filePath, err)
	}

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		_ = appendRecordLine(sess, line)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if sess.title == defaultSessionTitle {
		for _, msg := range sess.messages {
			if msg.Role == schema.User && strings.TrimSpace(msg.Content) != "" {
				sess.title = deriveTitle(msg.Content)
				break
			}
		}
	}
	if sess.UpdatedAt.IsZero() {
		sess.UpdatedAt = sess.CreatedAt
	}
	sess.trimTokenUsageDisplayEventsLocked("")
	return sess, nil
}

func appendRecordLine(sess *Session, line string) error {
	var typed struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal([]byte(line), &typed); err == nil && typed.Type == historyTypeClear {
		var marker clearRecord
		if err := json.Unmarshal([]byte(line), &marker); err != nil {
			return err
		}
		sess.clearAfterIndex = len(sess.messages)
		sess.records = append(sess.records, historyRecord{kind: historyTypeClear, createdAt: marker.CreatedAt})
		if marker.CreatedAt.After(sess.UpdatedAt) {
			sess.UpdatedAt = marker.CreatedAt
		}
		return nil
	}
	if typed.Type == historyTypeInterrupt {
		var marker interruptionRecord
		if err := json.Unmarshal([]byte(line), &marker); err != nil {
			return err
		}
		interruption := marker.Interruption
		if strings.TrimSpace(interruption.ID) == "" {
			interruption.ID = newInterruptionID()
		}
		if strings.TrimSpace(interruption.Status) == "" {
			interruption.Status = InterruptionPending
		}
		sess.records = append(sess.records, historyRecord{kind: historyTypeInterrupt, interruption: &interruption, createdAt: interruption.CreatedAt})
		if interruption.CreatedAt.After(sess.UpdatedAt) {
			sess.UpdatedAt = interruption.CreatedAt
		}
		return nil
	}
	if typed.Type == historyTypeCompaction {
		var record ContextCompaction
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			return err
		}
		if strings.TrimSpace(record.ID) == "" {
			record.ID = newContextCompactionID()
		}
		if record.CreatedAt.IsZero() {
			record.CreatedAt = sess.UpdatedAt
		}
		if record.Type == "" {
			record.Type = historyTypeCompaction
		}
		sess.records = append(sess.records, historyRecord{kind: historyTypeCompaction, compaction: &record, createdAt: record.CreatedAt})
		if record.CreatedAt.After(sess.UpdatedAt) {
			sess.UpdatedAt = record.CreatedAt
		}
		return nil
	}
	if typed.Type == historyTypeCompactionRemoved {
		var record ContextCompactionRemoval
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			return err
		}
		if strings.TrimSpace(record.ID) == "" {
			record.ID = newContextCompactionRemovalID()
		}
		if record.CreatedAt.IsZero() {
			record.CreatedAt = sess.UpdatedAt
		}
		if record.Type == "" {
			record.Type = historyTypeCompactionRemoved
		}
		sess.records = append(sess.records, historyRecord{kind: historyTypeCompactionRemoved, compactionRemoval: &record, createdAt: record.CreatedAt})
		if record.CreatedAt.After(sess.UpdatedAt) {
			sess.UpdatedAt = record.CreatedAt
		}
		return nil
	}
	if typed.Type == historyTypeDisplay {
		var marker displayRecord
		if err := json.Unmarshal([]byte(line), &marker); err != nil {
			return err
		}
		event := marker.DisplayEvent
		if event.CreatedAt.IsZero() {
			event.CreatedAt = sess.UpdatedAt
		}
		sess.records = append(sess.records, historyRecord{
			kind:                      historyTypeDisplay,
			display:                   &event,
			createdAt:                 event.CreatedAt,
			displayArgsPersistedBytes: len(event.Args),
		})
		if event.CreatedAt.After(sess.UpdatedAt) {
			sess.UpdatedAt = event.CreatedAt
		}
		return nil
	}
	if typed.Type == historyTypeMessage {
		return appendMessageRecordLine(sess, line, historyTypeMessage)
	}
	if typed.Type == historyTypeContextMessage {
		return appendMessageRecordLine(sess, line, historyTypeContextMessage)
	}
	return appendMessageLine(sess, line)
}

func appendMessageRecordLine(sess *Session, line string, kind string) error {
	var record messageRecord
	if err := json.Unmarshal([]byte(line), &record); err != nil {
		return err
	}
	if record.Message.Role == "" && record.Message.Content == "" && len(record.Message.ToolCalls) == 0 {
		return nil
	}
	createdAt := record.CreatedAt
	if createdAt.IsZero() {
		createdAt = nextLegacyMessageCreatedAt(sess)
	}
	msg := record.Message
	sess.messages = append(sess.messages, &msg)
	sess.records = append(sess.records, historyRecord{kind: kind, message: &msg, createdAt: createdAt})
	if createdAt.After(sess.UpdatedAt) {
		sess.UpdatedAt = createdAt
	}
	return nil
}

func appendMessageLine(sess *Session, line string) error {
	var msg schema.Message
	if err := json.Unmarshal([]byte(line), &msg); err != nil {
		return err
	}
	if msg.Role == "" && msg.Content == "" {
		return nil
	}
	createdAt := nextLegacyMessageCreatedAt(sess)
	sess.messages = append(sess.messages, &msg)
	sess.records = append(sess.records, historyRecord{kind: historyTypeMessage, message: &msg, createdAt: createdAt})
	if createdAt.After(sess.UpdatedAt) {
		sess.UpdatedAt = createdAt
	}
	return nil
}

func nextLegacyMessageCreatedAt(sess *Session) time.Time {
	base := sess.UpdatedAt
	if base.IsZero() {
		base = sess.CreatedAt
	}
	if base.IsZero() {
		base = time.Now().UTC()
	}
	return base.Add(time.Duration(len(sess.records)+1) * time.Millisecond)
}

func writeJSONLine(sb *strings.Builder, v interface{}) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	sb.Write(data)
	sb.WriteByte('\n')
	return nil
}
