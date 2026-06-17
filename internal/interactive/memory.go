package interactive

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	defaultMemoryImportance = 3
	maxMemoryTextBytes      = 12 * 1024
	maxMemoryListItems      = 24
	maxMemoryRecalls        = 20
)

type interactiveMemoryBook struct {
	V       int                       `json:"v"`
	StoryID string                    `json:"story_id"`
	Entries []InteractiveMemoryEntry  `json:"entries"`
	Recalls []InteractiveMemoryRecall `json:"recalls,omitempty"`
}

func (s *Store) memoryDir() string {
	return filepath.Join(s.root, "interactive", "memory")
}

func (s *Store) memoryPath(storyID string) string {
	return filepath.Join(s.memoryDir(), "story-"+storyID+".json")
}

func (s *Store) InteractiveMemory(storyID, branchID string, includeHidden bool) (InteractiveMemoryState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	meta, lines, err := s.readStoryLocked(storyID)
	if err != nil {
		return InteractiveMemoryState{}, err
	}
	branchID, branch, err := resolveBranch(meta, branchID)
	if err != nil {
		return InteractiveMemoryState{}, err
	}
	book, err := s.readMemoryBookLocked(storyID)
	if err != nil {
		return InteractiveMemoryState{}, err
	}
	entries := filterMemoryEntries(book.Entries, branchID, includeHidden)
	status, statusErr := latestMemorySyncStatus(lines, branchID, branch.Head)
	return InteractiveMemoryState{
		StoryID:      storyID,
		BranchID:     branchID,
		Entries:      entries,
		RecentRecall: latestMemoryRecall(book.Recalls, branchID),
		SyncStatus:   status,
		SyncError:    statusErr,
	}, nil
}

func (s *Store) CreateInteractiveMemory(storyID string, req InteractiveMemoryCreateRequest) (InteractiveMemoryEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	meta, _, err := s.readStoryLocked(storyID)
	if err != nil {
		return InteractiveMemoryEntry{}, err
	}
	branchID, _, err := resolveBranch(meta, req.BranchID)
	if err != nil {
		return InteractiveMemoryEntry{}, err
	}
	entry, err := newInteractiveMemoryEntry(branchID, strings.TrimSpace(req.TurnID), true, req)
	if err != nil {
		return InteractiveMemoryEntry{}, err
	}
	book, err := s.readMemoryBookLocked(storyID)
	if err != nil {
		return InteractiveMemoryEntry{}, err
	}
	book.Entries = append(book.Entries, entry)
	if err := s.writeMemoryBookLocked(storyID, book); err != nil {
		return InteractiveMemoryEntry{}, err
	}
	return entry, nil
}

