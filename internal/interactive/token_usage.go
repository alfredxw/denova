package interactive

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"time"
)

// AppendTokenUsageEvent stores model usage separately from story events.
// It does not move branch heads, rewrite story JSONL, or affect story event counts.
func (s *Store) AppendTokenUsageEvent(storyID string, event TokenUsageEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	meta, _, err := s.readStoryLocked(storyID)
	if err != nil {
		return err
	}
	if event.BranchID == "" {
		event.BranchID = meta.CurrentBranch
	}
	if _, ok := meta.Branches[event.BranchID]; !ok {
		return fmt.Errorf("分支不存在: %s", event.BranchID)
	}
	event.StoryID = storyID
	if event.CreatedAt == "" {
		event.CreatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	event = sanitizeTokenUsageEvent(event)
	if event.TotalTokens == 0 && event.PromptTokens == 0 && event.CompletionTokens == 0 && len(event.UsageCalls) == 0 {
		return nil
	}
	return appendJSONL(s.usagePath(storyID), event)
}

func (s *Store) readTokenUsageEventsLocked(storyID, branchID string) ([]TokenUsageEvent, error) {
	file, err := os.Open(s.usagePath(storyID))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), maxStoryLineBytes)
	events := make([]TokenUsageEvent, 0)
	for scanner.Scan() {
		var event TokenUsageEvent
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			return nil, fmt.Errorf("解析模型用量记录失败: %w", err)
		}
		if event.Type != "" && event.Type != TokenUsageEventType {
			continue
		}
		event = sanitizeTokenUsageEvent(event)
		if event.BranchID != branchID {
			continue
		}
		events = append(events, event)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	sort.SliceStable(events, func(i, j int) bool {
		return events[i].CreatedAt < events[j].CreatedAt
	})
	return events, nil
}
