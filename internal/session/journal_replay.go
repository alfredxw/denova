package session

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cloudwego/eino/schema"
)

func loadSession(filePath string) (*Session, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	stat, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !stat.Mode().IsRegular() {
		return nil, fmt.Errorf("会话 journal 不是普通文件: %s", filePath)
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
		journalSize:     stat.Size(),
		messages:        make([]*schema.Message, 0),
		records:         make([]historyRecord, 0),
	}

	reader := bufio.NewReader(f)
	var readOffset int64
	lineNumber := 0
	validLineCount := 0
	sawRecord := false
	for {
		lineStart := readOffset
		line, readErr := reader.ReadBytes('\n')
		if len(line) == 0 && readErr == io.EOF {
			break
		}
		lineNumber++
		readOffset += int64(len(line))
		newlineTerminated := len(line) > 0 && line[len(line)-1] == '\n'
		trimmed := bytes.TrimSpace(line)

		if len(trimmed) == 0 {
			sess.journalOffset = readOffset
			sess.journalNeedsLF = false
			validLineCount = lineNumber
		} else {
			var parseErr error
			if !sawRecord {
				parseErr = appendFirstRecordLine(sess, trimmed)
			} else {
				parseErr = appendRecordLine(sess, trimmed, lineNumber)
			}
			if parseErr != nil {
				if readErr == io.EOF && !newlineTerminated && sawRecord && !json.Valid(trimmed) {
					// Only an unterminated malformed final line is recoverable.
					sess.journalOffset = lineStart
					break
				}
				return nil, fmt.Errorf("会话 journal 损坏 %s line %d: %w", filePath, lineNumber, parseErr)
			}
			sawRecord = true
			sess.journalOffset = readOffset
			sess.journalNeedsLF = !newlineTerminated
			validLineCount = lineNumber
		}

		if readErr != nil {
			if readErr == io.EOF {
				break
			}
			return nil, fmt.Errorf("读取会话 journal 失败 %s line %d: %w", filePath, lineNumber, readErr)
		}
	}
	if !sawRecord {
		return nil, fmt.Errorf("会话文件为空或没有有效记录: %s", filePath)
	}
	finalStat, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if finalStat.Size() != stat.Size() || readOffset != stat.Size() {
		return nil, fmt.Errorf("会话 journal 在加载期间被修改: %s", filePath)
	}
	sess.journalLineCount = validLineCount
	sess.lastReplayBytes = sess.journalOffset
	sess.lastReplayRecords = validLineCount

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
	case "":
		return appendLegacyMessageLine(sess, line)
	case "session":
		return fmt.Errorf("session header 只能出现在首条记录")
	default:
		return fmt.Errorf("未知 journal record type: %q", typed.Type)
	}
}
