package interactive

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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
	if err := os.MkdirAll(s.storyDir(), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.indexPath(), data, 0o644)
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
	file, err := os.Open(s.storyPath(storyID))
	if err != nil {
		return StoryMeta{}, nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), maxStoryLineBytes)
	if !scanner.Scan() {
		return StoryMeta{}, nil, fmt.Errorf("故事文件为空: %s", storyID)
	}
	var meta StoryMeta
	if err := json.Unmarshal(scanner.Bytes(), &meta); err != nil {
		return StoryMeta{}, nil, fmt.Errorf("解析故事元信息失败: %w", err)
	}
	meta = normalizeStoryMeta(meta)
	if err := validateStoryMeta(meta); err != nil {
		return StoryMeta{}, nil, fmt.Errorf("校验故事元信息失败: %w", err)
	}
	var lines []StoryEventRecord
	for scanner.Scan() {
		record, err := decodeStoryEventRecord(scanner.Bytes())
		if err != nil {
			return StoryMeta{}, nil, fmt.Errorf("解析故事事件失败: %w", err)
		}
		lines = append(lines, record)
	}
	if err := scanner.Err(); err != nil {
		return StoryMeta{}, nil, err
	}
	return meta, lines, nil
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
	return writeJSONL(s.storyPath(storyID), lines)
}

func writeJSONL(path string, lines []any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	file, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
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
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func mapToStruct(raw map[string]any, out any) error {
	data, err := json.Marshal(raw)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, out)
}
