package interactive

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func (s *Store) storyDir() string {
	return filepath.Join(s.root, "interactive", "story")
}

func (s *Store) indexPath() string {
	return filepath.Join(s.storyDir(), "index.json")
}

func (s *Store) storyPath(storyID string) string {
	return filepath.Join(s.storyDir(), "story-"+storyID+".jsonl")
}

func (s *Store) actorStateSchemaPath(storyID string) string {
	return filepath.Join(s.root, "interactive", "story-schema", "story-"+storyID+"-actor-state.json")
}

func (s *Store) usageDir() string {
	return filepath.Join(s.root, "interactive", "usage")
}

func (s *Store) usagePath(storyID string) string {
	return filepath.Join(s.usageDir(), "usage-"+storyID+".jsonl")
}

func (s *Store) readIndexLocked() (Index, error) {
	data, err := os.ReadFile(s.indexPath())
	if os.IsNotExist(err) {
		return Index{Stories: []StorySummary{}}, nil
	}
	if err != nil {
		return Index{}, err
	}
	var index Index
	if err := json.Unmarshal(data, &index); err != nil {
		return Index{}, fmt.Errorf("解析互动故事索引失败: %w", err)
	}
	for i := range index.Stories {
		index.Stories[i] = normalizeStorySummary(index.Stories[i])
	}
	return index, nil
}

func (s *Store) writeIndexLocked(index Index) error {
	data, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomicBytes(s.indexPath(), append(data, '\n'), 0o644)
}

func (s *Store) touchIndexLocked(storyID, updatedAt string, eventDelta int) error {
	index, err := s.readIndexLocked()
	if err != nil {
		return err
	}
	for i := range index.Stories {
		if index.Stories[i].ID == storyID {
			index.Stories[i].UpdatedAt = updatedAt
			index.Stories[i].Events += eventDelta
			return s.writeIndexLocked(index)
		}
	}
	return fmt.Errorf("故事不存在: %s", storyID)
}

func (s *Store) updateIndexBranchesLocked(storyID string, branches int, updatedAt string, eventDelta int) error {
	index, err := s.readIndexLocked()
	if err != nil {
		return err
	}
	for i := range index.Stories {
		if index.Stories[i].ID == storyID {
			index.Stories[i].Branches = branches
			index.Stories[i].UpdatedAt = updatedAt
			index.Stories[i].Events += eventDelta
			return s.writeIndexLocked(index)
		}
	}
	return fmt.Errorf("故事不存在: %s", storyID)
}

func (s *Store) readStoryLocked(storyID string) (StoryMeta, []StoryEventRecord, error) {
	release, err := s.acquireStoryReadLeaseLocked(storyID)
	if err != nil {
		return StoryMeta{}, nil, err
	}
	defer release()
	meta, lines, err := s.readStoryJournalWithRepairLocked(storyID, true)
	if err != nil {
		return StoryMeta{}, nil, err
	}
	if err := s.freezeLegacyActorStateSchemaLocked(storyID, &meta, lines); err != nil {
		return StoryMeta{}, nil, err
	}
	// A legacy sidecar may carry a revision that was not available while the
	// JSONL metadata was normalized. Keep the fixed status aligned with the
	// actual frozen schema without changing the schema itself.
	normalizeFixedStoryStateSchemaInitialization(&meta)
	return meta, lines, nil
}

// readStoryJournalLocked is the read-only half of story loading. Receipt
// reconciliation uses it so opening an unfinished Agent operation cannot
// trigger legacy migrations or any other canonical write.
func (s *Store) readStoryJournalLocked(storyID string) (StoryMeta, []StoryEventRecord, error) {
	release, err := s.acquireStoryReadLeaseLocked(storyID)
	if err != nil {
		return StoryMeta{}, nil, err
	}
	defer release()
	return s.readStoryJournalWithRepairLocked(storyID, false)
}

