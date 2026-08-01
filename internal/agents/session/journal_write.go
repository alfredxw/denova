package session

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"denova/internal/agents/conversationconfig"
	"denova/internal/agents/conversationjournal"
	"denova/internal/localfs"
)

func createSession(id, filePath, title string) (*Session, error) {
	return createSessionWithRuntimeConfig(id, filePath, title, nil)
}

func createSessionWithRuntimeConfig(id, filePath, title string, runtimeConfig *conversationconfig.Config) (*Session, error) {
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
	if runtimeConfig != nil {
		value := *runtimeConfig
		header.RuntimeConfig = &value
		header.RuntimeConfigRevision = 1
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

	return loadSession(filePath)
}

// appendJournalRecordLocked durably appends one domain record through the
// shared conversation journal. Callers hold s.mu and update their in-memory
// materialization only after this returns successfully.
func (s *Session) appendJournalRecordLocked(record any) error {
	_, err := s.appendJournalRecordsLocked(record)
	return err
}

// appendJournalRecordsLocked publishes one canonical transaction and returns
// exact record locations for references stored by later domain records.
func (s *Session) appendJournalRecordsLocked(records ...any) (conversationjournal.Commit, error) {
	if s.journal == nil {
		return conversationjournal.Commit{}, fmt.Errorf("会话 journal 未打开")
	}
	payloads := make([]json.RawMessage, len(records))
	for index, record := range records {
		data, err := json.Marshal(record)
		if err != nil {
			return conversationjournal.Commit{}, err
		}
		payloads[index] = data
	}
	head := s.journal.Head()
	commit, err := s.journal.Append(context.Background(), conversationjournal.Guard{Cursor: s.materializedCursor, RecordSHA256: head.RecordSHA256}, payloads...)
	if err != nil {
		return conversationjournal.Commit{}, err
	}
	s.materializedCursor = commit.Head.Cursor
	s.journalSize = commit.Head.VerifiedBytes
	s.journalOffset = commit.Head.VerifiedBytes
	s.journalNeedsLF = false
	s.journalLineCount = int(commit.Head.Cursor)
	return commit, nil
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
	return localfs.SyncDirectory(filepath.Dir(filePath))
}
