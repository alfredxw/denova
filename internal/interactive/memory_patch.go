package interactive

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"
)

const interactiveMemoryRunMarkerPrefix = "memory-run:"

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
	structures, _ := s.storyMemoryStructuresForStoryLocked(meta, book)
	record, err := saveStoryMemoryRecordWithStructuresLocked(&book, structures, branchID, branch.Head, req, true, eventPathSet(branch.Head, lines))
	if err != nil {
		return StoryMemoryRecord{}, err
	}
	if err := s.writeMemoryBookLocked(storyID, book); err != nil {
		return StoryMemoryRecord{}, err
	}
	return record, nil
}

func (s *Store) SetStoryMemoryRecordArchived(storyID, recordID, branchID string, archived bool) (StoryMemoryRecord, error) {
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
	record, err := setStoryMemoryRecordArchivedLocked(&book, branchID, branch.Head, recordID, archived, eventPathSet(branch.Head, lines))
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
	structures, structureSource := s.storyMemoryStructuresForStoryLocked(meta, book)
	runtimeStructures := runtimeStoryMemoryStructures(structures, structureSource)
	if len(runtimeStructures) == 0 {
		log.Printf("[interactive-memory] skip story memory patches because memory structure is disabled or empty story_id=%s branch_id=%s memory_structure_id=%s disabled=%t", storyID, branchID, structureSource.ID, structureSource.Disabled)
		return []StoryMemoryRecord{}, nil
	}
	pathSet := eventPathSet(branch.Head, lines)
	records := make([]StoryMemoryRecord, 0, len(patches))
	for _, patch := range patches {
		normalizedPatch, ok := normalizeStoryMemoryPatchForAgent(book, runtimeStructures, patch)
		if !ok {
			log.Printf("[interactive-memory] skip story memory patch with missing keyed key story_id=%s branch_id=%s structure_id=%s", storyID, branchID, patch.StructureID)
			continue
		}
		structure := storyMemoryStructureByID(runtimeStructures, normalizedPatch.StructureID)
		if structure.Mode == "append" && strings.TrimSpace(normalizedPatch.RecordID) == "" {
			normalizedPatch.RecordID = storyMemoryPatchRecordID(anchorTurnID, normalizedPatch)
		}
		record, err := applyStoryMemoryPatchWithStructuresLocked(&book, runtimeStructures, branchID, anchorTurnID, normalizedPatch, pathSet)
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

func storyMemoryPatchRecordID(anchorTurnID string, patch StoryMemoryPatch) string {
	canonical := struct {
		TurnID      string            `json:"turn_id"`
		Op          string            `json:"op"`
		StructureID string            `json:"structure_id"`
		Key         string            `json:"key,omitempty"`
		Values      map[string]string `json:"values,omitempty"`
	}{
		TurnID:      strings.TrimSpace(anchorTurnID),
		Op:          strings.TrimSpace(patch.Op),
		StructureID: sanitizeMemoryID(patch.StructureID),
		Key:         strings.TrimSpace(patch.Key),
		Values:      patch.Values,
	}
	data, _ := json.Marshal(canonical)
	sum := sha256.Sum256(data)
	return "memrun-" + hex.EncodeToString(sum[:12])
}

func normalizeStoryMemoryPatchForAgent(book interactiveMemoryBook, structures []StoryMemoryStructure, patch StoryMemoryPatch) (StoryMemoryPatch, bool) {
	op := strings.TrimSpace(patch.Op)
	if op == "" {
		op = "upsert"
	}
	if op == "archive" || op == "restore" {
		return patch, true
	}
	structureID := sanitizeMemoryID(patch.StructureID)
	structure := storyMemoryStructureByID(structures, structureID)
	if structure.ID == "" || !storyMemoryStructureEnabled(structure) {
		return patch, false
	}
	if len(patch.Values) > 0 {
		nextValues := make(map[string]string, len(patch.Values))
		enabledFieldIDs := make(map[string]bool, len(structure.Fields))
		for _, field := range structure.Fields {
			if storyMemoryFieldEnabled(field) {
				enabledFieldIDs[field.ID] = true
			}
		}
		for key, value := range patch.Values {
			if enabledFieldIDs[key] {
				nextValues[key] = value
			}
		}
		patch.Values = nextValues
	}
	if structure.Mode != "keyed" {
		return patch, true
	}
	if strings.TrimSpace(patch.Key) != "" {
		return patch, true
	}
	if structure.KeyFieldID != "" {
		if key := strings.TrimSpace(patch.Values[structure.KeyFieldID]); key != "" {
			patch.Key = key
			return patch, true
		}
	}
	for _, record := range book.Records {
		if record.ID == sanitizeMemoryID(patch.RecordID) && record.StructureID == structure.ID {
			patch.Key = record.Key
			return patch, strings.TrimSpace(patch.Key) != ""
		}
	}
	return patch, false
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
	structures, source := s.storyMemoryStructuresForStoryLocked(meta, book)
	if len(runtimeStoryMemoryStructures(structures, source)) == 0 {
		return false, 0, nil
	}
	should, next := storyMemoryAutoDecisionLocked(book, lines, branchID, branch.Head)
	return should, next, nil
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
	structures, _ := s.storyMemoryStructuresForStoryLocked(meta, book)
	record, err := saveStoryMemoryRecordWithStructuresLocked(&book, structures, branchID, branch.Head, interactiveMemoryCreateToStoryRecord(req), true, eventPathSet(branch.Head, lines))
	if err != nil {
		return InteractiveMemoryEntry{}, err
	}
	if err := s.writeMemoryBookLocked(storyID, book); err != nil {
		return InteractiveMemoryEntry{}, err
	}
	return storyMemoryRecordToInteractiveEntry(record, storyMemoryStructureByID(structures, record.StructureID)), nil
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
	structures, _ := s.storyMemoryStructuresForStoryLocked(meta, book)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	pathSet := eventPathSet(branch.Head, lines)
	for i := range book.Records {
		if book.Records[i].ID != memoryID {
			continue
		}
		next := book.Records[i]
		applyInteractiveMemoryUpdateToRecord(&next, req)
		record, err := saveStoryMemoryRecordWithStructuresLocked(&book, structures, branchID, branch.Head, StoryMemoryRecordRequest{
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
		return storyMemoryRecordToInteractiveEntry(record, storyMemoryStructureByID(structures, record.StructureID)), nil
	}
	return InteractiveMemoryEntry{}, fmt.Errorf("记忆不存在: %s", memoryID)
}

func (s *Store) SetInteractiveMemoryArchived(storyID, memoryID string, archived bool) (InteractiveMemoryEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	meta, _, err := s.readStoryLocked(storyID)
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
	structures, _ := s.storyMemoryStructuresForStoryLocked(meta, book)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for i := range book.Records {
		if book.Records[i].ID != memoryID {
			continue
		}
		book.Records[i].Archived = archived
		book.Records[i].UpdatedAt = now
		if err := s.writeMemoryBookLocked(storyID, book); err != nil {
			return InteractiveMemoryEntry{}, err
		}
		return storyMemoryRecordToInteractiveEntry(book.Records[i], storyMemoryStructureByID(structures, book.Records[i].StructureID)), nil
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
	structures, source := s.storyMemoryStructuresForStoryLocked(meta, book)
	runtimeStructures := runtimeStoryMemoryStructures(structures, source)
	if len(runtimeStructures) == 0 {
		return InteractiveMemoryEntry{}, fmt.Errorf("故事记忆结构已禁用或为空")
	}
	recordReq := interactiveMemoryCreateToStoryRecord(req)
	recordReq.BranchID = branchID
	recordReq.TurnID = turnID
	record, err := saveStoryMemoryRecordWithStructuresLocked(&book, runtimeStructures, branchID, branch.Head, recordReq, false, eventPathSet(branch.Head, lines))
	if err != nil {
		return InteractiveMemoryEntry{}, err
	}
	if err := s.writeMemoryBookLocked(storyID, book); err != nil {
		return InteractiveMemoryEntry{}, err
	}
	if err := s.markTurnMemoryReadyLocked(storyID, meta, lines, branchID, turnID, record.ID); err != nil {
		return InteractiveMemoryEntry{}, err
	}
	return storyMemoryRecordToInteractiveEntry(record, storyMemoryStructureByID(runtimeStructures, record.StructureID)), nil
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
	if manual && structure.ReadOnly {
		return StoryMemoryRecord{}, fmt.Errorf("故事记忆结构为只读派生表，不能手动编辑: %s", structure.ID)
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
			record.Archived = book.Records[i].Archived
			book.Records[i] = record
			return record, validateStoryMemoryRecord(record, structure)
		}
	}
	if !strings.HasPrefix(record.ID, "memrun_") {
		record.ID = newID("mem")
	}
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

func saveStoryMemoryRecordWithStructuresLocked(book *interactiveMemoryBook, structures []StoryMemoryStructure, branchID, anchorTurnID string, req StoryMemoryRecordRequest, manual bool, pathSet map[string]bool) (StoryMemoryRecord, error) {
	originalStructures := book.Structures
	book.Structures = structures
	defer func() {
		book.Structures = originalStructures
	}()
	return saveStoryMemoryRecordLocked(book, branchID, anchorTurnID, req, manual, pathSet)
}

func setStoryMemoryRecordArchivedLocked(book *interactiveMemoryBook, branchID, anchorTurnID, recordID string, archived bool, pathSet map[string]bool) (StoryMemoryRecord, error) {
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
			copy.Archived = archived
			copy.InheritedFrom = book.Records[i].ID
			copy.CreatedAt = now
			copy.UpdatedAt = now
			book.Records = append(book.Records, copy)
			return copy, nil
		}
		book.Records[i].Archived = archived
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
	case "archive":
		archived := true
		if patch.Archived != nil {
			archived = *patch.Archived
		}
		return setStoryMemoryRecordArchivedLocked(book, branchID, anchorTurnID, patch.RecordID, archived, pathSet)
	case "restore":
		return setStoryMemoryRecordArchivedLocked(book, branchID, anchorTurnID, patch.RecordID, false, pathSet)
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
					record.Source = "agent"
					break
				}
			}
		}
		return record, err
	default:
		return StoryMemoryRecord{}, fmt.Errorf("不支持的故事记忆操作: %s", op)
	}
}

func applyStoryMemoryPatchWithStructuresLocked(book *interactiveMemoryBook, structures []StoryMemoryStructure, branchID, anchorTurnID string, patch StoryMemoryPatch, pathSet map[string]bool) (StoryMemoryRecord, error) {
	originalStructures := book.Structures
	book.Structures = structures
	defer func() {
		book.Structures = originalStructures
	}()
	return applyStoryMemoryPatchLocked(book, branchID, anchorTurnID, patch, pathSet)
}

func findStoryMemoryUpsertRecord(records []StoryMemoryRecord, structure StoryMemoryStructure, branchID, key string, pathSet map[string]bool) (StoryMemoryRecord, bool) {
	visible := visibleStoryMemoryRecords(records, branchID, pathSet, false)
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
	for i, turn := range turns {
		if strings.HasPrefix(turn.MemoryEntryID, interactiveMemoryRunMarkerPrefix) {
			lastIndex = i
		}
	}
	pathSet := eventPathSet(headID, lines)
	for _, record := range visibleStoryMemoryRecords(book.Records, branchID, pathSet, false) {
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