func (s *Store) readStoryJournalWithRepairLocked(storyID string, repairTornTail bool) (StoryMeta, []StoryEventRecord, error) {
	file, err := os.Open(s.storyPath(storyID))
	if err != nil {
		return StoryMeta{}, nil, err
	}
	reader := bufio.NewReaderSize(file, 64*1024)
	var stats StoryJournalReplayStats
	first, readErr := reader.ReadBytes('\n')
	stats.BytesRead += int64(len(first))
	if len(first) == 0 {
		_ = file.Close()
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			return StoryMeta{}, nil, readErr
		}
		return StoryMeta{}, nil, fmt.Errorf("故事文件为空: %s", storyID)
	}
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		_ = file.Close()
		return StoryMeta{}, nil, readErr
	}
	first = trimJSONLRecord(first)
	if len(first) == 0 {
		_ = file.Close()
		return StoryMeta{}, nil, fmt.Errorf("故事元信息为空: %s", storyID)
	}
	var meta StoryMeta
	if err := json.Unmarshal(first, &meta); err != nil {
		_ = file.Close()
		return StoryMeta{}, nil, fmt.Errorf("解析故事元信息失败: %w", err)
	}
	meta = normalizeStoryMeta(meta)
	if err := validateStoryMeta(meta); err != nil {
		_ = file.Close()
		return StoryMeta{}, nil, fmt.Errorf("校验故事元信息失败: %w", err)
	}
	canonicalStoryID := meta.StoryID
	stats.RecordsRead = 1
	validBytes := stats.BytesRead
	var lines []StoryEventRecord
	lineNumber := 1
	for {
		recordBytes, recordErr := reader.ReadBytes('\n')
		stats.BytesRead += int64(len(recordBytes))
		if len(recordBytes) == 0 && errors.Is(recordErr, io.EOF) {
			break
		}
		lineNumber++
		hasNewline := len(recordBytes) > 0 && recordBytes[len(recordBytes)-1] == '\n'
		line := trimJSONLRecord(recordBytes)
		if len(line) == 0 {
			_ = file.Close()
			return StoryMeta{}, nil, fmt.Errorf("解析故事事件失败: line %d is empty", lineNumber)
		}
		transactionMeta, transactionEvents, isTransaction, decodeErr := decodeStoryAppendTransaction(line)
		if decodeErr != nil {
			if repairTornTail && errors.Is(recordErr, io.EOF) && !hasNewline && isTornStoryJSON(line, decodeErr) {
				if closeErr := file.Close(); closeErr != nil {
					return StoryMeta{}, nil, closeErr
				}
				file = nil
				if repairErr := repairTornStoryTail(s.storyPath(storyID), validBytes); repairErr != nil {
					return StoryMeta{}, nil, fmt.Errorf("备份并修复故事日志尾部失败: %w", repairErr)
				}
				stats.BytesRead = validBytes
				break
			}
			_ = file.Close()
			return StoryMeta{}, nil, fmt.Errorf("解析故事事件失败 (line %d): %w", lineNumber, decodeErr)
		}
		if isTransaction {
			if transactionMeta.StoryID != canonicalStoryID {
				_ = file.Close()
				return StoryMeta{}, nil, fmt.Errorf("故事追加事务 identity 不匹配: have %s want %s", transactionMeta.StoryID, canonicalStoryID)
			}
			meta = transactionMeta
			lines = append(lines, transactionEvents...)
			stats.TransactionsRead++
			stats.EventsRead += int64(len(transactionEvents))
		} else {
			record, err := decodeStoryEventRecord(line)
			if err != nil {
				_ = file.Close()
				return StoryMeta{}, nil, fmt.Errorf("解析故事事件失败 (line %d): %w", lineNumber, err)
			}
			lines = append(lines, record)
			stats.EventsRead++
		}
		stats.RecordsRead++
		validBytes += int64(len(recordBytes))
		if recordErr != nil && !errors.Is(recordErr, io.EOF) {
			_ = file.Close()
			return StoryMeta{}, nil, recordErr
		}
		if errors.Is(recordErr, io.EOF) {
			break
		}
	}
	if file != nil {
		if err := file.Close(); err != nil {
			return StoryMeta{}, nil, err
		}
	}
	s.rememberStoryReplayStats(storyID, stats)
	return meta, lines, nil
}

func trimJSONLRecord(record []byte) []byte {
	if len(record) > 0 && record[len(record)-1] == '\n' {
		record = record[:len(record)-1]
	}
	if len(record) > 0 && record[len(record)-1] == '\r' {
		record = record[:len(record)-1]
	}
	return record
}

func isTornStoryJSON(line []byte, decodeErr error) bool {
	var syntaxErr *json.SyntaxError
	if !errors.As(decodeErr, &syntaxErr) {
		return false
	}
	return syntaxErr.Offset >= int64(len(line)) && strings.Contains(syntaxErr.Error(), "unexpected end of JSON input")
}

func (s *Store) rememberStoryReplayStats(storyID string, stats StoryJournalReplayStats) {
	if s.lastStoryReplayByStory == nil {
		s.lastStoryReplayByStory = make(map[string]StoryJournalReplayStats)
	}
	s.lastStoryReplayByStory[strings.TrimSpace(storyID)] = stats
}