func (s *Store) UpdateInteractiveMemory(storyID, memoryID string, req InteractiveMemoryUpdateRequest) (InteractiveMemoryEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, _, err := s.readStoryLocked(storyID); err != nil {
		return InteractiveMemoryEntry{}, err
	}
	memoryID = strings.TrimSpace(memoryID)
	if memoryID == "" {
		return InteractiveMemoryEntry{}, fmt.Errorf("记忆 ID 不能为空")
	}
	book, err := s.readMemoryBookLocked(storyID)
	if err != nil {
		return InteractiveMemoryEntry{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for i := range book.Entries {
		if book.Entries[i].ID != memoryID {
			continue
		}
		applyMemoryUpdate(&book.Entries[i], req)
		if err := validateMemoryEntry(book.Entries[i]); err != nil {
			return InteractiveMemoryEntry{}, err
		}
		book.Entries[i].UpdatedAt = now
		if err := s.writeMemoryBookLocked(storyID, book); err != nil {
			return InteractiveMemoryEntry{}, err
		}
		return book.Entries[i], nil
	}
	return InteractiveMemoryEntry{}, fmt.Errorf("记忆不存在: %s", memoryID)
}

func (s *Store) SetInteractiveMemoryHidden(storyID, memoryID string, hidden bool) (InteractiveMemoryEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, _, err := s.readStoryLocked(storyID); err != nil {
		return InteractiveMemoryEntry{}, err
	}
	memoryID = strings.TrimSpace(memoryID)
	if memoryID == "" {
		return InteractiveMemoryEntry{}, fmt.Errorf("记忆 ID 不能为空")
	}
	book, err := s.readMemoryBookLocked(storyID)
	if err != nil {
		return InteractiveMemoryEntry{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for i := range book.Entries {
		if book.Entries[i].ID != memoryID {
			continue
		}
		book.Entries[i].Hidden = hidden
		book.Entries[i].UpdatedAt = now
		if err := s.writeMemoryBookLocked(storyID, book); err != nil {
			return InteractiveMemoryEntry{}, err
		}
		return book.Entries[i], nil
	}
	return InteractiveMemoryEntry{}, fmt.Errorf("记忆不存在: %s", memoryID)
}

func (s *Store) AppendInteractiveMemory(storyID, branchID, turnID string, req InteractiveMemoryCreateRequest) (InteractiveMemoryEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	meta, lines, err := s.readStoryLocked(storyID)
	if err != nil {
		return InteractiveMemoryEntry{}, err
	}
	branchID, _, err = resolveBranch(meta, branchID)
	if err != nil {
		return InteractiveMemoryEntry{}, err
	}
	turnID = strings.TrimSpace(turnID)
	if turnID == "" {
		return InteractiveMemoryEntry{}, fmt.Errorf("记忆缺少所属回合")
	}
	entry, err := newInteractiveMemoryEntry(branchID, turnID, false, req)
	if err != nil {
		return InteractiveMemoryEntry{}, err
	}
	book, err := s.readMemoryBookLocked(storyID)
	if err != nil {
		return InteractiveMemoryEntry{}, err
	}
	replaced := false
	for i := range book.Entries {
		if book.Entries[i].BranchID == branchID && book.Entries[i].TurnID == turnID && !book.Entries[i].Manual {
			entry.ID = book.Entries[i].ID
			entry.CreatedAt = book.Entries[i].CreatedAt
			book.Entries[i] = entry
			replaced = true
			break
		}
	}
	if !replaced {
		book.Entries = append(book.Entries, entry)
	}
	if err := s.writeMemoryBookLocked(storyID, book); err != nil {
		return InteractiveMemoryEntry{}, err
	}
	if err := s.markTurnMemoryReadyLocked(storyID, meta, lines, branchID, turnID, entry.ID); err != nil {
		return InteractiveMemoryEntry{}, err
	}
	return entry, nil
}

func (s *Store) MarkInteractiveMemoryReady(storyID, branchID, turnID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	meta, lines, err := s.readStoryLocked(storyID)
	if err != nil {
		return err
	}
	branchID, _, err = resolveBranch(meta, branchID)
	if err != nil {
		return err
	}
	return s.markTurnMemoryReadyLocked(storyID, meta, lines, branchID, strings.TrimSpace(turnID), "")
}

func (s *Store) MarkInteractiveMemoryFailed(storyID string, req MarkStateFailedRequest) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	meta, lines, err := s.readStoryLocked(storyID)
	if err != nil {
		return err
	}
	branchID := req.BranchID
	if branchID == "" {
		branchID = meta.CurrentBranch
	}
	if _, ok := meta.Branches[branchID]; !ok {
		return fmt.Errorf("分支不存在: %s", branchID)
	}
	parentID := strings.TrimSpace(req.ParentID)
	if parentID == "" {
		return fmt.Errorf("记忆失败标记缺少所属回合")
	}
	errText := strings.TrimSpace(req.Error)
	if errText == "" {
		errText = "记忆生成失败"
	}
	updated := false
	for _, record := range lines {
		raw := record.Raw
		if record.Envelope.ID != parentID || record.Envelope.Type != StoryEventTypeTurn {
			continue
		}
		raw["memory_status"] = "failed"
		raw["memory_error"] = errText
		if current, ok := raw["state_status"].(string); ok && current == "pending" {
			raw["state_status"] = "failed"
			raw["state_error"] = errText
		}
		updated = true
		break
	}
	if !updated {
		return fmt.Errorf("记忆失败标记所属回合不存在: %s", parentID)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	meta.UpdatedAt = now
	if err := s.rewriteStoryLocked(storyID, meta, lines); err != nil {
		return err
	}
	return s.touchIndexLocked(storyID, now, 0)
}

func (s *Store) VisibleInteractiveMemories(storyID, branchID string, limit int) ([]InteractiveMemoryEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	meta, _, err := s.readStoryLocked(storyID)
	if err != nil {
		return nil, err
	}
	branchID, _, err = resolveBranch(meta, branchID)
	if err != nil {
		return nil, err
	}
	book, err := s.readMemoryBookLocked(storyID)
	if err != nil {
		return nil, err
	}
	entries := filterMemoryEntries(book.Entries, branchID, false)
	if limit <= 0 || limit > maxMemoryListItems {
		limit = maxMemoryListItems
	}
	if len(entries) > limit {
		entries = entries[:limit]
	}
	return entries, nil
}

func (s *Store) ReadVisibleInteractiveMemories(storyID, branchID string, ids []string, limit int) ([]InteractiveMemoryEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	meta, _, err := s.readStoryLocked(storyID)
	if err != nil {
		return nil, err
	}
	branchID, _, err = resolveBranch(meta, branchID)
	if err != nil {
		return nil, err
	}
	wanted := sanitizeStringList(ids)
	if len(wanted) == 0 {
		return []InteractiveMemoryEntry{}, nil
	}
	if limit <= 0 || limit > 6 {
		limit = 6
	}
	book, err := s.readMemoryBookLocked(storyID)
	if err != nil {
		return nil, err
	}
	visible := filterMemoryEntries(book.Entries, branchID, false)
	byID := make(map[string]InteractiveMemoryEntry, len(visible))
	for _, entry := range visible {
		byID[entry.ID] = entry
	}
	capacity := len(wanted)
	if capacity > limit {
		capacity = limit
	}
	out := make([]InteractiveMemoryEntry, 0, capacity)
	seen := map[string]bool{}
	for _, id := range wanted {
		if seen[id] {
			continue
		}
		entry, ok := byID[id]
		if !ok {
			continue
		}
		out = append(out, entry)
		seen[id] = true
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (s *Store) RecordInteractiveMemoryRecall(storyID, branchID, turnID, query string, memoryIDs []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	meta, _, err := s.readStoryLocked(storyID)
	if err != nil {
		return err
	}
	branchID, _, err = resolveBranch(meta, branchID)
	if err != nil {
		return err
	}
	book, err := s.readMemoryBookLocked(storyID)
	if err != nil {
		return err
	}
	recall := InteractiveMemoryRecall{
		BranchID:  branchID,
		TurnID:    strings.TrimSpace(turnID),
		Query:     trimMemoryText(query),
		MemoryIDs: sanitizeStringList(memoryIDs),
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	book.Recalls = append(book.Recalls, recall)
	if len(book.Recalls) > maxMemoryRecalls {
		book.Recalls = book.Recalls[len(book.Recalls)-maxMemoryRecalls:]
	}
	return s.writeMemoryBookLocked(storyID, book)
}

func (s *Store) readMemoryBookLocked(storyID string) (interactiveMemoryBook, error) {
	path := s.memoryPath(storyID)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return interactiveMemoryBook{V: schemaVersion, StoryID: storyID, Entries: []InteractiveMemoryEntry{}}, nil
	}
	if err != nil {
		return interactiveMemoryBook{}, err
	}
	var book interactiveMemoryBook
	if err := json.Unmarshal(data, &book); err != nil {
		return interactiveMemoryBook{}, fmt.Errorf("解析互动记忆失败: %w", err)
	}
	book.V = schemaVersion
	book.StoryID = storyID
	if book.Entries == nil {
		book.Entries = []InteractiveMemoryEntry{}
	}
	return book, nil
}

func (s *Store) writeMemoryBookLocked(storyID string, book interactiveMemoryBook) error {
	if err := os.MkdirAll(s.memoryDir(), 0o755); err != nil {
		return err
	}
	book.V = schemaVersion
	book.StoryID = storyID
	if book.Entries == nil {
		book.Entries = []InteractiveMemoryEntry{}
	}
	data, err := json.MarshalIndent(book, "", "  ")
	if err != nil {
		return err
	}
	path := s.memoryPath(storyID)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (s *Store) markTurnMemoryReadyLocked(storyID string, meta StoryMeta, lines []StoryEventRecord, branchID, turnID, memoryID string) error {
	if turnID == "" {
		return fmt.Errorf("记忆完成标记缺少所属回合")
	}
	updated := false
	for _, record := range lines {
		raw := record.Raw
		if record.Envelope.ID != turnID || record.Envelope.Type != StoryEventTypeTurn {
			continue
		}
		raw["memory_status"] = "ready"
		if memoryID != "" {
			raw["memory_entry_id"] = memoryID
		}
		delete(raw, "memory_error")
		if current, ok := raw["state_status"].(string); ok && current == "pending" {
			raw["state_status"] = "ready"
			delete(raw, "state_error")
		}
		updated = true
		break
	}
	if !updated {
		return fmt.Errorf("记忆所属回合不存在: %s", turnID)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	meta.UpdatedAt = now
	if branch, ok := meta.Branches[branchID]; ok {
		branch.Head = turnID
		meta.Branches[branchID] = branch
	}
	if err := s.rewriteStoryLocked(storyID, meta, lines); err != nil {
		return err
	}
	return s.touchIndexLocked(storyID, now, 0)
}

func newInteractiveMemoryEntry(branchID, turnID string, manual bool, req InteractiveMemoryCreateRequest) (InteractiveMemoryEntry, error) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	entry := InteractiveMemoryEntry{
		ID:         newID("mem"),
		BranchID:   strings.TrimSpace(branchID),
		TurnID:     strings.TrimSpace(turnID),
		Title:      trimMemoryText(req.Title),
		Summary:    trimMemoryText(req.Summary),
		Content:    trimMemoryText(req.Content),
		People:     sanitizeStringList(req.People),
		Places:     sanitizeStringList(req.Places),
		Tags:       sanitizeStringList(req.Tags),
		Importance: normalizeMemoryImportance(req.Importance),
		Manual:     manual,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if entry.Title == "" && entry.Summary != "" {
		entry.Title = memoryPreview(entry.Summary, 24)
	}
	if err := validateMemoryEntry(entry); err != nil {
		return InteractiveMemoryEntry{}, err
	}
	return entry, nil
}

func applyMemoryUpdate(entry *InteractiveMemoryEntry, req InteractiveMemoryUpdateRequest) {
	if req.Title != nil {
		entry.Title = trimMemoryText(*req.Title)
	}
	if req.Summary != nil {
		entry.Summary = trimMemoryText(*req.Summary)
	}
	if req.Content != nil {
		entry.Content = trimMemoryText(*req.Content)
	}
	if req.People != nil {
		entry.People = sanitizeStringList(req.People)
	}
	if req.Places != nil {
		entry.Places = sanitizeStringList(req.Places)
	}
	if req.Tags != nil {
		entry.Tags = sanitizeStringList(req.Tags)
	}
	if req.Importance != nil {
		entry.Importance = normalizeMemoryImportance(*req.Importance)
	}
}

func validateMemoryEntry(entry InteractiveMemoryEntry) error {
	if strings.TrimSpace(entry.BranchID) == "" {
		return fmt.Errorf("记忆缺少分支")
	}
	if strings.TrimSpace(entry.Title) == "" {
		return fmt.Errorf("记忆标题不能为空")
	}
	if strings.TrimSpace(entry.Summary) == "" && strings.TrimSpace(entry.Content) == "" {
		return fmt.Errorf("记忆摘要或正文至少需要一项")
	}
	return nil
}

func filterMemoryEntries(entries []InteractiveMemoryEntry, branchID string, includeHidden bool) []InteractiveMemoryEntry {
	out := make([]InteractiveMemoryEntry, 0, len(entries))
	for _, entry := range entries {
		if entry.BranchID != branchID {
			continue
		}
		if entry.Hidden && !includeHidden {
			continue
		}
		out = append(out, entry)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].UpdatedAt == out[j].UpdatedAt {
			return out[i].CreatedAt > out[j].CreatedAt
		}
		return out[i].UpdatedAt > out[j].UpdatedAt
	})
	return out
}

func latestMemoryRecall(recalls []InteractiveMemoryRecall, branchID string) *InteractiveMemoryRecall {
	for i := len(recalls) - 1; i >= 0; i-- {
		if recalls[i].BranchID != branchID {
			continue
		}
		recall := recalls[i]
		return &recall
	}
	return nil
}

func latestMemorySyncStatus(lines []StoryEventRecord, branchID, headID string) (string, string) {
	if headID == "" {
		return "", ""
	}
	for _, record := range lines {
		if record.Envelope.ID != headID || record.Envelope.BranchID != branchID || record.Envelope.Type != StoryEventTypeTurn {
			continue
		}
		var turn TurnEvent
		if err := mapToStruct(record.Raw, &turn); err != nil {
			return "failed", err.Error()
		}
		return turn.MemoryStatus, turn.MemoryError
	}
	return "", ""
}

func trimMemoryText(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= maxMemoryTextBytes {
		return value
	}
	return value[:maxMemoryTextBytes]
}

func normalizeMemoryImportance(value int) int {
	if value <= 0 {
		return defaultMemoryImportance
	}
	if value > 5 {
		return 5
	}
	return value
}

func sanitizeStringList(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		out = append(out, value)
		seen[value] = true
		if len(out) >= 20 {
			break
		}
	}
	return out
}

func memoryPreview(value string, limit int) string {
	runes := []rune(strings.TrimSpace(value))
	if limit <= 0 || len(runes) <= limit {
		return string(runes)
	}
	return string(runes[:limit])
}
