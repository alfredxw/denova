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
	defaultMemoryImportance    = 3
	defaultStoryMemoryInterval = 3
	maxMemoryTextBytes         = 12 * 1024
	maxMemoryListItems         = 24
	maxMemoryRecalls           = 20
)

type interactiveMemoryBook struct {
	V          int                       `json:"v"`
	StoryID    string                    `json:"story_id"`
	Settings   StoryMemorySettings       `json:"settings"`
	Structures []StoryMemoryStructure    `json:"structures"`
	Records    []StoryMemoryRecord       `json:"records"`
	Entries    []InteractiveMemoryEntry  `json:"entries,omitempty"`
	Recalls    []InteractiveMemoryRecall `json:"recalls,omitempty"`
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
	records := visibleStoryMemoryRecords(book.Records, branchID, eventPathSet(branch.Head, lines), includeHidden)
	entries := storyMemoryRecordsToInteractiveEntries(records, book.Structures)
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

func (s *Store) StoryMemory(storyID, branchID string, includeHidden bool) (StoryMemoryState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	meta, lines, err := s.readStoryLocked(storyID)
	if err != nil {
		return StoryMemoryState{}, err
	}
	branchID, branch, err := resolveBranch(meta, branchID)
	if err != nil {
		return StoryMemoryState{}, err
	}
	book, err := s.readMemoryBookLocked(storyID)
	if err != nil {
		return StoryMemoryState{}, err
	}
	records := visibleStoryMemoryRecords(book.Records, branchID, eventPathSet(branch.Head, lines), includeHidden)
	status, statusErr := latestMemorySyncStatus(lines, branchID, branch.Head)
	_, nextAuto := storyMemoryAutoDecisionLocked(book, lines, branchID, branch.Head)
	return StoryMemoryState{
		StoryID:         storyID,
		BranchID:        branchID,
		Settings:        book.Settings,
		Structures:      book.Structures,
		Records:         records,
		RecentRecall:    latestMemoryRecall(book.Recalls, branchID),
		SyncStatus:      status,
		SyncError:       statusErr,
		NextAutoInTurns: nextAuto,
	}, nil
}

func (s *Store) UpdateStoryMemorySettings(storyID string, req StoryMemorySettingsUpdateRequest) (StoryMemorySettings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, _, err := s.readStoryLocked(storyID); err != nil {
		return StoryMemorySettings{}, err
	}
	book, err := s.readMemoryBookLocked(storyID)
	if err != nil {
		return StoryMemorySettings{}, err
	}
	if req.Enabled != nil {
		book.Settings.Enabled = *req.Enabled
	}
	if req.AutoIntervalTurns != nil {
		book.Settings.AutoIntervalTurns = normalizeStoryMemoryInterval(*req.AutoIntervalTurns)
	}
	if err := s.writeMemoryBookLocked(storyID, book); err != nil {
		return StoryMemorySettings{}, err
	}
	return book.Settings, nil
}

func (s *Store) SaveStoryMemoryStructure(storyID string, req StoryMemoryStructureRequest) (StoryMemoryStructure, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, _, err := s.readStoryLocked(storyID); err != nil {
		return StoryMemoryStructure{}, err
	}
	book, err := s.readMemoryBookLocked(storyID)
	if err != nil {
		return StoryMemoryStructure{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	structure := normalizeStoryMemoryStructure(req, now)
	if structure.ID == "" {
		structure.ID = newID("sm")
	}
	if err := validateStoryMemoryStructure(structure); err != nil {
		return StoryMemoryStructure{}, err
	}
	for i := range book.Structures {
		if book.Structures[i].ID != structure.ID {
			continue
		}
		structure.BuiltIn = book.Structures[i].BuiltIn
		structure.CreatedAt = firstMemoryText(book.Structures[i].CreatedAt, now)
		structure.UpdatedAt = now
		book.Structures[i] = structure
		if err := s.writeMemoryBookLocked(storyID, book); err != nil {
			return StoryMemoryStructure{}, err
		}
		return structure, nil
	}
	structure.CreatedAt = now
	structure.UpdatedAt = now
	book.Structures = append(book.Structures, structure)
	sortStoryMemoryStructures(book.Structures)
	if err := s.writeMemoryBookLocked(storyID, book); err != nil {
		return StoryMemoryStructure{}, err
	}
	return structure, nil
}

func (s *Store) DeleteStoryMemoryStructure(storyID, structureID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, _, err := s.readStoryLocked(storyID); err != nil {
		return err
	}
	structureID = strings.TrimSpace(structureID)
	if structureID == "" {
		return fmt.Errorf("故事记忆结构 ID 不能为空")
	}
	book, err := s.readMemoryBookLocked(storyID)
	if err != nil {
		return err
	}
	next := book.Structures[:0]
	removed := false
	for _, structure := range book.Structures {
		if structure.ID == structureID {
			removed = true
			continue
		}
		next = append(next, structure)
	}
	if !removed {
		return fmt.Errorf("故事记忆结构不存在: %s", structureID)
	}
	book.Structures = next
	for i := range book.Records {
		if book.Records[i].StructureID == structureID {
			book.Records[i].Hidden = true
			book.Records[i].UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		}
	}
	return s.writeMemoryBookLocked(storyID, book)
}

func (s *Store) SaveStoryMemoryRecord(storyID string, req StoryMemoryRecordRequest) (StoryMemoryRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	meta, lines, err := s.readStoryLocked(storyID)
	if err != nil {
		return StoryMemoryRecord{}, err
	}
	branchID, branch, err := resolveBranch(meta, req.BranchID)
	if err != nil {
		return StoryMemoryRecord{}, err
	}
	book, err := s.readMemoryBookLocked(storyID)
	if err != nil {
		return StoryMemoryRecord{}, err
	}
	record, err := saveStoryMemoryRecordLocked(&book, branchID, branch.Head, req, true, eventPathSet(branch.Head, lines))
	if err != nil {
		return StoryMemoryRecord{}, err
	}
	if err := s.writeMemoryBookLocked(storyID, book); err != nil {
		return StoryMemoryRecord{}, err
	}
	return record, nil
}

func (s *Store) SetStoryMemoryRecordHidden(storyID, recordID, branchID string, hidden bool) (StoryMemoryRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	meta, lines, err := s.readStoryLocked(storyID)
	if err != nil {
		return StoryMemoryRecord{}, err
	}
	branchID, branch, err := resolveBranch(meta, branchID)
	if err != nil {
		return StoryMemoryRecord{}, err
	}
	book, err := s.readMemoryBookLocked(storyID)
	if err != nil {
		return StoryMemoryRecord{}, err
	}
	record, err := setStoryMemoryRecordHiddenLocked(&book, branchID, branch.Head, recordID, hidden, eventPathSet(branch.Head, lines))
	if err != nil {
		return StoryMemoryRecord{}, err
	}
	if err := s.writeMemoryBookLocked(storyID, book); err != nil {
		return StoryMemoryRecord{}, err
	}
	return record, nil
}

func (s *Store) ApplyStoryMemoryPatches(storyID, branchID, turnID string, patches []StoryMemoryPatch) ([]StoryMemoryRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	meta, lines, err := s.readStoryLocked(storyID)
	if err != nil {
		return nil, err
	}
	branchID, branch, err := resolveBranch(meta, branchID)
	if err != nil {
		return nil, err
	}
	anchorTurnID := strings.TrimSpace(turnID)
	if anchorTurnID == "" {
		anchorTurnID = branch.Head
	}
	book, err := s.readMemoryBookLocked(storyID)
	if err != nil {
		return nil, err
	}
	pathSet := eventPathSet(branch.Head, lines)
	records := make([]StoryMemoryRecord, 0, len(patches))
	for _, patch := range patches {
		record, err := applyStoryMemoryPatchLocked(&book, branchID, anchorTurnID, patch, pathSet)
		if err != nil {
			return nil, err
		}
		if record.ID != "" {
			records = append(records, record)
		}
	}
	if err := s.writeMemoryBookLocked(storyID, book); err != nil {
		return nil, err
	}
	return records, nil
}

func (s *Store) ShouldGenerateStoryMemory(storyID, branchID string) (bool, int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	meta, lines, err := s.readStoryLocked(storyID)
	if err != nil {
		return false, 0, err
	}
	branchID, branch, err := resolveBranch(meta, branchID)
	if err != nil {
		return false, 0, err
	}
	book, err := s.readMemoryBookLocked(storyID)
	if err != nil {
		return false, 0, err
	}
	should, next := storyMemoryAutoDecisionLocked(book, lines, branchID, branch.Head)
	return should, next, nil
}

func (s *Store) StoryMemoryContextSummary(storyID, branchID string, limit int) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	meta, lines, err := s.readStoryLocked(storyID)
	if err != nil {
		return "", err
	}
	branchID, branch, err := resolveBranch(meta, branchID)
	if err != nil {
		return "", err
	}
	book, err := s.readMemoryBookLocked(storyID)
	if err != nil {
		return "", err
	}
	records := visibleStoryMemoryRecords(book.Records, branchID, eventPathSet(branch.Head, lines), false)
	return formatStoryMemoryContextSummary(book.Structures, records, limit), nil
}

func (s *Store) CreateInteractiveMemory(storyID string, req InteractiveMemoryCreateRequest) (InteractiveMemoryEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	meta, lines, err := s.readStoryLocked(storyID)
	if err != nil {
		return InteractiveMemoryEntry{}, err
	}
	branchID, branch, err := resolveBranch(meta, req.BranchID)
	if err != nil {
		return InteractiveMemoryEntry{}, err
	}
	book, err := s.readMemoryBookLocked(storyID)
	if err != nil {
		return InteractiveMemoryEntry{}, err
	}
	record, err := saveStoryMemoryRecordLocked(&book, branchID, branch.Head, interactiveMemoryCreateToStoryRecord(req), true, eventPathSet(branch.Head, lines))
	if err != nil {
		return InteractiveMemoryEntry{}, err
	}
	if err := s.writeMemoryBookLocked(storyID, book); err != nil {
		return InteractiveMemoryEntry{}, err
	}
	return storyMemoryRecordToInteractiveEntry(record, storyMemoryStructureByID(book.Structures, record.StructureID)), nil
}

func (s *Store) UpdateInteractiveMemory(storyID, memoryID string, req InteractiveMemoryUpdateRequest) (InteractiveMemoryEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	meta, lines, err := s.readStoryLocked(storyID)
	if err != nil {
		return InteractiveMemoryEntry{}, err
	}
	branchID, branch, err := resolveBranch(meta, "")
	if err != nil {
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
	pathSet := eventPathSet(branch.Head, lines)
	for i := range book.Records {
		if book.Records[i].ID != memoryID {
			continue
		}
		next := book.Records[i]
		applyInteractiveMemoryUpdateToRecord(&next, req)
		record, err := saveStoryMemoryRecordLocked(&book, branchID, branch.Head, StoryMemoryRecordRequest{
			ID:          next.ID,
			BranchID:    branchID,
			StructureID: next.StructureID,
			TurnID:      next.TurnID,
			Key:         next.Key,
			Values:      next.Values,
			Manual:      next.Manual,
		}, next.Manual, pathSet)
		if err != nil {
			return InteractiveMemoryEntry{}, err
		}
		record.UpdatedAt = now
		if err := s.writeMemoryBookLocked(storyID, book); err != nil {
			return InteractiveMemoryEntry{}, err
		}
		return storyMemoryRecordToInteractiveEntry(record, storyMemoryStructureByID(book.Structures, record.StructureID)), nil
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
	for i := range book.Records {
		if book.Records[i].ID != memoryID {
			continue
		}
		book.Records[i].Hidden = hidden
		book.Records[i].UpdatedAt = now
		if err := s.writeMemoryBookLocked(storyID, book); err != nil {
			return InteractiveMemoryEntry{}, err
		}
		return storyMemoryRecordToInteractiveEntry(book.Records[i], storyMemoryStructureByID(book.Structures, book.Records[i].StructureID)), nil
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
	branchID, branch, err := resolveBranch(meta, branchID)
	if err != nil {
		return InteractiveMemoryEntry{}, err
	}
	turnID = strings.TrimSpace(turnID)
	if turnID == "" {
		return InteractiveMemoryEntry{}, fmt.Errorf("记忆缺少所属回合")
	}
	book, err := s.readMemoryBookLocked(storyID)
	if err != nil {
		return InteractiveMemoryEntry{}, err
	}
	recordReq := interactiveMemoryCreateToStoryRecord(req)
	recordReq.BranchID = branchID
	recordReq.TurnID = turnID
	record, err := saveStoryMemoryRecordLocked(&book, branchID, branch.Head, recordReq, false, eventPathSet(branch.Head, lines))
	if err != nil {
		return InteractiveMemoryEntry{}, err
	}
	if err := s.writeMemoryBookLocked(storyID, book); err != nil {
		return InteractiveMemoryEntry{}, err
	}
	if err := s.markTurnMemoryReadyLocked(storyID, meta, lines, branchID, turnID, record.ID); err != nil {
		return InteractiveMemoryEntry{}, err
	}
	return storyMemoryRecordToInteractiveEntry(record, storyMemoryStructureByID(book.Structures, record.StructureID)), nil
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

	meta, lines, err := s.readStoryLocked(storyID)
	if err != nil {
		return nil, err
	}
	branchID, branch, err := resolveBranch(meta, branchID)
	if err != nil {
		return nil, err
	}
	book, err := s.readMemoryBookLocked(storyID)
	if err != nil {
		return nil, err
	}
	records := visibleStoryMemoryRecords(book.Records, branchID, eventPathSet(branch.Head, lines), false)
	entries := storyMemoryRecordsToInteractiveEntries(records, book.Structures)
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

	meta, lines, err := s.readStoryLocked(storyID)
	if err != nil {
		return nil, err
	}
	branchID, branch, err := resolveBranch(meta, branchID)
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
	visible := storyMemoryRecordsToInteractiveEntries(visibleStoryMemoryRecords(book.Records, branchID, eventPathSet(branch.Head, lines), false), book.Structures)
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
		return normalizeMemoryBook(interactiveMemoryBook{V: 2, StoryID: storyID, Settings: StoryMemorySettings{Enabled: true, AutoIntervalTurns: defaultStoryMemoryInterval}}), nil
	}
	if err != nil {
		return interactiveMemoryBook{}, err
	}
	var book interactiveMemoryBook
	if err := json.Unmarshal(data, &book); err != nil {
		return interactiveMemoryBook{}, fmt.Errorf("解析互动记忆失败: %w", err)
	}
	book.StoryID = storyID
	return normalizeMemoryBook(book), nil
}

func (s *Store) writeMemoryBookLocked(storyID string, book interactiveMemoryBook) error {
	if err := os.MkdirAll(s.memoryDir(), 0o755); err != nil {
		return err
	}
	book = normalizeMemoryBook(book)
	book.V = 2
	book.StoryID = storyID
	book.Entries = nil
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

func normalizeMemoryBook(book interactiveMemoryBook) interactiveMemoryBook {
	if book.V <= 0 {
		book.V = 1
	}
	book.Settings = normalizeStoryMemorySettings(book.Settings, book.V)
	if len(book.Structures) == 0 {
		book.Structures = defaultStoryMemoryStructures()
	}
	if len(book.Records) == 0 && len(book.Entries) > 0 {
		book.Records = migrateInteractiveEntriesToStoryMemoryRecords(book.Entries)
	}
	for i := range book.Structures {
		book.Structures[i] = normalizeStoryMemoryStructureFromStored(book.Structures[i])
	}
	sortStoryMemoryStructures(book.Structures)
	if book.Records == nil {
		book.Records = []StoryMemoryRecord{}
	}
	return book
}

func normalizeStoryMemorySettings(settings StoryMemorySettings, version int) StoryMemorySettings {
	if settings.AutoIntervalTurns <= 0 {
		settings.AutoIntervalTurns = defaultStoryMemoryInterval
	}
	if version < 2 && !settings.Enabled {
		settings.Enabled = true
	}
	return settings
}

func normalizeStoryMemoryInterval(value int) int {
	if value <= 0 {
		return defaultStoryMemoryInterval
	}
	if value > 50 {
		return 50
	}
	return value
}

func defaultStoryMemoryStructures() []StoryMemoryStructure {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	return []StoryMemoryStructure{
		defaultStoryMemoryStructure("current_state", "当前状态", "Current State", "singleton", "", 10, []StoryMemoryField{
			defaultStoryMemoryField("time", "时间", "当前剧情时间", true, 10),
			defaultStoryMemoryField("location", "地点", "当前主要地点", true, 20),
			defaultStoryMemoryField("event", "当前事件", "正在发生或刚发生的事件", true, 30),
		}, now),
		defaultStoryMemoryStructure("protagonist", "主角信息", "Protagonist", "singleton", "", 20, []StoryMemoryField{
			defaultStoryMemoryField("name", "姓名", "主角姓名", false, 10),
			defaultStoryMemoryField("gender_age", "性别年龄", "性别、年龄或外显阶段", false, 20),
			defaultStoryMemoryField("appearance", "外貌", "稳定外貌和辨识特征", false, 30),
			defaultStoryMemoryField("experience", "经历", "已经发生并影响主角的经历", false, 40),
			defaultStoryMemoryField("skills", "技能", "能力、专长或限制", false, 50),
			defaultStoryMemoryField("items", "物品", "持有或失去的重要物品", false, 60),
		}, now),
		defaultStoryMemoryStructure("important_character", "重要角色", "Important Character", "keyed", "name", 30, []StoryMemoryField{
			defaultStoryMemoryField("name", "姓名", "角色姓名或称呼", true, 10),
			defaultStoryMemoryField("brief", "简介", "身份、性格和当前立场", false, 20),
			defaultStoryMemoryField("relationship", "关系", "与主角或关键角色的关系", false, 30),
			defaultStoryMemoryField("left_scene", "是否离场", "是否已经离开当前剧情舞台", false, 40),
			defaultStoryMemoryField("skills_items", "技能与持有物品", "能力、资源和持有物", false, 50),
			defaultStoryMemoryField("experience", "经历", "已发生的重要经历", false, 60),
		}, now),
		defaultStoryMemoryStructure("quest_event", "任务事件", "Quest Event", "keyed", "name", 40, []StoryMemoryField{
			defaultStoryMemoryField("name", "任务名", "任务、危机或承诺名称", true, 10),
			defaultStoryMemoryField("description", "描述", "任务背景和目标", false, 20),
			defaultStoryMemoryField("progress", "进度", "当前完成度和阻碍", false, 30),
			defaultStoryMemoryField("stakes", "奖励与惩罚", "成功收益、失败代价或倒计时", false, 40),
		}, now),
		defaultStoryMemoryStructure("plot_summary", "剧情纪要", "Plot Summary", "append", "", 50, []StoryMemoryField{
			defaultStoryMemoryField("time", "时间", "发生时间", false, 10),
			defaultStoryMemoryField("place", "地点", "发生地点", false, 20),
			defaultStoryMemoryField("event", "事件", "已经发生且后续需要承接的事实", true, 30),
		}, now),
	}
}

func defaultStoryMemoryStructure(id, name, description, mode, keyFieldID string, order int, fields []StoryMemoryField, now string) StoryMemoryStructure {
	return StoryMemoryStructure{ID: id, Name: name, Description: description, Mode: mode, KeyFieldID: keyFieldID, Fields: fields, Order: order, BuiltIn: true, CreatedAt: now, UpdatedAt: now}
}

func defaultStoryMemoryField(id, name, description string, required bool, order int) StoryMemoryField {
	return StoryMemoryField{ID: id, Name: name, Description: description, Required: required, Order: order}
}

func normalizeStoryMemoryStructure(req StoryMemoryStructureRequest, now string) StoryMemoryStructure {
	structure := StoryMemoryStructure{
		ID:          sanitizeMemoryID(req.ID),
		Name:        trimMemoryText(req.Name),
		Description: trimMemoryText(req.Description),
		Mode:        strings.TrimSpace(req.Mode),
		KeyFieldID:  sanitizeMemoryID(req.KeyFieldID),
		Order:       req.Order,
		Fields:      normalizeStoryMemoryFields(req.Fields),
		UpdatedAt:   now,
	}
	if structure.Mode == "" {
		structure.Mode = "append"
	}
	return structure
}

func normalizeStoryMemoryStructureFromStored(structure StoryMemoryStructure) StoryMemoryStructure {
	structure.ID = sanitizeMemoryID(structure.ID)
	structure.Name = trimMemoryText(structure.Name)
	structure.Description = trimMemoryText(structure.Description)
	structure.Mode = strings.TrimSpace(structure.Mode)
	if structure.Mode == "" {
		structure.Mode = "append"
	}
	structure.KeyFieldID = sanitizeMemoryID(structure.KeyFieldID)
	structure.Fields = normalizeStoryMemoryFields(structure.Fields)
	return structure
}

func normalizeStoryMemoryFields(fields []StoryMemoryField) []StoryMemoryField {
	out := make([]StoryMemoryField, 0, len(fields))
	for i, field := range fields {
		field.ID = sanitizeMemoryID(field.ID)
		if field.ID == "" {
			field.ID = fmt.Sprintf("field_%d", i+1)
		}
		field.Name = trimMemoryText(field.Name)
		if field.Name == "" {
			field.Name = field.ID
		}
		field.Description = trimMemoryText(field.Description)
		if field.Order == 0 {
			field.Order = (i + 1) * 10
		}
		out = append(out, field)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Order == out[j].Order {
			return out[i].ID < out[j].ID
		}
		return out[i].Order < out[j].Order
	})
	return out
}

func validateStoryMemoryStructure(structure StoryMemoryStructure) error {
	if strings.TrimSpace(structure.ID) == "" {
		return fmt.Errorf("故事记忆结构 ID 不能为空")
	}
	if strings.TrimSpace(structure.Name) == "" {
		return fmt.Errorf("故事记忆结构名称不能为空")
	}
	switch structure.Mode {
	case "singleton", "keyed", "append":
	default:
		return fmt.Errorf("故事记忆结构模式无效: %s", structure.Mode)
	}
	if len(structure.Fields) == 0 {
		return fmt.Errorf("故事记忆结构至少需要一个字段")
	}
	if structure.Mode == "keyed" && structure.KeyFieldID == "" {
		return fmt.Errorf("keyed 结构必须配置 key_field_id")
	}
	return nil
}

func sortStoryMemoryStructures(structures []StoryMemoryStructure) {
	sort.SliceStable(structures, func(i, j int) bool {
		if structures[i].Order == structures[j].Order {
			return structures[i].ID < structures[j].ID
		}
		return structures[i].Order < structures[j].Order
	})
}

func migrateInteractiveEntriesToStoryMemoryRecords(entries []InteractiveMemoryEntry) []StoryMemoryRecord {
	records := make([]StoryMemoryRecord, 0, len(entries))
	for _, entry := range entries {
		values := map[string]string{
			"event": firstMemoryText(entry.Summary, entry.Content, entry.Title),
		}
		if strings.TrimSpace(entry.Content) != "" {
			values["detail"] = trimMemoryText(entry.Content)
		}
		if len(entry.Places) > 0 {
			values["place"] = strings.Join(entry.Places, "，")
		}
		record := StoryMemoryRecord{
			ID:           firstMemoryText(entry.ID, newID("mem")),
			StructureID:  "plot_summary",
			BranchID:     entry.BranchID,
			TurnID:       entry.TurnID,
			AnchorTurnID: entry.TurnID,
			Key:          entry.Title,
			Values:       values,
			Hidden:       entry.Hidden,
			Manual:       entry.Manual,
			Source:       "legacy",
			CreatedAt:    entry.CreatedAt,
			UpdatedAt:    entry.UpdatedAt,
		}
		if record.CreatedAt == "" {
			record.CreatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		}
		if record.UpdatedAt == "" {
			record.UpdatedAt = record.CreatedAt
		}
		records = append(records, record)
	}
	return records
}

func visibleStoryMemoryRecords(records []StoryMemoryRecord, branchID string, pathSet map[string]bool, includeHidden bool) []StoryMemoryRecord {
	candidates := make([]StoryMemoryRecord, 0, len(records))
	for _, record := range records {
		if !recordVisibleOnBranch(record, branchID, pathSet) {
			continue
		}
		candidates = append(candidates, record)
	}
	overridden := map[string]bool{}
	for _, record := range candidates {
		if record.InheritedFrom != "" {
			overridden[record.InheritedFrom] = true
		}
	}
	out := make([]StoryMemoryRecord, 0, len(candidates))
	for _, record := range candidates {
		if overridden[record.ID] {
			continue
		}
		if record.Hidden && !includeHidden {
			continue
		}
		out = append(out, record)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].UpdatedAt == out[j].UpdatedAt {
			return out[i].CreatedAt > out[j].CreatedAt
		}
		return out[i].UpdatedAt > out[j].UpdatedAt
	})
	return out
}

func recordVisibleOnBranch(record StoryMemoryRecord, branchID string, pathSet map[string]bool) bool {
	if record.BranchID == branchID {
		return true
	}
	anchor := firstMemoryText(record.AnchorTurnID, record.TurnID)
	return anchor != "" && pathSet[anchor]
}

func storyMemoryRecordsToInteractiveEntries(records []StoryMemoryRecord, structures []StoryMemoryStructure) []InteractiveMemoryEntry {
	entries := make([]InteractiveMemoryEntry, 0, len(records))
	for _, record := range records {
		entries = append(entries, storyMemoryRecordToInteractiveEntry(record, storyMemoryStructureByID(structures, record.StructureID)))
	}
	return entries
}

func storyMemoryRecordToInteractiveEntry(record StoryMemoryRecord, structure StoryMemoryStructure) InteractiveMemoryEntry {
	title := record.Key
	if title == "" {
		title = firstMemoryText(record.Values["title"], record.Values["name"], structure.Name)
	}
	summary := firstMemoryText(record.Values["summary"], record.Values["event"], record.Values["description"], record.Values["brief"])
	contentParts := make([]string, 0, len(record.Values))
	used := map[string]bool{}
	for _, field := range structure.Fields {
		if value := strings.TrimSpace(record.Values[field.ID]); value != "" {
			contentParts = append(contentParts, field.Name+"："+value)
			used[field.ID] = true
		}
	}
	for key, value := range record.Values {
		if used[key] || strings.TrimSpace(value) == "" {
			continue
		}
		contentParts = append(contentParts, key+"："+value)
	}
	content := strings.Join(contentParts, "\n")
	return InteractiveMemoryEntry{
		ID:         record.ID,
		BranchID:   record.BranchID,
		TurnID:     record.TurnID,
		Title:      trimMemoryText(title),
		Summary:    trimMemoryText(summary),
		Content:    trimMemoryText(content),
		People:     valueListFromRecord(record, []string{"name", "people"}),
		Places:     valueListFromRecord(record, []string{"location", "place"}),
		Tags:       []string{structure.Name},
		Importance: defaultMemoryImportance,
		Hidden:     record.Hidden,
		Manual:     record.Manual,
		CreatedAt:  record.CreatedAt,
		UpdatedAt:  record.UpdatedAt,
	}
}

func valueListFromRecord(record StoryMemoryRecord, keys []string) []string {
	var values []string
	for _, key := range keys {
		if value := strings.TrimSpace(record.Values[key]); value != "" {
			values = append(values, value)
		}
	}
	return sanitizeStringList(values)
}

func storyMemoryStructureByID(structures []StoryMemoryStructure, id string) StoryMemoryStructure {
	for _, structure := range structures {
		if structure.ID == id {
			return structure
		}
	}
	return StoryMemoryStructure{ID: id, Name: id, Mode: "append"}
}

func interactiveMemoryCreateToStoryRecord(req InteractiveMemoryCreateRequest) StoryMemoryRecordRequest {
	values := map[string]string{
		"event": firstMemoryText(req.Summary, req.Content, req.Title),
	}
	if strings.TrimSpace(req.Content) != "" {
		values["detail"] = trimMemoryText(req.Content)
	}
	if len(req.Places) > 0 {
		values["place"] = strings.Join(sanitizeStringList(req.Places), "，")
	}
	return StoryMemoryRecordRequest{
		BranchID:    req.BranchID,
		StructureID: "plot_summary",
		TurnID:      req.TurnID,
		Key:         trimMemoryText(req.Title),
		Values:      values,
		Manual:      true,
	}
}

func saveStoryMemoryRecordLocked(book *interactiveMemoryBook, branchID, anchorTurnID string, req StoryMemoryRecordRequest, manual bool, pathSet map[string]bool) (StoryMemoryRecord, error) {
	req.StructureID = sanitizeMemoryID(req.StructureID)
	structure := storyMemoryStructureByID(book.Structures, req.StructureID)
	if structure.ID == "" {
		return StoryMemoryRecord{}, fmt.Errorf("故事记忆结构不存在: %s", req.StructureID)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	record := StoryMemoryRecord{
		ID:           sanitizeMemoryID(req.ID),
		StructureID:  req.StructureID,
		BranchID:     branchID,
		TurnID:       strings.TrimSpace(req.TurnID),
		AnchorTurnID: firstMemoryText(req.TurnID, anchorTurnID),
		Key:          trimMemoryText(req.Key),
		Values:       sanitizeStoryMemoryValues(req.Values),
		Manual:       manual || req.Manual,
		Source:       "manual",
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if record.Key == "" && structure.KeyFieldID != "" {
		record.Key = record.Values[structure.KeyFieldID]
	}
	if record.ID != "" {
		for i := range book.Records {
			if book.Records[i].ID != record.ID {
				continue
			}
			if book.Records[i].BranchID != branchID && recordVisibleOnBranch(book.Records[i], branchID, pathSet) {
				copy := book.Records[i]
				copy.ID = newID("mem")
				copy.BranchID = branchID
				copy.TurnID = ""
				copy.AnchorTurnID = ""
				copy.InheritedFrom = book.Records[i].ID
				copy.Values = record.Values
				copy.Key = record.Key
				copy.Manual = record.Manual
				copy.Source = record.Source
				copy.CreatedAt = now
				copy.UpdatedAt = now
				book.Records = append(book.Records, copy)
				return copy, validateStoryMemoryRecord(copy, structure)
			}
			record.CreatedAt = firstMemoryText(book.Records[i].CreatedAt, now)
			record.UpdatedAt = now
			book.Records[i] = record
			return record, validateStoryMemoryRecord(record, structure)
		}
	}
	record.ID = newID("mem")
	if structure.Mode != "append" {
		if existing, ok := findStoryMemoryUpsertRecord(book.Records, structure, branchID, record.Key, pathSet); ok {
			record.ID = existing.ID
			req.ID = existing.ID
			return saveStoryMemoryRecordLocked(book, branchID, anchorTurnID, req, manual, pathSet)
		}
	}
	if err := validateStoryMemoryRecord(record, structure); err != nil {
		return StoryMemoryRecord{}, err
	}
	book.Records = append(book.Records, record)
	return record, nil
}

func setStoryMemoryRecordHiddenLocked(book *interactiveMemoryBook, branchID, anchorTurnID, recordID string, hidden bool, pathSet map[string]bool) (StoryMemoryRecord, error) {
	recordID = sanitizeMemoryID(recordID)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for i := range book.Records {
		if book.Records[i].ID != recordID {
			continue
		}
		if book.Records[i].BranchID != branchID && recordVisibleOnBranch(book.Records[i], branchID, pathSet) {
			copy := book.Records[i]
			copy.ID = newID("mem")
			copy.BranchID = branchID
			copy.TurnID = ""
			copy.AnchorTurnID = ""
			copy.Hidden = hidden
			copy.InheritedFrom = book.Records[i].ID
			copy.CreatedAt = now
			copy.UpdatedAt = now
			book.Records = append(book.Records, copy)
			return copy, nil
		}
		book.Records[i].Hidden = hidden
		book.Records[i].UpdatedAt = now
		return book.Records[i], nil
	}
	return StoryMemoryRecord{}, fmt.Errorf("故事记忆不存在: %s", recordID)
}

func applyStoryMemoryPatchLocked(book *interactiveMemoryBook, branchID, anchorTurnID string, patch StoryMemoryPatch, pathSet map[string]bool) (StoryMemoryRecord, error) {
	op := strings.TrimSpace(patch.Op)
	if op == "" {
		op = "upsert"
	}
	switch op {
	case "hide":
		hidden := true
		if patch.Hidden != nil {
			hidden = *patch.Hidden
		}
		return setStoryMemoryRecordHiddenLocked(book, branchID, anchorTurnID, patch.RecordID, hidden, pathSet)
	case "upsert", "append", "set":
		record, err := saveStoryMemoryRecordLocked(book, branchID, anchorTurnID, StoryMemoryRecordRequest{
			ID:          patch.RecordID,
			StructureID: patch.StructureID,
			Key:         patch.Key,
			Values:      patch.Values,
		}, false, pathSet)
		if record.ID != "" {
			for i := range book.Records {
				if book.Records[i].ID == record.ID {
					book.Records[i].Source = "agent"
					break
				}
			}
		}
		return record, err
	default:
		return StoryMemoryRecord{}, fmt.Errorf("不支持的故事记忆操作: %s", op)
	}
}

func findStoryMemoryUpsertRecord(records []StoryMemoryRecord, structure StoryMemoryStructure, branchID, key string, pathSet map[string]bool) (StoryMemoryRecord, bool) {
	visible := visibleStoryMemoryRecords(records, branchID, pathSet, true)
	for _, record := range visible {
		if record.StructureID != structure.ID {
			continue
		}
		if structure.Mode == "singleton" {
			return record, true
		}
		if structure.Mode == "keyed" && strings.TrimSpace(record.Key) == strings.TrimSpace(key) {
			return record, true
		}
	}
	return StoryMemoryRecord{}, false
}

func validateStoryMemoryRecord(record StoryMemoryRecord, structure StoryMemoryStructure) error {
	if record.StructureID == "" {
		return fmt.Errorf("故事记忆缺少结构")
	}
	if record.BranchID == "" {
		return fmt.Errorf("故事记忆缺少分支")
	}
	if len(record.Values) == 0 {
		return fmt.Errorf("故事记忆内容不能为空")
	}
	if structure.Mode == "keyed" && strings.TrimSpace(record.Key) == "" {
		return fmt.Errorf("keyed 故事记忆缺少 key")
	}
	return nil
}

func applyInteractiveMemoryUpdateToRecord(record *StoryMemoryRecord, req InteractiveMemoryUpdateRequest) {
	if record.Values == nil {
		record.Values = map[string]string{}
	}
	if req.Title != nil {
		record.Key = trimMemoryText(*req.Title)
	}
	if req.Summary != nil {
		record.Values["event"] = trimMemoryText(*req.Summary)
	}
	if req.Content != nil && strings.TrimSpace(*req.Content) != "" {
		record.Values["detail"] = trimMemoryText(*req.Content)
	}
	if req.Places != nil {
		record.Values["place"] = strings.Join(sanitizeStringList(req.Places), "，")
	}
}

func storyMemoryAutoDecisionLocked(book interactiveMemoryBook, lines []StoryEventRecord, branchID, headID string) (bool, int) {
	if !book.Settings.Enabled {
		return false, 0
	}
	interval := normalizeStoryMemoryInterval(book.Settings.AutoIntervalTurns)
	turns := turnPath(lines, headID)
	lastIndex := -1
	pathSet := eventPathSet(headID, lines)
	for _, record := range visibleStoryMemoryRecords(book.Records, branchID, pathSet, true) {
		if record.Source != "agent" {
			continue
		}
		anchor := firstMemoryText(record.AnchorTurnID, record.TurnID)
		for i, turn := range turns {
			if turn.ID == anchor && i > lastIndex {
				lastIndex = i
			}
		}
	}
	delta := len(turns) - lastIndex - 1
	if delta >= interval {
		return true, interval
	}
	return false, interval - delta
}

func formatStoryMemoryContextSummary(structures []StoryMemoryStructure, records []StoryMemoryRecord, limit int) string {
	if limit <= 0 || limit > maxMemoryTextBytes {
		limit = maxMemoryTextBytes
	}
	var sb strings.Builder
	sb.WriteString("来源: interactive/memory/story-{story_id}.json 的当前分支可见故事记忆\n")
	sb.WriteString(fmt.Sprintf("上限: %d bytes\n", limit))
	count := 0
	for _, structure := range structures {
		items := make([]StoryMemoryRecord, 0)
		for _, record := range records {
			if record.StructureID == structure.ID {
				items = append(items, record)
			}
		}
		if len(items) == 0 {
			continue
		}
		sb.WriteString("\n## ")
		sb.WriteString(structure.Name)
		sb.WriteString("\n")
		for _, record := range items {
			if count >= maxMemoryListItems || sb.Len() >= limit {
				sb.WriteString("\n(后续故事记忆已截断)\n")
				return trimMemoryText(sb.String())
			}
			if record.Key != "" {
				sb.WriteString("- ")
				sb.WriteString(record.Key)
				sb.WriteString(": ")
			} else {
				sb.WriteString("- ")
			}
			parts := make([]string, 0, len(structure.Fields))
			for _, field := range structure.Fields {
				if value := strings.TrimSpace(record.Values[field.ID]); value != "" {
					parts = append(parts, field.Name+"="+value)
				}
			}
			if len(parts) == 0 {
				for key, value := range record.Values {
					parts = append(parts, key+"="+value)
				}
			}
			sb.WriteString(strings.Join(parts, "；"))
			sb.WriteString("\n")
			count++
		}
	}
	return trimMemoryText(sb.String())
}

func eventPathSet(headID string, lines []StoryEventRecord) map[string]bool {
	_, pathSet := eventPath(headID, eventsByID(lines))
	return pathSet
}

func turnPath(lines []StoryEventRecord, headID string) []TurnEvent {
	path, _ := eventPath(headID, eventsByID(lines))
	turns := make([]TurnEvent, 0, len(path))
	for _, record := range path {
		if record.Envelope.Type != StoryEventTypeTurn {
			continue
		}
		var turn TurnEvent
		if err := mapToStruct(record.Raw, &turn); err == nil {
			turns = append(turns, turn)
		}
	}
	return turns
}

func sanitizeStoryMemoryValues(values map[string]string) map[string]string {
	out := map[string]string{}
	for key, value := range values {
		key = sanitizeMemoryID(key)
		value = trimMemoryText(value)
		if key != "" && value != "" {
			out[key] = value
		}
	}
	return out
}

func sanitizeMemoryID(value string) string {
	value = strings.TrimSpace(value)
	value = strings.ReplaceAll(value, " ", "_")
	value = strings.ReplaceAll(value, "-", "_")
	return value
}

func firstMemoryText(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
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
