package interactive

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"denova/internal/agents/conversationjournal"
	"denova/internal/localfs"
)

func (s *Store) openStoryJournalLocked(storyID string) (*storyJournalHandle, error) {
	storyID = strings.TrimSpace(storyID)
	if s.storyJournals != nil {
		if handle := s.storyJournals[storyID]; handle != nil {
			return handle, nil
		}
	}
	meta, err := readStoryJournalHeader(s.storyPath(storyID))
	if err != nil {
		return nil, err
	}
	generation := storyJournalGeneration(meta)
	projection := newStoryJournalProjection(storyID, generation)
	journal, err := conversationjournal.Open(
		context.Background(), s.storyPath(storyID),
		conversationjournal.Identity{ID: storyID, Generation: generation}, projection,
		conversationjournal.Options{},
	)
	if err != nil {
		return nil, err
	}
	handle := &storyJournalHandle{journal: journal, projection: projection, recent: make(map[string]storyRecentCache)}
	if s.storyJournals == nil {
		s.storyJournals = make(map[string]*storyJournalHandle)
	}
	s.storyJournals[storyID] = handle
	return handle, nil
}

func readStoryJournalHeader(path string) (StoryMeta, error) {
	file, err := os.Open(path)
	if err != nil {
		return StoryMeta{}, err
	}
	defer file.Close()
	reader := bufio.NewReaderSize(file, 64*1024)
	line, readErr := reader.ReadBytes('\n')
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		return StoryMeta{}, readErr
	}
	line = trimJSONLRecord(line)
	if len(line) == 0 {
		return StoryMeta{}, fmt.Errorf("故事文件为空: %s", path)
	}
	var meta StoryMeta
	if err := json.Unmarshal(line, &meta); err != nil {
		return StoryMeta{}, fmt.Errorf("解析故事元信息失败: %w", err)
	}
	meta = normalizeStoryMeta(meta)
	if err := validateStoryMeta(meta); err != nil {
		return StoryMeta{}, fmt.Errorf("校验故事元信息失败: %w", err)
	}
	return meta, nil
}

func (s *Store) readStoryConversationJournalLocked(storyID string, repairTornTail bool) (StoryMeta, []StoryEventRecord, error) {
	handle, err := s.refreshStoryJournalLocked(storyID, repairTornTail)
	if err != nil {
		return StoryMeta{}, nil, err
	}
	records, err := handle.journal.ReadRange(context.Background(), conversationjournal.Range{})
	if err != nil {
		// A maintenance rewrite may replace the canonical file behind a cached
		// handle. Reopen once; incarnation validation remains authoritative.
		delete(s.storyJournals, storyID)
		handle, err = s.openStoryJournalLocked(storyID)
		if err != nil {
			return StoryMeta{}, nil, err
		}
		records, err = handle.journal.ReadRange(context.Background(), conversationjournal.Range{})
		if err != nil {
			return StoryMeta{}, nil, err
		}
	}
	meta := StoryMeta{}
	events := make([]StoryEventRecord, 0)
	stats := StoryJournalReplayStats{BytesRead: handle.journal.ReplayStats().LastRangeBytesRead}
	seenPhysical := make(map[conversationjournal.Cursor]bool)
	seenCommonTransaction := make(map[conversationjournal.Cursor]bool)
	for _, record := range records {
		if !seenPhysical[record.Location.Cursor] {
			seenPhysical[record.Location.Cursor] = true
			stats.RecordsRead++
		}
		decodedMeta, decodedEvents, legacyTransaction, decodeErr := decodeStoryProjectionPayload(record.Payload)
		if decodeErr != nil {
			return StoryMeta{}, nil, fmt.Errorf("解析故事事件失败 (cursor %d): %w", record.Location.Cursor, decodeErr)
		}
		if legacyTransaction {
			stats.TransactionsRead++
		} else if !record.Legacy && !seenCommonTransaction[record.Location.Cursor] {
			seenCommonTransaction[record.Location.Cursor] = true
			stats.TransactionsRead++
		}
		if decodedMeta.StoryID != "" {
			meta = decodedMeta
		}
		events = append(events, decodedEvents...)
		stats.EventsRead += int64(len(decodedEvents))
	}
	if meta.StoryID == "" {
		meta = handle.projection.Meta
	}
	meta = normalizeStoryMeta(meta)
	if err := validateStoryMeta(meta); err != nil {
		return StoryMeta{}, nil, err
	}
	s.rememberStoryReplayStats(storyID, stats)
	return meta, events, nil
}

func (s *Store) refreshStoryJournalLocked(storyID string, repairTornTail bool) (*storyJournalHandle, error) {
	handle, err := s.openStoryJournalLocked(storyID)
	if err != nil {
		return nil, err
	}
	// Refresh first so a complete transaction appended through another handle
	// is distinguished from an incomplete physical tail.
	if _, refreshErr := handle.journal.ReadRange(context.Background(), conversationjournal.Range{After: handle.journal.Head().Cursor}); refreshErr != nil {
		delete(s.storyJournals, storyID)
		handle, err = s.openStoryJournalLocked(storyID)
		if err != nil {
			return nil, err
		}
	}
	path := s.storyPath(storyID)
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	head := handle.journal.Head()
	if info.Size() <= head.VerifiedBytes {
		return handle, nil
	}
	if !repairTornTail {
		return nil, fmt.Errorf("故事 journal 存在不完整尾行: verified=%d actual=%d", head.VerifiedBytes, info.Size())
	}
	if err := backupStoryBeforeTailRepair(path); err != nil {
		return nil, err
	}
	if err := handle.journal.RepairTail(context.Background()); err != nil {
		return nil, err
	}
	return handle, nil
}

func backupStoryBeforeTailRepair(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	backupPath := fmt.Sprintf("%s.recovery.%d.%d.bak", path, time.Now().UTC().UnixNano(), os.Getpid())
	file, err := os.OpenFile(backupPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	writeErr := writeAllStoryBytes(file, data)
	syncErr := file.Sync()
	closeErr := file.Close()
	if writeErr != nil || syncErr != nil || closeErr != nil {
		return errors.Join(writeErr, syncErr, closeErr)
	}
	return localfs.SyncDirectory(filepath.Dir(path))
}
