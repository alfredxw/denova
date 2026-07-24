package session

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"

	agent "github.com/alfredxw/denova/agent"
)

// refreshCanonicalTailLocked applies only records appended since this Session
// last observed the journal. Callers hold both the cross-instance file lease
// and s.mu, so a stable size check provides the same canonical boundary as a
// full reload without making every domain commit O(total history).
func (s *Session) refreshCanonicalTailLocked() error {
	stat, err := os.Stat(s.filePath)
	if err != nil {
		return err
	}
	if !stat.Mode().IsRegular() {
		return fmt.Errorf("会话 journal 不是普通文件: %s", s.filePath)
	}
	incarnation, err := readSessionJournalIncarnation(s.filePath)
	if err != nil {
		return err
	}
	if incarnation != s.journalIncarnation {
		return fmt.Errorf("session journal incarnation changed: expected=%q actual=%q path=%s", s.journalIncarnation, incarnation, s.filePath)
	}
	if stat.Size() == s.journalSize && s.journalOffset == s.journalSize {
		s.lastReplayBytes = 0
		s.lastReplayRecords = 0
		return nil
	}
	if s.journalOffset <= 0 || stat.Size() < s.journalOffset {
		return s.reloadCanonicalLocked()
	}

	file, err := os.Open(s.filePath)
	if err != nil {
		return err
	}
	defer file.Close()
	if _, err := file.Seek(s.journalOffset, io.SeekStart); err != nil {
		return fmt.Errorf("定位会话 journal 增量失败: %w", err)
	}
	candidate := cloneSessionForTailReplay(s)
	reader := bufio.NewReader(file)
	startOffset := s.journalOffset
	readOffset := startOffset
	lineNumber := s.journalLineCount
	validLineCount := lineNumber
	replayedRecords := 0
	firstSegment := true
	for {
		lineStart := readOffset
		line, readErr := reader.ReadBytes('\n')
		if len(line) == 0 && readErr == io.EOF {
			break
		}
		readOffset += int64(len(line))
		newlineTerminated := len(line) > 0 && line[len(line)-1] == '\n'
		trimmed := bytes.TrimSpace(line)

		// A prior valid record may have ended exactly at EOF without a newline.
		// The append path inserts that separator before the next JSON record; it
		// completes the previous physical line and is not a new journal record.
		if firstSegment && s.journalNeedsLF && len(trimmed) == 0 && newlineTerminated {
			candidate.journalOffset = readOffset
			candidate.journalNeedsLF = false
			firstSegment = false
			if readErr == io.EOF {
				break
			}
			continue
		}
		firstSegment = false
		lineNumber++
		if len(trimmed) == 0 {
			candidate.journalOffset = readOffset
			candidate.journalNeedsLF = false
			validLineCount = lineNumber
		} else {
			parseErr := appendRecordLine(candidate, trimmed, lineNumber)
			if parseErr != nil {
				if readErr == io.EOF && !newlineTerminated && !json.Valid(trimmed) {
					candidate.journalOffset = lineStart
					break
				}
				return fmt.Errorf("解析会话 journal 增量 line %d: %w", lineNumber, parseErr)
			}
			candidate.journalOffset = readOffset
			candidate.journalNeedsLF = !newlineTerminated
			validLineCount = lineNumber
			replayedRecords++
		}
		if readErr != nil {
			if readErr == io.EOF {
				break
			}
			return fmt.Errorf("读取会话 journal 增量失败: %w", readErr)
		}
	}
	finalStat, err := file.Stat()
	if err != nil {
		return err
	}
	if finalStat.Size() != stat.Size() || readOffset != stat.Size() {
		return fmt.Errorf("会话 journal 在增量加载期间被修改: %s", s.filePath)
	}
	candidate.journalSize = stat.Size()
	candidate.journalLineCount = validLineCount
	candidate.lastReplayBytes = readOffset - startOffset
	candidate.lastReplayRecords = replayedRecords
	s.replaceCanonicalStateLocked(candidate)
	return nil
}

func (s *Session) reloadCanonicalLocked() error {
	recovered, err := loadSession(s.filePath)
	if err != nil {
		return err
	}
	if recovered.journalIncarnation != s.journalIncarnation {
		return fmt.Errorf("session journal incarnation changed during reload: expected=%q actual=%q path=%s", s.journalIncarnation, recovered.journalIncarnation, s.filePath)
	}
	s.replaceCanonicalStateLocked(recovered)
	return nil
}

func cloneSessionForTailReplay(source *Session) *Session {
	result := &Session{
		ID: source.ID, CreatedAt: source.CreatedAt, UpdatedAt: source.UpdatedAt,
		filePath: source.filePath, title: source.title,
		clearAfterIndex: source.clearAfterIndex, contextRevision: source.contextRevision,
		journalSize: source.journalSize, journalOffset: source.journalOffset,
		journalIncarnation: source.journalIncarnation,
		journalNeedsLF:     source.journalNeedsLF, journalLineCount: source.journalLineCount,
		messages: append([]*agent.Message(nil), source.messages...),
		records:  make([]historyRecord, len(source.records)),
	}
	for index, record := range source.records {
		result.records[index] = cloneHistoryRecordForTailReplay(record)
	}
	return result
}

func cloneHistoryRecordForTailReplay(record historyRecord) historyRecord {
	clone := record
	clone.messageMetadata.RunPath = append([]string(nil), record.messageMetadata.RunPath...)
	clone.messageMetadata.UserReferences = append([]UserMessageReference(nil), record.messageMetadata.UserReferences...)
	if record.display != nil {
		display := *record.display
		display.Illustration = cloneChapterIllustration(record.display.Illustration)
		display.RunPath = append([]string(nil), record.display.RunPath...)
		display.UsageCalls = append([]TokenUsageCall(nil), record.display.UsageCalls...)
		for index := range display.UsageCalls {
			display.UsageCalls[index].RequestedTools = append([]string(nil), record.display.UsageCalls[index].RequestedTools...)
			display.UsageCalls[index].AfterTools = append([]string(nil), record.display.UsageCalls[index].AfterTools...)
		}
		display.SSEHiddenFields = append([]string(nil), record.display.SSEHiddenFields...)
		clone.display = &display
	}
	if record.interruption != nil {
		interruption := *record.interruption
		if record.interruption.ResolvedAt != nil {
			resolvedAt := *record.interruption.ResolvedAt
			interruption.ResolvedAt = &resolvedAt
		}
		clone.interruption = &interruption
	}
	if record.compaction != nil {
		compaction := *record.compaction
		clone.compaction = &compaction
	}
	if record.compactionRemoval != nil {
		removal := *record.compactionRemoval
		clone.compactionRemoval = &removal
	}
	return clone
}