func (s *Store) freezeLegacyActorStateSchemaLocked(storyID string, meta *StoryMeta, events []StoryEventRecord) error {
	if meta == nil || meta.ActorStateSchema != nil {
		return nil
	}
	if data, err := os.ReadFile(s.actorStateSchemaPath(storyID)); err == nil {
		var snapshot ActorStateSchemaSnapshot
		if err := json.Unmarshal(data, &snapshot); err != nil {
			return fmt.Errorf("解析旧故事冻结状态 schema 失败: %w", err)
		}
		before, _ := json.Marshal(snapshot)
		enrichLegacyActorStateSchema(&snapshot, stateFromPath(events))
		meta.ActorStateSchema = normalizeActorStateSchemaSnapshot(&snapshot)
		after, _ := json.Marshal(meta.ActorStateSchema)
		if string(before) == string(after) {
			return nil
		}
		return s.writeActorStateSchemaSnapshot(storyID, meta.ActorStateSchema)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("读取旧故事冻结状态 schema 失败: %w", err)
	}
	if strings.TrimSpace(s.novaDir) == "" {
		return nil
	}
	director := s.storyDirectorForMeta(*meta)
	if err := validateActorStateSystem(director.ActorState); err != nil {
		return fmt.Errorf("旧故事状态 schema 需要人工处理，未执行迁移: %w", err)
	}
	backupDir := filepath.Join(s.novaDir, "backups", "state-system-v6", time.Now().UTC().Format("20060102T150405.000000000Z"))
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		return fmt.Errorf("创建旧故事状态迁移备份目录失败: %w", err)
	}
	data, err := os.ReadFile(s.storyPath(storyID))
	if err != nil {
		return fmt.Errorf("读取旧故事状态迁移备份失败: %w", err)
	}
	if err := os.WriteFile(filepath.Join(backupDir, filepath.Base(s.storyPath(storyID))), data, 0o644); err != nil {
		return fmt.Errorf("写入旧故事状态迁移备份失败: %w", err)
	}
	meta.ActorStateSchema = FreezeActorStateSchemaWithRules(director.ActorState, director.TRPGSystem, true)
	enrichLegacyActorStateSchema(meta.ActorStateSchema, stateFromPath(events))
	return s.writeActorStateSchemaSnapshot(storyID, meta.ActorStateSchema)
}

func (s *Store) writeActorStateSchemaSnapshot(storyID string, snapshot *ActorStateSchemaSnapshot) error {
	schemaPath := s.actorStateSchemaPath(storyID)
	if err := os.MkdirAll(filepath.Dir(schemaPath), 0o755); err != nil {
		return fmt.Errorf("创建旧故事冻结状态 schema 目录失败: %w", err)
	}
	schemaData, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化旧故事冻结状态 schema 失败: %w", err)
	}
	tmp := schemaPath + ".tmp"
	if err := os.WriteFile(tmp, append(schemaData, '\n'), 0o644); err != nil {
		return fmt.Errorf("写入旧故事冻结状态 schema 失败: %w", err)
	}
	if err := os.Rename(tmp, schemaPath); err != nil {
		return fmt.Errorf("提交旧故事冻结状态 schema 失败: %w", err)
	}
	return nil
}

func (s *Store) rewriteStoryLocked(storyID string, meta StoryMeta, events []StoryEventRecord, newEvents ...any) error {
	meta = normalizeStoryMeta(meta)
	if err := validateStoryMeta(meta); err != nil {
		return err
	}
	lines := make([]any, 0, len(events)+len(newEvents)+1)
	lines = append(lines, meta)
	for _, event := range events {
		record, err := mapToStoryEventRecord(event.Raw)
		if err != nil {
			return err
		}
		lines = append(lines, record.Raw)
	}
	for _, event := range newEvents {
		record, err := storyEventRecordForWrite(event)
		if err != nil {
			return err
		}
		lines = append(lines, record.Raw)
	}
	writer := writeJSONL
	if s.rewriteJSONL != nil {
		writer = s.rewriteJSONL
	}
	return writer(s.storyPath(storyID), lines)
}

func writeJSONL(path string, lines []any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmp := file.Name()
	defer os.Remove(tmp)
	if err := file.Chmod(0o644); err != nil {
		_ = file.Close()
		return err
	}
	enc := json.NewEncoder(file)
	enc.SetEscapeHTML(false)
	for _, line := range lines {
		if err := enc.Encode(line); err != nil {
			_ = file.Close()
			return err
		}
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	return syncDirectory(filepath.Dir(path))
}

func writeAtomicBytes(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	file, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmp := file.Name()
	defer os.Remove(tmp)
	if err := file.Chmod(mode); err != nil {
		_ = file.Close()
		return err
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	return syncDirectory(dir)
}

func syncDirectory(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

func mapToStruct(raw map[string]any, out any) error {
	data, err := json.Marshal(raw)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, out)
}
