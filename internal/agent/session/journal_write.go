package session

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	adk "github.com/alfredxw/denova/adk"
)

func createSession(id, filePath, title string) (*Session, error) {
	now := time.Now().UTC()
	incarnationID := newSessionJournalIncarnationID()
	if strings.TrimSpace(title) == "" {
		title = defaultSessionTitle
	}
	header := sessionHeader{
		Type:          "session",
		Version:       journalFormatVersion,
		ID:            id,
		IncarnationID: incarnationID,
		Title:         title,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	data, err := marshalJSONLine(header)
	if err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return nil, err
	}
	created := true
	defer func() {
		_ = f.Close()
		if created {
			_ = os.Remove(filePath)
		}
	}()
	if err := writeAndSync(f, data); err != nil {
		return nil, err
	}
	if err := f.Close(); err != nil {
		return nil, err
	}
	if err := syncParentDirectory(filePath); err != nil {
		return nil, fmt.Errorf("同步会话目录失败: %w", err)
	}
	created = false

	journalBytes := int64(len(data))
	return &Session{
		ID:                 id,
		CreatedAt:          now,
		UpdatedAt:          now,
		filePath:           filePath,
		title:              title,
		clearAfterIndex:    0,
		journalSize:        journalBytes,
		journalOffset:      journalBytes,
		journalIncarnation: incarnationID,
		journalLineCount:   1,
		lastReplayBytes:    journalBytes,
		lastReplayRecords:  1,
		messages:           make([]*adk.Message, 0),
		records:            make([]historyRecord, 0),
	}, nil
}

// appendJournalRecordLocked durably appends one record. Callers must already be
// inside withCanonicalMutation, which holds the file lease and session lock,
// and must not mutate materialized state until this returns nil.
func (s *Session) appendJournalRecordLocked(record any) error {
	data, err := marshalJSONLine(record)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(s.filePath, os.O_RDWR|os.O_APPEND, 0)
	if err != nil {
		return fmt.Errorf("打开会话 journal 失败: %w", err)
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return fmt.Errorf("读取会话 journal 状态失败: %w", err)
	}
	if !stat.Mode().IsRegular() {
		return fmt.Errorf("会话 journal 不是普通文件: %s", s.filePath)
	}
	if stat.Size() != s.journalSize {
		return fmt.Errorf("会话 journal 已被外部修改: expected_size=%d actual_size=%d", s.journalSize, stat.Size())
	}
	if s.journalOffset < 0 || s.journalOffset > s.journalSize {
		return fmt.Errorf("会话 journal 恢复偏移非法: offset=%d size=%d", s.journalOffset, s.journalSize)
	}

	validOffset := s.journalOffset
	var incompleteTail []byte
	if validOffset < s.journalSize {
		incompleteTail = make([]byte, s.journalSize-validOffset)
		if _, err := f.ReadAt(incompleteTail, validOffset); err != nil && err != io.EOF {
			return fmt.Errorf("读取会话 journal 不完整尾行失败: %w", err)
		}
		if err := preserveIncompleteTail(s.filePath, incompleteTail); err != nil {
			return err
		}
		if err := f.Truncate(validOffset); err != nil {
			return fmt.Errorf("截断会话 journal 不完整尾行失败: %w", err)
		}
	}

	payload := data
	if s.journalNeedsLF {
		payload = make([]byte, 0, len(data)+1)
		payload = append(payload, '\n')
		payload = append(payload, data...)
	}
	if err := writeAndSync(f, payload); err != nil {
		rollbackJournalTail(f, validOffset, incompleteTail)
		return fmt.Errorf("追加会话 journal 失败: %w", err)
	}

	newSize := validOffset + int64(len(payload))
	s.journalSize = newSize
	s.journalOffset = newSize
	s.journalNeedsLF = false
	s.journalLineCount++
	return nil
}

func writeAndSync(f *os.File, data []byte) error {
	written, err := f.Write(data)
	if err != nil {
		return err
	}
	if written != len(data) {
		return io.ErrShortWrite
	}
	return f.Sync()
}

func syncParentDirectory(filePath string) error {
	dir, err := os.Open(filepath.Dir(filePath))
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}
